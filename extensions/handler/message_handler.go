package handler

import (
	"context"
	"encoding/json"
	"github.com/sirupsen/logrus"
	pushErrors "github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"sync"
	"time"
)

type messageHandler struct {
	app               string
	logger            *logrus.Logger
	client            interfaces.PushClient
	config            messageHandlerConfig
	stats             messagesStats
	statsMutex        sync.Mutex
	feedbackReporters []interfaces.FeedbackReporter
	statsReporters    []interfaces.StatsReporter
}

var _ interfaces.MessageHandler = &messageHandler{}

func NewMessageHandler(
	app string,
	client interfaces.PushClient,
	feedbackReporters []interfaces.FeedbackReporter,
	statsReporters []interfaces.StatsReporter,
	logger *logrus.Logger,
) interfaces.MessageHandler {
	l := logger.WithFields(logrus.Fields{
		"app":    app,
		"source": "messageHandler",
	})
	return &messageHandler{
		app:               app,
		client:            client,
		feedbackReporters: feedbackReporters,
		statsReporters:    statsReporters,
		logger:            l.Logger,
		config:            newDefaultMessageHandlerConfig(),
	}
}

func (h *messageHandler) HandleMessages(ctx context.Context, msg interfaces.KafkaMessage) {
	l := h.logger.WithFields(logrus.Fields{
		"method": "HandleMessages",
	})
	km := extensions.KafkaGCMMessage{}
	err := json.Unmarshal(msg.Value, &km)
	if err != nil {
		l.WithError(err).Error("Error unmarshaling message.")
		return
	}

	if km.PushExpiry > 0 && km.PushExpiry < extensions.MakeTimestamp() {
		l.Warnf("ignoring push message because it has expired: %s", km.Data)

		h.statsMutex.Lock()
		h.stats.ignored++
		h.statsMutex.Unlock()

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

	err = h.client.SendPush(ctx, km.Message)
	if err != nil {
		l.WithError(err).Error("Error sending push message.")
		h.statsMutex.Lock()
		h.stats.failures++
		h.statsMutex.Unlock()

		h.statsReporterHandleNotificationFailure(err)
		return
	}

	h.statsReporterHandleNotificationSent()

	h.statsMutex.Lock()
	h.stats.sent++
	h.statsMutex.Unlock()

}

func (h *messageHandler) HandleResponses() {
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

func (h *messageHandler) statsReporterHandleNotificationSent() {
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationSent(h.app, "gcm")
		statsReporter.HandleNotificationSuccess(h.app, "gcm")
	}
}

func (h *messageHandler) statsReporterHandleNotificationFailure(err error) {
	pushError := translateToPushError(err)
	for _, statsReporter := range h.statsReporters {
		statsReporter.HandleNotificationFailure(h.app, "gcm", pushError)
	}
}

func translateToPushError(err error) *pushErrors.PushError {
	if pusherError, ok := err.(*pushErrors.PushError); ok {
		return pusherError

	}
	return pushErrors.NewPushError("unknown", err.Error())
}