package bandwidth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	//	"encoding/json"
	//	"fmt"
	"net/http"
	//	"net/url"
	//	"strconv"
	//	"strings"
	//	"time"

	//	"github.com/buger/jsonparser"
	//	"github.com/go-errors/errors"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"

	//	"github.com/nyaruka/courier/utils"
	//	"github.com/nyaruka/gocommon/urns"
	validator "gopkg.in/go-playground/validator.v9"
)

var (
	validate = validator.New()
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
	err := DecodeAndValidateJSONArray(&payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload[0].Message.MessageID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, payload[0].Message.Text)

	return nil, nil
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	// the status that will be written for this message
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	status.SetStatus(courier.MsgWired)

	return status, nil
}

func DecodeAndValidateJSONArray(envelope *[]incomingMessage, r *http.Request) error {
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
