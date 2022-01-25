package bandwidth_international

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/gsm7"

	validator "gopkg.in/go-playground/validator.v9"
)

var (
	maxMsgLength = 2048
	validate     = validator.New()
)

var apiURL = "https://bulksms.ia2p.bandwidth.com:12021/bulk/sendsms"

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

	if encoding == "auto" {
		if gsm7.IsValid(msg.Text()) {
			encoding = "gsm"
		} else {
			encoding = "ucs"
		}
	}

	sender := msg.Channel().Address()
	if sender == "" {
		return nil, fmt.Errorf("invalid channel sender")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	callbackDomain := msg.Channel().CallbackDomain(h.Server().Config().Domain)

	parts := handlers.SplitMsg(msg.Text(), maxMsgLength)
	for _, part := range parts {
		payload := outgoingMessage{}
		payload.Type = "text"
		payload.Sender = sender
		payload.Receiver = strings.Trim(msg.URN().Path(),"+")
		payload.Dsc = strings.ToUpper(encoding)
		payload.Text = part
		payload.DlrMask = 0
		payload.DlrUrl = callbackDomain
	
		username, err := decrypt(authUsername.(string))

		if err != nil {
			return status, err
		}

		password, err := decrypt(authPassword.(string))

		if err != nil {
			return status, err
		}

		payload.Auth.Username = username
		payload.Auth.Password = password

		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return status, err
		}

		sendURL := apiURL
		req, _ := http.NewRequest(http.MethodPost, sendURL, bytes.NewReader(jsonBody))
		req.Header.Add("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Accept", "application/json")

		rr, err := utils.MakeHTTPRequest(req)

		rr.Request = ""

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

func decrypt(b64 string) (string, error) {
	rawKey, exists := os.LookupEnv("COURIER_BWI_KEY")

	if !exists {
		return "", fmt.Errorf("Please configure a BWI encryption key")
	}

    key := sha256.Sum256([]byte(rawKey))
	text, _ := base64.StdEncoding.DecodeString(b64)
	
    block, err := aes.NewCipher(key[:])
    if err != nil {
        panic(err)
    }
    if len(text) < aes.BlockSize {
        panic("ciphertext too short")
    }
    iv := text[:aes.BlockSize]
    text = text[aes.BlockSize:]
    cfb := cipher.NewOFB(block, iv)
	cfb.XORKeyStream(text, text)
	
	clearText := string(text)
    clearText = clearText[:len(clearText) - int(clearText[len(clearText)-1])]

    return clearText, nil
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
	DlrMask     int   `json:"dlrMask"`
	DlrUrl      string   `json:"dlrUrl"`
}
