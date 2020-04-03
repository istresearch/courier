package postmaster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"

	validator "gopkg.in/go-playground/validator.v9"
)

var (
	maxMsgLength = 2048
	validate     = validator.New()
)

var statusMapping = map[string]courier.MsgStatusValue{
	"S":      courier.MsgSent,
	"E":      courier.MsgErrored,
	"D":        courier.MsgDelivered,
	"F": courier.MsgFailed,
}

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("PSM"), "Postmaster")}
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
	payload := &incomingMessage{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	var urn urns.URN
	urn, err = urns.NewURNFromParts(urns.TelScheme, payload.Contact.Urn, "", "")

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Text)

	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &messageStatus{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	cid, err := strconv.ParseInt(payload.MessageID, 10, 64)

	status := h.Backend().NewMsgStatusForID(channel, courier.NewMsgID(cid), statusMapping[payload.Status])

	return handlers.WriteMsgStatusAndResponse(ctx, h, channel, status, w, r)
}


	func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	apiUrl,err := getPostofficeEndpoint()

	if err != nil {
		return nil, err
	}

	chatMode := msg.Channel().ConfigForKey("chat_mode", "").(string)
	if chatMode == "" {
		return nil, fmt.Errorf("invalid chat mode")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for _, part := range parts {
		payload := outgoingMessage{}
		payload.Contact.Name = msg.ContactName()
		payload.Contact.Urn = msg.URN().Path()
		payload.Text = part
		payload.Mode = chatMode
		payload.ChannelID = msg.Channel().UUID().String()
		payload.DeviceID = "123"
		payload.ID = fmt.Sprintf("%d",msg.ID())

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		sendURL := fmt.Sprintf("%s/api/message/send", apiUrl)
		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Accept", "application/json")

		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}
	}

	status.SetStatus(courier.MsgWired)

	return status, nil
}

func getPostofficeEndpoint() (string, error) {
	apiUrl, exists := os.LookupEnv("COURIER_POSTOFFICE_ENDPOINT")

	if !exists {
		return "", fmt.Errorf("Please configure a postoffice endpoint")
	}

	return apiUrl, nil
}

/*
POST /your_url HTTP/1.1
Content-Type: application/json; charset=utf-8

{
	"time": 1583343305,
	"text": "bla",
	"contact": {
		"name": "Bob",
		"urn": "tel:+11234567890"
	},
	"mode": "sms",
	"channel_id": "7cc23772-e933-47b4-b025-19cbaec01edf",
	"media": ["http://example.com/example.jpg"]
}
*/

type incomingMessage struct {
	Time      int    `json:"time" validate:"required"`
	Text      string `json:"text" validate:"required"`
	Contact struct {
		Name string `json:"name"`
		Urn  string `json:"urn" validate:"required"`
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
		"urn": "tel:+11234567890"
	},
	"mode": "sms",
	"channel_id": "7cc23772-e933-47b4-b025-19cbaec01edf",
	"device_id": "7cc23773-e933-47b4-b025-19cbaec01edf",
	"id": "32423432432",
	"media": ["http://example.com/example.jpg"]
}
*/
type outgoingMessage struct {
	Text      string `json:"text" validate:"required"`
	Contact struct {
		Name string `json:"name"`
		Urn  string `json:"urn" validate:"required"`
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
	Status string `json:"status" validate:"required"`
}