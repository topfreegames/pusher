/*
 * Copyright (c) 2016 TFG Co <backend@tfgco.com>
 * Author: TFG Co <backend@tfgco.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package apns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/topfreegames/pusher/extensions"

	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	pusher_errors "github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/structs"
)

// pusherAPNSKafkaMessage is the notification format received in Kafka messages.
type pusherAPNSKafkaMessage struct {
	ApnsID      string
	DeviceToken string
	Payload     interface{}
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	PushExpiry  int64                  `json:"push_expiry,omitempty"`
	CollapseID  string                 `json:"collapse_id,omitempty"`
}

// APNSMessageHandler implements the messagehandler interface.
type APNSMessageHandler struct {
	feedbackReporters            []interfaces.FeedbackReporter
	StatsReporters               []interfaces.StatsReporter
	authKeyPath                  string
	keyID                        string
	teamID                       string
	appName                      string
	PushQueue                    interfaces.APNSPushQueue
	ApnsTopic                    string
	Config                       *viper.Viper
	failuresReceived             int64
	Logger                       *log.Logger
	LogStatsInterval             time.Duration
	pendingMessagesWG            *sync.WaitGroup
	inFlightNotificationsMapLock *sync.Mutex
	responsesReceived            int64
	sentMessages                 int64
	ignoredMessages              int64
	successesReceived            int64
	CacheCleaningInterval        int
	IsProduction                 bool
	retryInterval                time.Duration
	maxRetryAttempts             uint
	rateLimiter                  interfaces.RateLimiter
	dedup                        interfaces.Dedup
}

var _ interfaces.MessageHandler = &APNSMessageHandler{}

// NewAPNSMessageHandler returns a new instance of a APNSMessageHandler.
func NewAPNSMessageHandler(
	authKeyPath, keyID, teamID, topic, appName string,
	isProduction bool,
	config *viper.Viper,
	logger *log.Logger,
	pendingMessagesWG *sync.WaitGroup,
	statsReporters []interfaces.StatsReporter,
	feedbackReporters []interfaces.FeedbackReporter,
	pushQueue interfaces.APNSPushQueue,
	rateLimiter interfaces.RateLimiter,
	dedup interfaces.Dedup,
) (*APNSMessageHandler, error) {
	a := &APNSMessageHandler{
		authKeyPath:       authKeyPath,
		keyID:             keyID,
		teamID:            teamID,
		ApnsTopic:         topic,
		appName:           appName,
		Config:            config,
		feedbackReporters: feedbackReporters,
		IsProduction:      isProduction,
		Logger:            logger,
		pendingMessagesWG: pendingMessagesWG,
		StatsReporters:    statsReporters,
		PushQueue:         pushQueue,
		rateLimiter:       rateLimiter,
		dedup:             dedup,
	}

	if a.Logger != nil {
		a.Logger = a.Logger.WithFields(log.Fields{
			"source": "APNSMessageHandler",
			"game":   appName,
		}).Logger
	}

	if err := a.configure(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *APNSMessageHandler) configure() error {
	a.loadConfigurationDefaults()
	interval := a.Config.GetInt("apns.logStatsInterval")
	a.LogStatsInterval = time.Duration(interval) * time.Millisecond
	a.CacheCleaningInterval = a.Config.GetInt("feedback.cache.cleaningInterval")
	a.retryInterval = a.Config.GetDuration("apns.retry.interval")
	a.maxRetryAttempts = a.Config.GetUint("apns.retry.maxRetryAttempts")
	if a.PushQueue == nil {
		a.PushQueue = NewAPNSPushQueue(
			a.authKeyPath,
			a.keyID,
			a.teamID,
			a.IsProduction,
			a.Logger,
			a.Config,
		)
		err := a.PushQueue.Configure()
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *APNSMessageHandler) loadConfigurationDefaults() {
	a.Config.SetDefault("apns.concurrentWorkers", 10)
	a.Config.SetDefault("apns.logStatsInterval", 5000)
	a.Config.SetDefault("feedback.cache.cleaningInterval", 300000)
	a.Config.SetDefault("apns.retry.interval", "1s")
	a.Config.SetDefault("apns.retry.maxRetryAttempts", 3)
}

// HandleResponses from apns.
func (a *APNSMessageHandler) HandleResponses() {
	for response := range a.PushQueue.ResponseChannel() {
		err := a.handleAPNSResponse(response)
		if err != nil {
			a.Logger.
				WithField("method", "HandleResponses").
				WithError(err).
				Error("error handling response")
		}
	}
}

// CleanMetadataCache clears expired requests from memory.
// No longer needed, kept for compatibility with the interface.
func (a *APNSMessageHandler) CleanMetadataCache() {
}

// HandleMessages get messages from msgChan and send to APNS.
func (a *APNSMessageHandler) HandleMessages(ctx context.Context, message interfaces.KafkaMessage) {
	l := a.Logger.WithFields(log.Fields{
		"method":    "HandleMessages",
		"jsonValue": string(message.Value),
		"topic":     message.Topic,
	})
	l.Debug("received message to send to apns")

	parsedNotification, err := a.parseKafkaMessage(message)
	if err != nil {
		l.WithError(err).Error("error parsing kafka message")
		a.waitGroupDone()
		return
	}
	l = l.WithField("notification", parsedNotification)

	dedupMsg := a.createDedupContentFromPayload(parsedNotification)
	uniqueMessage := a.dedup.IsUnique(ctx, parsedNotification.DeviceToken, dedupMsg, a.appName, "apns")
	if !uniqueMessage {
		l.WithFields(log.Fields{
			"extension": "dedup",
			"game":     a.appName,
		}).Debug("duplicate message detected")
		extensions.StatsReporterDuplicateMessageDetected(a.StatsReporters, a.appName, "apns")
		//does not return because we don't want to block the message
	}

	allowed := a.rateLimiter.Allow(ctx, parsedNotification.DeviceToken, a.appName, "apns")
	if !allowed {
		extensions.StatsReporterNotificationRateLimitReached(a.StatsReporters, a.appName, "apns")
		a.waitGroupDone()
		return
	}

	n, err := a.buildAndValidateNotification(parsedNotification)
	if err != nil {
		l.WithError(err).Error("notification is invalid")
		a.waitGroupDone()
		return

	}
	a.sendNotification(n)
	extensions.StatsReporterHandleNotificationSent(a.StatsReporters, a.appName, "apns", message.Topic)
}

func (a *APNSMessageHandler) parseKafkaMessage(message interfaces.KafkaMessage) (*pusherAPNSKafkaMessage, error) {
	notification := &pusherAPNSKafkaMessage{}
	err := json.Unmarshal(message.Value, notification)
	if err != nil {
		return nil, err
	}

	addMetadataToPayload(notification)
	if notification.Metadata == nil {
		notification.Metadata = map[string]interface{}{}
	}
	notification.Metadata["game"] = a.appName
	notification.Metadata["deviceToken"] = notification.DeviceToken

	hostname, err := os.Hostname()
	if err != nil {
		a.Logger.WithError(err).Error("error retrieving hostname")
	} else {
		notification.Metadata["hostname"] = hostname
	}
	notification.Metadata["timestamp"] = time.Now().Unix()
	notification.ApnsID = uuid.NewV4().String()
	return notification, nil
}

func (a *APNSMessageHandler) createDedupContentFromPayload(notification *pusherAPNSKafkaMessage) string {
	if notification.Payload != nil {
		payloadJSON, err := json.Marshal(notification.Payload)
		if err != nil {
			a.Logger.WithError(err).Error("error marshalling notification payload for deduplication")
			return "error-marshalling-payload-for-deduplication"
		}
		return string(payloadJSON)
	}
	return "empty-payload"

}

func (a *APNSMessageHandler) buildAndValidateNotification(notification *pusherAPNSKafkaMessage) (*structs.ApnsNotification, error) {
	if notification.PushExpiry > 0 && notification.PushExpiry < extensions.MakeTimestamp() {
		return nil, errors.New("push message has expired")
	}

	payload, err := json.Marshal(notification.Payload)
	if err != nil {
		return nil, fmt.Errorf("error marshalling notification payload: %w", err)
	}

	n := &structs.ApnsNotification{
		Notification: apns2.Notification{
			Topic:       a.ApnsTopic,
			DeviceToken: notification.DeviceToken,
			Payload:     payload,
			ApnsID:      notification.ApnsID,
			CollapseID:  notification.CollapseID,
		},
		Metadata: notification.Metadata,
	}
	return n, nil
}

func (a *APNSMessageHandler) sendNotification(notification *structs.ApnsNotification) {
	before := time.Now()
	defer extensions.StatsReporterReportSendNotificationLatency(a.StatsReporters, time.Since(before), a.appName, "apns", "client", "apns")

	notification.SendAttempts += 1
	a.PushQueue.Push(notification)
}

func (a *APNSMessageHandler) handleAPNSResponse(responseWithMetadata *structs.ResponseWithMetadata) error {
	l := a.Logger.WithFields(log.Fields{
		"method": "handleAPNSResponse",
		"res":    responseWithMetadata,
	})
	l.Debug("got response from apns")

	// retry on too many requests (429)
	// retries are transparent and are not counted as sent or received messages
	sendAttempts := responseWithMetadata.Notification.SendAttempts
	if responseWithMetadata.Reason == apns2.ReasonTooManyRequests &&
		uint(sendAttempts) < a.maxRetryAttempts {
		l.WithFields(log.Fields{
			"sendAttempts": sendAttempts,
			"maxRetries":   a.maxRetryAttempts,
			"apnsID":       responseWithMetadata.ApnsID,
		}).Info("retrying notification")
		<-time.After(a.retryInterval)
		a.sendNotification(responseWithMetadata.Notification)
		return nil
	}

	responseWithMetadata.Metadata = responseWithMetadata.Notification.Metadata
	if timestamp, ok := responseWithMetadata.Metadata["timestamp"].(int64); ok {
		responseWithMetadata.Timestamp = timestamp
	}
	delete(responseWithMetadata.Metadata, "timestamp")

	parsedTopic := extensions.ParsedTopic{
		Game:     a.appName,
		Platform: "apns",
	}

	a.waitGroupDone()

	if responseWithMetadata.Reason == "" {
		extensions.StatsReporterHandleNotificationSuccess(a.StatsReporters, a.appName, "apns")
		return nil
	}

	pErr := pusher_errors.NewPushError(a.mapErrorReason(responseWithMetadata.Reason), responseWithMetadata.Reason)
	responseWithMetadata.Err = pErr
	l.Info("notification failed")
	extensions.StatsReporterHandleNotificationFailure(a.StatsReporters, a.appName, "apns", pErr)

	switch responseWithMetadata.Reason {
	case apns2.ReasonBadDeviceToken, apns2.ReasonUnregistered, apns2.ReasonTopicDisallowed, apns2.ReasonDeviceTokenNotForTopic:
		// https://developer.apple.com/library/content/documentation/NetworkingInternet/Conceptual/RemoteNotificationsPG/CommunicatingwithAPNs.html
		if responseWithMetadata.Metadata == nil {
			responseWithMetadata.Metadata = map[string]interface{}{}
		}
		responseWithMetadata.Metadata["deleteToken"] = true
	}

	sendFeedbackErr := extensions.SendToFeedbackReporters(a.feedbackReporters, responseWithMetadata, parsedTopic)
	if sendFeedbackErr != nil {
		l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
	}

	return nil
}

func (a *APNSMessageHandler) mapErrorReason(reason string) string {
	switch reason {
	case apns2.ReasonPayloadEmpty:
		return "payload-empty"
	case apns2.ReasonPayloadTooLarge:
		return "payload-too-large"
	case apns2.ReasonMissingDeviceToken:
		return "missing-device-token"
	case apns2.ReasonBadDeviceToken:
		return "bad-device-token"
	case apns2.ReasonTooManyRequests:
		return "too-many-requests"
	case apns2.ReasonBadMessageID:
		return "bad-message-id"
	case apns2.ReasonBadExpirationDate:
		return "bad-expiration-date"
	case apns2.ReasonBadPriority:
		return "bad-priority"
	case apns2.ReasonBadTopic:
		return "bad-topic"
	case apns2.ReasonBadCertificate:
		return "bad-certificate"
	case apns2.ReasonBadCertificateEnvironment:
		return "bad-certificate-environment"
	case apns2.ReasonForbidden:
		return "forbidden"
	case apns2.ReasonMissingTopic:
		return "missing-topic"
	case apns2.ReasonTopicDisallowed:
		return "topic-disallowed"
	case apns2.ReasonUnregistered:
		return "unregistered"
	case apns2.ReasonDeviceTokenNotForTopic:
		return "device-token-not-for-topic"
	case apns2.ReasonDuplicateHeaders:
		return "duplicate-headers"
	case apns2.ReasonBadPath:
		return "bad-path"
	case apns2.ReasonMethodNotAllowed:
		return "method-not-allowed"
	case apns2.ReasonIdleTimeout:
		return "idle-timeout"
	case apns2.ReasonShutdown:
		return "shutdown"
	case apns2.ReasonInternalServerError:
		return "internal-server-error"
	case apns2.ReasonServiceUnavailable:
		return "service-unavailable"
	case apns2.ReasonExpiredProviderToken:
		return "expired-provider-token"
	case apns2.ReasonInvalidProviderToken:
		return "invalid-provider-token"
	case apns2.ReasonMissingProviderToken:
		return "missing-provider-token"
	default:
		return "unexpected"
	}
}

func addMetadataToPayload(notification *pusherAPNSKafkaMessage) {
	if notification.Metadata == nil {
		return
	}

	if notification.Payload == nil {
		notification.Payload = map[string]interface{}{}
	}

	if p, ok := notification.Payload.(map[string]interface{}); ok {
		if p["M"] == nil {
			p["M"] = map[string]interface{}{}
		}

		m, ok := p["M"].(map[string]interface{})
		if !ok {
			return
		}

		for k, v := range notification.Metadata {
			if m[k] == nil {
				m[k] = v
			}
		}
	}
}

func (a *APNSMessageHandler) waitGroupDone() {
	if a.pendingMessagesWG != nil {
		a.pendingMessagesWG.Done()
	}
}

// Cleanup closes connections to APNS.
func (a *APNSMessageHandler) Cleanup() error {
	a.PushQueue.Close()
	return nil
}
