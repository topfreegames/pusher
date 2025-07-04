package firebase

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	pushErrors "github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

type messageHandler struct {
	app                        string
	logger                     *logrus.Logger
	client                     interfaces.PushClient
	config                     messageHandlerConfig
	feedbackReporters          []interfaces.FeedbackReporter
	statsReporters             []interfaces.StatsReporter
	pendingMessagesWaitGroup   *sync.WaitGroup
	rateLimiter                interfaces.RateLimiter
	dedup                      interfaces.Dedup
	statsDClient               extensions.StatsD
	sendPushConcurrencyControl chan interface{}
	responsesChannel           chan struct {
		msg   interfaces.Message
		error error
	}
}

type kafkaFCMMessage struct {
	interfaces.Message
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	PushExpiry int64                  `json:"push_expiry,omitempty"`
}

var _ interfaces.MessageHandler = &messageHandler{}

func NewMessageHandler(
	app string,
	client interfaces.PushClient,
	feedbackReporters []interfaces.FeedbackReporter,
	statsReporters []interfaces.StatsReporter,
	rateLimiter interfaces.RateLimiter,
	dedup interfaces.Dedup,
	pendingMessagesWaitGroup *sync.WaitGroup,
	logger *logrus.Logger,
	concurrentWorkers int,
) interfaces.MessageHandler {
	l := logger.WithFields(logrus.Fields{
		"app":    app,
		"source": "messageHandler",
	})
	cfg := newDefaultMessageHandlerConfig()
	cfg.concurrentResponseHandlers = concurrentWorkers

	h := &messageHandler{
		app:                        app,
		client:                     client,
		feedbackReporters:          feedbackReporters,
		statsReporters:             statsReporters,
		rateLimiter:                rateLimiter,
		dedup:                      dedup,
		pendingMessagesWaitGroup:   pendingMessagesWaitGroup,
		logger:                     l.Logger,
		config:                     cfg,
		sendPushConcurrencyControl: make(chan interface{}, concurrentWorkers),
		responsesChannel: make(chan struct {
			msg   interfaces.Message
			error error
		}, concurrentWorkers),
	}

	for i := 0; i < concurrentWorkers; i++ {
		h.sendPushConcurrencyControl <- struct{}{}
	}

	return h
}

func (h *messageHandler) HandleMessages(ctx context.Context, msg interfaces.KafkaMessage) {
	l := h.logger.WithFields(logrus.Fields{
		"method": "HandleMessages",
	})
	km := kafkaFCMMessage{}
	err := json.Unmarshal(msg.Value, &km)
	if err != nil {
		l.WithError(err).Error("Error unmarshalling message.")
		h.waitGroupDone()
		return
	}

	if km.PushExpiry > 0 && km.PushExpiry < extensions.MakeTimestamp() {
		l.Warnf("ignoring push message because it has expired: %s", km.Data)
		h.waitGroupDone()
		return
	}

	// if there is any error on deduplication, it does not block the message.
	dedupMsg, err := h.createDedupContentFromPayload(km)
	if err == nil {
		uniqueMessage := h.dedup.IsUnique(ctx, km.To, dedupMsg, h.app, "gcm")
		if !uniqueMessage {
			l.WithFields(logrus.Fields{
				"extension": "dedup",
				"game":      h.app,
			}).Info("duplicate message detected")
			extensions.StatsReporterDuplicateMessageDetected(h.statsReporters, h.app, "gcm")
			//does not return because we don't want to block the message
		}
	} else {
		l.WithError(err).Error("error creating deduplication content from payload")
	}

	allowed := h.rateLimiter.Allow(ctx, km.To, msg.Game, "gcm")
	if !allowed {
		h.reportRateLimitReached(msg.Game)
		h.waitGroupDone()
		l.WithField("message", msg).Warn("rate limit reached")
		return
	}

	if km.Metadata != nil {
		if km.Message.Data == nil {
			km.Message.Data = map[string]interface{}{}
		}

		for k, v := range km.Metadata {
			if km.Message.Data[k] == nil {
				km.Message.Data[k] = v
			}
		}
	}
	before := time.Now()
	defer h.reportLatency(time.Since(before))
	h.sendPush(ctx, km.Message, msg.Topic)
}

