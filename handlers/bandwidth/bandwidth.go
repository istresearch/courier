package bandwidth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

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

var apiURL = "https://messaging.bandwidth.com"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("BWD"), "Bandwidth")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {	
	var payload []incomingMessage
	err := DecodeAndValidateBWPayload(&payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload[0].Message.MessageID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	if payload[0].Type != "message-received" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring non received request, no message")
	}

	var urn urns.URN
	urn, err = urns.NewURNFromParts(urns.TelScheme, payload[0].Message.From, "", "")

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	real_channel, err := h.Server().Backend().GetChannelByAddress(ctx, courier.ChannelType("BWD"), courier.ChannelAddress(payload[0].To))

	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	msg := h.Backend().NewIncomingMsg(real_channel, urn, payload[0].Message.Text).WithExternalID(payload[0].Message.MessageID)

	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	authToken := msg.Channel().ConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return nil, fmt.Errorf("invalid auth token config")
	}

	authSecret := msg.Channel().ConfigForKey(courier.ConfigSecret, "")
	if authSecret == "" {
		return nil, fmt.Errorf("invalid auth secret config")
	}

	accountId := msg.Channel().ConfigForKey("account_sid", "")
	if accountId == "" {
		return nil, fmt.Errorf("invalid account ID config")
	}

	applicationID := msg.Channel().ConfigForKey("application_sid", "")
	if applicationID == "" {
		return nil, fmt.Errorf("invalid application ID config")
	}

	phoneNum := msg.Channel().Address()
	if phoneNum == "" {
		return nil, fmt.Errorf("invalid phone num config")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for _, part := range parts {
		payload := outgoingMessage{}
		payload.To = append(payload.To, msg.URN().Path())
		payload.From = phoneNum
		payload.Text = part
		payload.Application = applicationID.(string)

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		sendURL := fmt.Sprintf("%s/api/v2/users/%s/messages", apiURL, accountId)
		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(authToken.(string), authSecret.(string))

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

func DecodeAndValidateBWPayload(envelope *[]incomingMessage, r *http.Request) error {
	// read our body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 100000))
	defer r.Body.Close()
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	// try to decode our envelope
	if err = json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("unable to parse request JSON: %s", err)
	}

	callback := (*envelope)[0]

	// check our input is valid
	err = validate.Struct(callback)
	if err != nil {
		return fmt.Errorf("request JSON doesn't match required schema: %s", err)
	}

	return nil
}

/*
POST /your_url HTTP/1.1
Content-Type: application/json; charset=utf-8
User-Agent: BandwidthAPI/v2

[
  {
    "type"        : "message-received",
    "time"        : "2016-09-14T18:20:16Z",
    "description" : "Incoming message received",
    "to"          : "+12345678902",
    "message"     : {
      "id"            : "14762070468292kw2fuqty55yp2b2",
      "time"          : "2016-09-14T18:20:16Z",
      "to"            : ["+12345678902"],
      "from"          : "+12345678901",
      "text"          : "Hey, check this out!",
      "applicationId" : "93de2206-9669-4e07-948d-329f4b722ee2",
      "media"         : [
        "https://messaging.bandwidth.com/api/v2/users/{accountId}/media/14762070468292kw2fuqty55yp2b2/0/bw.png"
        ],
      "owner"         : "+12345678902",
      "direction"     : "in",
      "segmentCount"  : 1
    }
  }
]
*/

type incomingMessage struct {
	Type        string `json:"type" validate:"required"`
	Time        string `json:"time"`
	Description string `json:"description"`
	To          string `json:"to"`
	Message     struct {
		MessageID    string   `json:"id"`
		Time         string   `json:"time"`
		To           []string `json:"to"`
		From         string   `json:"from"`
		Text         string   `json:"text"`
		Application  string   `json:"applicationId"`
		Owner        string   `json:"owner"`
		Direction    string   `json:"direction"`
		SegmentCount int      `json:"segmentCount"`
	} `json:"message"`
}

type outgoingMessage struct {
	To          []string `json:"to"`
	From        string   `json:"from"`
	Text        string   `json:"text"`
	Application string   `json:"applicationId"`
	Tag         string   `json:"tag"`
}
