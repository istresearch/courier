package postmaster

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab",
		"PSM", "123", "US",
		map[string]interface{}{"device_id": "123", "chat_mode": "SMS"}),
}

var (
	receiveURL = "/c/psm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/receive/"

	statusURL = "/c/psm/8eb23e93-5ecb-45ba-b726-3b064e0c56ab/status/"
)

var acceptedMessage = `
{
	"time": "2006-01-02T15:04:05.000Z",
	"text": "bla",
	"contact": {
		"name": "Bob",
		"urn": "+11234567890"
	},
	"mode": "sms",
	"channel_id": "08ecc21a-8098-4ddb-a090-eca7ee97f65d",
	"media": ["http://example.com/example.jpg"]
}
`

var acceptedStatus = `
{
	"message_id": "1234",
	"status": "S"
}
`

var testCases = []ChannelHandleTestCase{
	{Label: "Accepted", URL: receiveURL, Data: acceptedMessage, Status: 200, Response: "Accepted"},
	{Label: "Receive Invalid JSON", URL: receiveURL, Data: "{blabla}", Status: 400, Response: "unable to parse"},

	{Label: "Accepted Status", URL: statusURL, Data: acceptedStatus, Status: 200, Response: "Accepted"},
	{Label: "Receive Invalid Status JSON", URL: statusURL, Data: "{blabla}", Status: 400, Response: "unable to parse"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	os.Setenv("COURIER_POSTOFFICE_ENDPOINT", s.URL)
	os.Setenv("COURIER_POSTOFFICE_APIKEY", "abc123")
}

var defaultSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message ☺", URN: "tel:+11234567890", Attachments: []string{"image/jpeg:https://foo.bar/image.jpg"},
		Status: "W",
		ResponseBody: `{"ack":"ok","error":"","code":200}`, ResponseStatus: 200,
		RequestBody: `{"text":"Simple Message ☺","contact":{"name":"","urn":"+11234567890"},"mode":"SMS","device_id":"123","channel_id":"8eb23e93-5ecb-45ba-b726-3b064e0c56ab","id":"10","media":["https://foo.bar/image.jpg"]}`,

		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	var defaultChannel = courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "PSM", "2020", "US",
		map[string]interface{}{
			"chat_mode": "SMS",
			"device_id": "123",
		})
	RunChannelSendTestCases(t, defaultChannel, newHandler(), defaultSendTestCases, nil)
}