func (h *messageHandler) createDedupContentFromPayload(km kafkaFCMMessage) (string, error) {
	contentData := make(map[string]interface{})

	if km.Data != nil {
		contentData["data"] = km.Data
	}

	if km.Notification != nil {
		contentData["notification"] = km.Notification
	}

	contentJSON, err := json.Marshal(contentData)
	if err != nil {
		h.logger.WithError(err).Error("Error marshalling content data for deduplication")
		return "", err
	}
	return string(contentJSON), nil
}

func (h *messageHandler) sendPush(ctx context.Context, msg interfaces.Message, topic string) {
	lock := <-h.sendPushConcurrencyControl

	go func(l interface{}) {
		defer func() {
			h.sendPushConcurrencyControl <- l
		}()

		before := time.Now()
		err := h.client.SendPush(ctx, msg)
		h.reportFirebaseLatency(time.Since(before))

		h.handleNotificationSent(topic)

		h.responsesChannel <- struct {
			msg   interfaces.Message
			error error
		}{
			msg:   msg,
			error: err,
		}
	}(lock)
}

// HandleResponses was needed as a callback to handle the responses from them in APNS and the legacy GCM.
// Here the responses are handled asynchronously. The method is kept to comply with the interface.
func (h *messageHandler) HandleResponses() {
	for i := 0; i < h.config.concurrentResponseHandlers; i++ {
		go func() {
			for {
				response := <-h.responsesChannel
				if response.error != nil {
					h.handleNotificationFailure(response.msg, response.error)
				} else {
					h.handleNotificationAck()
				}
				h.waitGroupDone()
			}
		}()
	}
}

func (h *messageHandler) sendToFeedbackReporters(res interface{}) error {
	jsonRes, err := json.Marshal(res)
	if err != nil {
		return err
	}

	for _, feedbackReporter := range h.feedbackReporters {
		feedbackReporter.SendFeedback(h.app, "gcm", jsonRes)
	}

	return nil
}

func (h *messageHandler) handleNotificationSent(topic string) {
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationSent(h.app, "gcm", topic)
	}
}

func (h *messageHandler) handleNotificationAck() {
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationSuccess(h.app, "gcm")
	}
}

func (h *messageHandler) handleNotificationFailure(message interfaces.Message, err error) {
	pushError := translateToPushError(err)
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationFailure(h.app, "gcm", pushError)
	}
	for _, feedbackReporter := range h.feedbackReporters {
		feedback := &FeedbackResponse{
			Error:            pushError.Key,
			ErrorDescription: pushError.Description,
			From:             message.To,
		}
		b, _ := json.Marshal(feedback)
		feedbackReporter.SendFeedback(h.app, "gcm", b)
	}
}

func (h *messageHandler) reportLatency(latency time.Duration) {
	for _, statsReporter := range h.statsReporters {
		statsReporter.ReportSendNotificationLatency(latency, h.app, "gcm", "client", "fcm")
	}
}

func (h *messageHandler) reportFirebaseLatency(latency time.Duration) {
	for _, statsReporter := range h.statsReporters {
		statsReporter.ReportFirebaseLatency(latency, h.app)
	}
}

func (h *messageHandler) reportRateLimitReached(game string) {
	for _, statsReporter := range h.statsReporters {
		statsReporter.NotificationRateLimitReached(game, "gcm")
	}
}

func translateToPushError(err error) *pushErrors.PushError {
	if pusherError, ok := err.(*pushErrors.PushError); ok {
		return pusherError
	}
	return pushErrors.NewPushError("unknown", err.Error())
}

func (h *messageHandler) waitGroupDone() {
	if h.pendingMessagesWaitGroup != nil {
		h.pendingMessagesWaitGroup.Done()
	}
}
