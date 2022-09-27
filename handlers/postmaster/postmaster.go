package postmaster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nyaruka/courier/backends/rapidpro"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"

	validator "gopkg.in/go-playground/validator.v9"
)

const (
	outgoingEndpoint = "engage/outgoing"
	purgeEndpoint    = "engage/outgoing/purge"
)

var (
	maxMsgLength = 20000
	validate     = validator.New()
	HEADER_API   = "po-api-key"
)

var statusMapping = map[string]courier.MsgStatusValue{
	"S": courier.MsgSent,
	"E": courier.MsgErrored,
	"D": courier.MsgDelivered,
	"F": courier.MsgFailed,
}

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler

	logger *logrus.Entry
}

func newHandler() courier.ChannelHandler {
	return &handler{
		BaseHandler: handlers.NewBaseHandler(courier.ChannelType("PSM"), "Postmaster"),
		logger: logrus.WithFields(logrus.Fields{
			"handler_type": courier.ChannelType("PSM"),
			"handler_name": "Postmaster",
		}),
	}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "status", h.receiveStatus)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	h.logger.WithFields(logrus.Fields{
		"channel_uuid": channel.UUID(),
		"channel_name": channel.Name(),
		"channel_type": channel.ChannelType(),
		"pm_imei":      channel.Address(),
		"scheme":       strings.Join(channel.Schemes(), ", "),
	}).Info("receiveMsg")
	payload := &incomingMessage{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if len(strings.Trim(payload.Text, " ")) == 0 && len(payload.Media) == 0 {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("no message content provided"))
	}

	var urn urns.URN
	mode := strings.ToUpper(payload.Mode)

	if len(channel.Schemes()) < 1 {
		return nil, fmt.Errorf("no scheme set for channel %s", channel.UUID())
	}

	scheme := channel.Schemes()[0]

	//Force scheme to be valid
	urns.ValidSchemes[scheme] = true

	//Handle SMS (tel) specially, everything else is a straight string passthrough
	if mode == "SMS" {
		// Remove out + and - just in case
		value := payload.Contact.Value
		value = strings.Replace(value, " ", "", -1)
		value = strings.Replace(value, "-", "", -1)

		// Only add + if it is a full phone, not a shortcode
		if len(value) > 8 {
			isNumeric := regexp.MustCompile(`^[0-9 ]+$`).MatchString

			if isNumeric(value) {
				value = "+" + value
			}
		}

		payload.Contact.Value = value
	}

	urn, err = urns.NewURNFromParts(scheme, payload.Contact.Value, "", "")

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Text).
		WithContactName(payload.Contact.Name).
		WithReceivedOn(payload.Time.Time)

	for _, att := range payload.Media {
		msg.WithAttachment(att)
	}

	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := new(messageStatus)
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	cid, err := strconv.ParseInt(payload.MessageID, 10, 64)

	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(cid), statusMapping[payload.Status])

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	apiUrl, err := getPostofficeEndpoint()

	if err != nil {
		return nil, err
	}

	apiKey, err := getPostofficeAPIKey()

	if err != nil {
		return nil, err
	}

	chatMode := msg.Channel().ConfigForKey("chat_mode", "").(string)
	if chatMode == "" {
		return nil, fmt.Errorf("invalid chat mode")
	}

	deviceId := msg.Channel().ConfigForKey("device_id", "").(string)
	if deviceId == "" {
		return nil, fmt.Errorf("invalid chat mode")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	// Grab contact info from DB
	contact, err := h.Backend().GetContact(ctx, msg.Channel(), msg.URN(), "", "")
	if err != nil {
		return nil, fmt.Errorf("unable to get contact for %s", msg.URN().String())
	}

	// cast to dbContact so we can access the name
	dbContact, ok := contact.(*rapidpro.DBContact)
	if !ok {
		return nil, fmt.Errorf("invalid contact data returned")
	}

	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for _, part := range parts {
		payload := new(outgoingMessage)
		payload.Contact.Name = string(dbContact.Name_)
		payload.Contact.Value = msg.URN().Path()
		payload.Text = part
		payload.Mode = strings.ToUpper(chatMode)
		payload.ChannelID = msg.Channel().UUID().String()
		payload.DeviceID = deviceId
		payload.ID = fmt.Sprintf("%d", msg.ID())

		for _, attachment := range msg.Attachments() {
			_, mediaURL := handlers.SplitAttachment(attachment)

			payload.Media = append(payload.Media, mediaURL)
		}

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		sendURL := fmt.Sprintf("%s%s", apiUrl, outgoingEndpoint)
		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Accept", "application/json")
		req.Header.Set(HEADER_API, apiKey)

		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			logrus.WithField("req.url", req.URL).
				WithField("status", rr.Status).
				WithError(err).Error("PO req fail")
			return status, err
		}
	}

	status.SetStatus(courier.MsgPending)

	return status, nil
}

