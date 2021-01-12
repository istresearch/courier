package courier

import (
	"fmt"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/courier/utils"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

func TestPurgeHandler_PurgeChannel(t *testing.T) {
	logger := logrus.New()
	config := NewConfig()
	backend := NewMockBackend()
	dmChannel := NewMockChannel("e4bb1578-29da-4fa5-a214-9da19dd24230",
		"DM", "123", "US",
		make(map[string]interface{}))
	backend.AddChannel(dmChannel)

	server := NewServerWithLogger(config, backend, logger)
	server.Start()
	defer server.Stop()

	// wait for server to come up
	time.Sleep(100 * time.Millisecond)

	//test no queue
	req, _ := http.NewRequest("POST", "http://localhost:8080/purge/dm/e4bb1578-29da-4fa5-a214-9da19dd24230", nil)
	rr, err := utils.MakeHTTPRequest(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, rr.StatusCode)
	assert.JSONEq(t, `{"message":"Ok","data":null}`, string(rr.Body), "Incorrect response returned")

	rate := 50
	conn := backend.RedisPool().Get()
	defer conn.Close()

	// Queue 20 messages
	for i := 0; i < 20; i++ {
		err := queue.PushOntoQueue(conn, "msgs", "e4bb1578-29da-4fa5-a214-9da19dd24230", rate, fmt.Sprintf(`[{"id":%d}]`, i), queue.LowPriority)
		assert.NoError(t, err)
	}

	// Ensure all 20 were added
	cnt, err := conn.Do("ZCOUNT", "msgs:e4bb1578-29da-4fa5-a214-9da19dd24230|50/0", "-inf", "+inf")
	assert.NoError(t, err)
	assert.Equal(t, int64(20), cnt)

	//test no queue
	req, _ = http.NewRequest("POST", "http://localhost:8080/purge/dm/e4bb1578-29da-4fa5-a214-9da19dd24230", nil)
	rr, err = utils.MakeHTTPRequest(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, rr.StatusCode)
	assert.JSONEq(t, `{"message":"Ok","data":null}`, string(rr.Body), "Incorrect response returned")
}