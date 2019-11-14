package bandwidth_international

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"crypto/sha256"

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

var apiURL = "https://sms.a2pi.bandwidth.com:12021/bulk/sendsms"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("BWI"), "Bandwidth International")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {	
	return nil, nil
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	authUsername := msg.Channel().ConfigForKey("username", "")
	if authUsername == "" {
		return nil, fmt.Errorf("invalid auth username")
	}

	authPassword := msg.Channel().ConfigForKey("password", "")
	if authPassword == "" {
		return nil, fmt.Errorf("invalid auth password")
	}

	encoding := msg.Channel().ConfigForKey("encoding", "").(string)
	if encoding == "" {
		return nil, fmt.Errorf("invalid encoding config")
	}

	sender := msg.Channel().Address()
	if sender == "" {
		return nil, fmt.Errorf("invalid phone num config")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for _, part := range parts {
		payload := outgoingMessage{}
		payload.Sender = sender
		payload.Receiver = msg.URN().Path()
		payload.Dsc = encoding
		payload.Text = part
		payload.DlrMask = "0"
		payload.DlrUrl = ""

		payload.Auth.Username = authUsername.(string)
		payload.Auth.Password = authPassword.(string)

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		sendURL := apiURL
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

func Decrypt(rawKey []byte, b64 string) string {
	key := sha256.Sum256(rawKey)
	text, _ := base64.StdEncoding.DecodeString(b64)
	
    block, err := aes.NewCipher(key[:])
    if err != nil {
        panic(err)
    }
    if len(text) < aes.BlockSize {
        panic("cipher text too short")
    }
    iv := text[:aes.BlockSize]
    text = text[aes.BlockSize:]
    cfb := cipher.NewOFB(block, iv)
    cfb.XORKeyStream(text, text)
    return string(text)
}

/*
{
  "type"     : "text",
  "auth"     : {"username":"testuser", "password":"testpassword"},
  "sender"   : "BulkTest",
  "receiver" : "4179123456",
  "dcs"      : "GSM",
  "text"     : "This is test message",
  "dlrMask"  : 19,
  "dlrUrl"   : "http://my-server.com/dlrjson.php"
}
*/

type outgoingMessage struct {
	Type        string   `json:"type"`
	Auth        struct {
		Username    string   `json:"username"`
		Password    string   `json:"password"`
	} `json:"auth"`
	Sender	    string   `json:"sender"`
	Receiver    string   `json:"receiver"`
	Dsc         string   `json:"dcs"`
	Text        string   `json:"text"`
	DlrMask     string   `json:"dlrMask"`
	DlrUrl      string   `json:"dlrUrl"`
}