func (h *handler) PurgeOutgoing(ctx context.Context, channel courier.Channel) error {
	chatMode := channel.ConfigForKey("chat_mode", "").(string)
	if chatMode == "" {
		return fmt.Errorf("invalid chat mode")
	}

	deviceID := channel.ConfigForKey("device_id", "").(string)
	if deviceID == "" {
		return fmt.Errorf("invalid chat mode")
	}

	pr := purgeRequest{
		DeviceID: deviceID,
		Mode:     chatMode,
	}

	apiUrl, err := getPostofficeEndpoint()

	if err != nil {
		return err
	}

	apiKey, err := getPostofficeAPIKey()

	if err != nil {
		return err
	}

	jsonBody, err := json.Marshal(pr)
	if err != nil {
		return err
	}

	sendURL := fmt.Sprintf("%s%s", apiUrl, purgeEndpoint)
	req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(HEADER_API, apiKey)

	rr, err := utils.MakeHTTPRequest(req)

	if err != nil {
		logrus.WithError(err).Error("Could not trigger purge in postoffice")
		return err
	}

	logrus.WithField("response", string(rr.Body)).Info("Purge response from postoffice")

	return nil
}

func getPostofficeEndpoint() (string, error) {
	apiUrl, exists := os.LookupEnv("COURIER_POSTOFFICE_ENDPOINT")

	if !exists {
		return "", fmt.Errorf("Please configure a postoffice endpoint")
	}

	if !strings.HasSuffix(apiUrl, "/") {
		apiUrl = apiUrl + "/"
	}

	return apiUrl, nil
}

func getPostofficeAPIKey() (string, error) {
	apiKey, exists := os.LookupEnv("COURIER_POSTOFFICE_APIKEY")

	if !exists {
		return "", fmt.Errorf("Please configure a postoffice api key")
	}

	return apiKey, nil
}

/*
POST /your_url HTTP/1.1
Content-Type: application/json; charset=utf-8

{
	"time": 1583343305,
	"text": "bla",
	"contact": {
		"name": "Bob",
		"value": "tel:+11234567890"
	},
	"mode": "sms",
	"channel_id": "7cc23772-e933-47b4-b025-19cbaec01edf",
	"media": ["http://example.com/example.jpg"]
}
*/

type incomingMessage struct {
	Time    ISO8601WithMilli `json:"time" validate:"required"`
	Text    string           `json:"text"`
	Contact struct {
		Name  string `json:"name"`
		Value string `json:"value" validate:"required"`
	} `json:"contact" validate:"required"`
	Mode      string `json:"mode" validate:"required"`
	ChannelID string `json:"channel_id" validate:"required"`

	Media []string ` json:"media"`
}

/*
{
	"text": "bla",
	"contact": {
		"name": "Bob",
		"value": "tel:+11234567890"
	},
	"mode": "sms",
	"channel_id": "7cc23772-e933-47b4-b025-19cbaec01edf",
	"device_id": "7cc23773-e933-47b4-b025-19cbaec01edf",
	"id": "32423432432",
	"media": ["http://example.com/example.jpg"]
}
*/
type outgoingMessage struct {
	Text    string `json:"text" validate:"required"`
	Contact struct {
		Name  string `json:"name"`
		Value string `json:"value" validate:"required"`
	} `json:"contact" validate:"required"`
	Mode      string `json:"mode" validate:"required"`
	DeviceID  string `json:"device_id" validate:"required"`
	ChannelID string `json:"channel_id" validate:"required"`

	ID string `json:"id" validate:"required"`

	Media []string ` json:"media"`
}

/*
{
	"message_id": "1234",
	"status": "S"
}
*/
type messageStatus struct {
	MessageID string `json:"message_id" validate:"required"`
	Status    string `json:"status" validate:"required"`
}

/*
{
	"device_id": "123",
	"mode": "SMS"
}
*/
type purgeRequest struct {
	DeviceID string `json:"device_id" validate:"required"`
	Mode     string `json:"mode" validate:"required"`
}
