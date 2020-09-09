package courier

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"net/http"
)

type PurgeHandler struct {
	server Server
}

func NewPurgeHandler(s Server) *PurgeHandler{
	p := new(PurgeHandler)
	p.server = s

	return p
}

func (p *PurgeHandler) PurgeChannel(w http.ResponseWriter, r *http.Request) {
	//uuid, err := NewChannelUUID("b2a8151e-5d21-42e6-8077-3abcf9e38173")
	uuid, err := NewChannelUUID("b2a8151e-5d21-42e6-8077-3abcf9e38173")

	if err != nil {
		logrus.Error("Invalid channel ID provided")
		return
	}

	queues, err := p.server.Backend().GetCurrentQueuesForChannel(context.Background(), uuid)

	if err != nil {
		logrus.Error(err)
	} else if len(queues) == 0 {
		logrus.Info("No queues found")
	}

	p.PurgeRoutine(queues)
}

func (p *PurgeHandler) PurgeRoutine(queueKeys []string) {
	rc := p.server.Backend().RedisPool().Get()
	defer rc.Close()

	// Iterate throuhg each queue for the channel, then iterate messages
	for _, v := range queueKeys {
		logrus.Info(v)

		hasMsg := true
		// Iterate through messages until we're out of them.
		for hasMsg == true {
			msgs, _ := p.server.Backend().PopMsgs(context.Background(), v, 1)

			if len(msgs) == 0 {
				logrus.Debug("out of messages")
				hasMsg = false
				break
			}

			for _, msg := range msgs {
				status := p.server.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), MsgFailed)
				status.AddLog(NewChannelLogFromError("Queue Purge", msg.Channel(), msg.ID(), 0,
					fmt.Errorf("failing message due to purge")))

				err := p.server.Backend().WriteMsgStatus(context.Background(), status)
				if err != nil {
					logrus.WithError(err).Info("error writing msg status")
				} else {
					logrus.WithField("msg", msg.ID()).Info("Failing message due to queue purge")
				}
			}
		}
	}
}