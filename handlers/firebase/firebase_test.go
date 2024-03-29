package firebase

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	. "github.com/nyaruka/courier/handlers"
)

var (
	receiveURL  = "/c/fcm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/receive"
	validMsg    = "from=12345&date=2017-01-01T08:50:00.000&fcm_token=token&name=fred&msg=hello+world"
	invalidDate = "from=12345&date=yo&fcm_token=token&name=fred&msg=hello+world"
	missingFrom = "date=2017-01-01T08:50:00.000&fcm_token=token&name=fred&msg=hello+world"

	registerURL   = "/c/fcm/8eb23e93-5ecb-45ba-b726-3b064e0c568c/register"
	validRegister = "urn=12345&fcm_token=token&name=fred"
	missingURN    = "fcm_token=token&name=fred"
)

var longMsg = `Lorem ipsum dolor sit amet, consectetur adipiscing elit. Maecenas convallis augue vel placerat congue.
Etiam nec tempus enim. Cras placerat at est vel suscipit. Duis quis faucibus metus, non elementum tortor.
Pellentesque posuere ullamcorper metus auctor venenatis. Proin eget hendrerit dui. Sed eget massa nec mauris consequat pretium.
Praesent mattis arcu tortor, ac aliquet turpis tincidunt eu.

Fusce ut lacinia augue. Vestibulum felis nisi, porta ut est condimentum, condimentum volutpat libero.
Suspendisse a elit venenatis, condimentum sem at, ultricies mauris. Morbi interdum sem id tempor tristique.
Ut tincidunt massa eu purus lacinia sodales a volutpat neque. Cras dolor quam, eleifend a rhoncus quis, sodales nec purus.
Vivamus justo dolor, gravida at quam eu, hendrerit rutrum justo. Sed hendrerit nisi vitae nisl ornare tristique.
Proin vulputate id justo non aliquet.

Duis eu arcu pharetra, laoreet nunc at, pharetra sapien. Nulla eu libero diam.
Donec euismod dapibus ligula, sit amet hendrerit neque vulputate ac.`

var testChannels = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FCM", "1234", "",
		map[string]interface{}{
			configKey:   "FCMKey",
			configTitle: "FCMTitle",
		}),
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "FCM", "1234", "",
		map[string]interface{}{
			configKey:          "FCMKey",
			configNotification: true,
			configTitle:        "FCMTitle",
		}),
}

var testCases = []ChannelHandleTestCase{
	{Label: "Receive Valid Message", URL: receiveURL, Data: validMsg, Status: 200, Response: "Accepted",
		Text: Sp("hello world"), URN: Sp("fcm:12345"), Date: Tp(time.Date(2017, 1, 1, 8, 50, 0, 0, time.UTC)), URNAuth: Sp("token"), Name: Sp("fred")},
	{Label: "Receive Invalid Date", URL: receiveURL, Data: invalidDate, Status: 400, Response: "unable to parse date"},
	{Label: "Receive Missing From", URL: receiveURL, Data: missingFrom, Status: 400, Response: "field 'from' required"},

	{Label: "Receive Valid Register", URL: registerURL, Data: validRegister, Status: 200, Response: "contact_uuid"},
	{Label: "Receive Missing URN", URL: registerURL, Data: missingURN, Status: 400, Response: "field 'urn' required"},
}

func TestHandler(t *testing.T) {
	RunChannelTestCases(t, testChannels, newHandler(), testCases)
}

func BenchmarkHandler(b *testing.B) {
	RunChannelBenchmarks(b, testChannels, newHandler(), testCases)
}

// setSendURL takes care of setting the base_url to our test server host
func setSendURL(s *httptest.Server, h courier.ChannelHandler, c courier.Channel, m courier.Msg) {
	sendURL = s.URL
}

var notificationSendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "fcm:250788123123", URNAuth: "auth1",
		Status: "W", ExternalID: "123456",
		ResponseBody: `{"success":1, "multicast_id": 123456}`, ResponseStatus: 200,
		Headers:     map[string]string{"Authorization": "key=FCMKey"},
		RequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"Simple Message","message_id":10,"session_status":""},"notification":{"title":"FCMTitle","body":"Simple Message"},"content_available":true,"to":"auth1","priority":"high"}`,
		SendPrep:    setSendURL},
}

var sendTestCases = []ChannelSendTestCase{
	{Label: "Plain Send",
		Text: "Simple Message", URN: "fcm:250788123123", URNAuth: "auth1",
		Status: "W", ExternalID: "123456",
		ResponseBody: `{"success":1, "multicast_id": 123456}`, ResponseStatus: 200,
		Headers:     map[string]string{"Authorization": "key=FCMKey"},
		RequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"Simple Message","message_id":10,"session_status":""},"content_available":false,"to":"auth1","priority":"high"}`,
		SendPrep:    setSendURL},
	{Label: "Long Message",
		Text: longMsg,
		URN:  "fcm:250788123123", URNAuth: "auth1",
		Status: "W", ExternalID: "123456",
		ResponseBody: `{"success":1, "multicast_id": 123456}`, ResponseStatus: 200,
		Headers:     map[string]string{"Authorization": "key=FCMKey"},
		RequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"ate ac.","message_id":10,"session_status":""},"content_available":false,"to":"auth1","priority":"high"}`,
		SendPrep:    setSendURL},
	{Label: "Quick Reply",
		Text: "Simple Message", URN: "fcm:250788123123", URNAuth: "auth1", QuickReplies: []string{"yes", "no"}, Attachments: []string{"image/jpeg:https://foo.bar"},
		Status: "W", ExternalID: "123456",
		ResponseBody: `{"success":1, "multicast_id": 123456}`, ResponseStatus: 200,
		Headers:     map[string]string{"Authorization": "key=FCMKey"},
		RequestBody: `{"data":{"type":"rapidpro","title":"FCMTitle","message":"Simple Message\nhttps://foo.bar","message_id":10,"session_status":"","quick_replies":["yes","no"]},"content_available":false,"to":"auth1","priority":"high"}`,
		SendPrep:    setSendURL},
	{Label: "Error",
		Text: "Error", URN: "fcm:250788123123", URNAuth: "auth1",
		Status:       "E",
		ResponseBody: `{ "success": 0 }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "No Multicast ID",
		Text: "Error", URN: "fcm:250788123123", URNAuth: "auth1",
		Status:       "E",
		ResponseBody: `{ "success": 1 }`, ResponseStatus: 200,
		SendPrep: setSendURL},
	{Label: "Request Error",
		Text: "Error", URN: "fcm:250788123123", URNAuth: "auth1",
		Status:       "E",
		ResponseBody: `{ "success": 0 }`, ResponseStatus: 500,
		SendPrep: setSendURL},
}

func TestSending(t *testing.T) {
	RunChannelSendTestCases(t, testChannels[0], newHandler(), sendTestCases, nil)
	RunChannelSendTestCases(t, testChannels[1], newHandler(), notificationSendTestCases, nil)
}
