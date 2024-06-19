package handler

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
	stats                      messagesStats
	statsMutex                 sync.Mutex
	feedbackReporters          []interfaces.FeedbackReporter
	statsReporters             []interfaces.StatsReporter
	rateLimiter                interfaces.RateLimiter
	statsDClient               extensions.StatsD
	sendPushConcurrencyControl chan interface{}
	responsesChannel           chan struct {
		msg   interfaces.Message
		error error
	}
}

var _ interfaces.MessageHandler = &messageHandler{}

func NewMessageHandler(
	app string,
	client interfaces.PushClient,
	feedbackReporters []interfaces.FeedbackReporter,
	statsReporters []interfaces.StatsReporter,
	rateLimiter interfaces.RateLimiter,
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
	km := extensions.KafkaGCMMessage{}
	err := json.Unmarshal(msg.Value, &km)
	if err != nil {
		l.WithError(err).Error("Error unmarshalling message.")
		return
	}

	if km.PushExpiry > 0 && km.PushExpiry < extensions.MakeTimestamp() {
		l.Warnf("ignoring push message because it has expired: %s", km.Data)

		h.statsMutex.Lock()
		h.stats.ignored++
		h.statsMutex.Unlock()

		return
	}

	allowed := h.rateLimiter.Allow(ctx, km.To, msg.Game, "gcm")
	if !allowed {
		h.reportRateLimitReached(msg.Game)
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
	h.sendPush(ctx, km.Message)
}

func (h *messageHandler) sendPush(ctx context.Context, msg interfaces.Message) {
	lock := <-h.sendPushConcurrencyControl

	go func(l interface{}) {
		defer func() {
			h.sendPushConcurrencyControl <- l
		}()

		before := time.Now()
		err := h.client.SendPush(ctx, msg)
		h.reportFirebaseLatency(time.Since(before))

		h.handleNotificationSent()

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
					h.handleNotificationFailure(response.error)
				} else {
					h.handleNotificationAck()
				}
			}
		}()
	}
}

func (h *messageHandler) LogStats() {
	l := h.logger.WithFields(logrus.Fields{
		"method":       "logStats",
		"interval(ns)": h.config.statusLogInterval.Nanoseconds(),
	})

	ticker := time.NewTicker(h.config.statusLogInterval)
	for range ticker.C {
		h.statsMutex.Lock()
		if h.stats.sent > 0 || h.stats.ignored > 0 || h.stats.failures > 0 {
			l.WithFields(logrus.Fields{
				"sentMessages":     h.stats.sent,
				"ignoredMessages":  h.stats.ignored,
				"failuresReceived": h.stats.failures,
			}).Info("flushing stats")

			h.stats.sent = 0
			h.stats.ignored = 0
			h.stats.failures = 0
		}
		h.statsMutex.Unlock()
	}
}

func (h *messageHandler) CleanMetadataCache() {
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

func (h *messageHandler) handleNotificationSent() {
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationSent(h.app, "gcm")
	}
}

func (h *messageHandler) handleNotificationAck() {
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationSuccess(h.app, "gcm")
	}

	for _, feedbackReporter := range h.feedbackReporters {
		r := &FeedbackResponse{}
		b, _ := json.Marshal(r)
		feedbackReporter.SendFeedback(h.app, "gcm", b)
	}

	h.statsMutex.Lock()
	h.stats.sent++
	h.statsMutex.Unlock()
}

func (h *messageHandler) handleNotificationFailure(err error) {
	pushError := translateToPushError(err)
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationFailure(h.app, "gcm", pushError)
	}
	for _, feedbackReporter := range h.feedbackReporters {
		feedback := &FeedbackResponse{
			Error:            pushError.Key,
			ErrorDescription: pushError.Description,
		}
		b, _ := json.Marshal(feedback)
		feedbackReporter.SendFeedback(h.app, "gcm", b)
	}
	h.statsMutex.Lock()
	h.stats.failures++
	h.statsMutex.Unlock()
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
