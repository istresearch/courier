package courier

import (
	"context"
	"fmt"
	"github.com/go-chi/chi"
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
	uuid, err := NewChannelUUID(chi.URLParam(r, "uuid"))

	if err != nil || len(uuid.String()) == 0 {
		logrus.Error("Invalid channel ID provided")
		WriteDataResponse(context.Background(), w, http.StatusBadRequest, "invalid channel ID", nil)
		return
	}

	queues, err := p.server.Backend().GetCurrentQueuesForChannel(context.Background(), uuid)

	if err != nil {
		logrus.Error(err)
		WriteDataResponse(context.Background(), w, http.StatusInternalServerError, "Error while fetching queues", nil)
		return
	} else if len(queues) == 0 {
		logrus.Info("No queues found")
		WriteDataResponse(context.Background(), w, http.StatusOK, "No outgoing queues found", nil)
		return
	}

	go p.PurgeRoutine(queues)

	WriteDataResponse(context.Background(), w, http.StatusOK, "Ok", nil)
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
			msgs, _ := p.server.Backend().PopMsgs(context.Background(), v, 10)

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