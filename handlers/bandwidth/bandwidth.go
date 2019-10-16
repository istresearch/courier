package bandwidth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/go-errors/errors"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
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
	payload := &incomingMessage{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.Message.MessageID == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Yahoo")
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
	Type string `json:"type" validate:"required"`
	Time string `json:"time"`
	Description string `json:"description"`
	To string `json:"to"`
	Message  struct {
		MessageID string `json:"id"`
		Time string `json:"time"`
		To string `json:"to"`
		From string `json:"from"`
		Text string `json:"text"`
		Application string `json:"applicationId"`
		Owner string `json:"owner"`
		Direction string `json:"direction"`
      	SegmentCount string `json:"segmentCount"`
	} `json:"message"`
}
