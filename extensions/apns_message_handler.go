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

package extensions

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	token "github.com/sideshow/apns2/token"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/structs"
)

var apnsResMutex sync.Mutex

// Notification is the notification base struct
type Notification struct {
	DeviceToken string
	Payload     interface{}
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	PushExpiry  int64                  `json:"push_expiry,omitempty"`
}

// APNSMessageHandler implements the messagehandler interface
type APNSMessageHandler struct {
	authKeyPath                  string
	keyID                        string
	teamID                       string
	token                        *token.Token
	appName                      string
	Config                       *viper.Viper
	clients                      chan *apns2.Client
	failuresReceived             int64
	feedbackReporters            []interfaces.FeedbackReporter
	InflightMessagesMetadata     map[string]interface{}
	InvalidTokenHandlers         []interfaces.InvalidTokenHandler
	IsProduction                 bool
	Logger                       *log.Logger
	LogStatsInterval             time.Duration
	pendingMessagesWG            *sync.WaitGroup
	inflightMessagesMetadataLock *sync.Mutex
	PushQueue                    interfaces.APNSPushQueue
	responsesReceived            int64
	run                          bool
	sentMessages                 int64
	ignoredMessages              int64
	StatsReporters               []interfaces.StatsReporter
	successesReceived            int64
	Topic                        string
	requestsHeap                 *TimeoutHeap
	CacheCleaningInterval        int
}

// NewAPNSMessageHandler returns a new instance of a APNSMessageHandler
func NewAPNSMessageHandler(
	authKeyPath, keyID, teamID, topic, appName string,
	isProduction bool,
	config *viper.Viper,
	logger *log.Logger,
	pendingMessagesWG *sync.WaitGroup,
	statsReporters []interfaces.StatsReporter,
	feedbackReporters []interfaces.FeedbackReporter,
	invalidTokenHandlers []interfaces.InvalidTokenHandler,
	pushQueue interfaces.APNSPushQueue,
) (*APNSMessageHandler, error) {
	a := &APNSMessageHandler{
		authKeyPath:                  authKeyPath,
		keyID:                        keyID,
		teamID:                       teamID,
		Topic:                        topic,
		appName:                      appName,
		Config:                       config,
		failuresReceived:             0,
		feedbackReporters:            feedbackReporters,
		InflightMessagesMetadata:     map[string]interface{}{},
		InvalidTokenHandlers:         invalidTokenHandlers,
		IsProduction:                 isProduction,
		Logger:                       logger,
		pendingMessagesWG:            pendingMessagesWG,
		ignoredMessages:              0,
		inflightMessagesMetadataLock: &sync.Mutex{},
		responsesReceived:            0,
		sentMessages:                 0,
		StatsReporters:               statsReporters,
		successesReceived:            0,
		requestsHeap:                 NewTimeoutHeap(config),
		PushQueue:                    pushQueue,
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
}

func (a *APNSMessageHandler) sendMessage(message interfaces.KafkaMessage) error {
	deviceIdentifier := uuid.NewV4().String()
	l := a.Logger.WithField("method", "sendMessage")
	l.WithField("message", message).Debug("sending message to apns")
	n := &Notification{}
	json.Unmarshal(message.Value, n)
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		l.WithError(err).Error("error marshaling message payload")
		return err
	}
	if n.PushExpiry > 0 && n.PushExpiry < makeTimestamp() {
		l.Warnf("ignoring push message because it has expired: %s", n.Payload)
		a.ignoredMessages++
		if a.pendingMessagesWG != nil {
			a.pendingMessagesWG.Done()
		}
		return nil
	}
	statsReporterHandleNotificationSent(a.StatsReporters, a.appName, "apns")
	a.PushQueue.Push(&apns2.Notification{
		Topic:       a.Topic,
		DeviceToken: n.DeviceToken,
		Payload:     payload,
		ApnsID:      deviceIdentifier,
	})
	if n.Metadata == nil {
		n.Metadata = map[string]interface{}{}
	}
	a.inflightMessagesMetadataLock.Lock()

	n.Metadata["timestamp"] = time.Now().Unix()
	n.Metadata["game"] = a.appName
	n.Metadata["platform"] = "apns"
	n.Metadata["deviceToken"] = n.DeviceToken
	hostname, err := os.Hostname()
	if err != nil {
		l.WithError(err).Error("error retrieving hostname")
	} else {
		n.Metadata["hostname"] = hostname
	}
	n.Metadata["msgid"] = uuid.NewV4().String()
	a.InflightMessagesMetadata[deviceIdentifier] = n.Metadata
	a.requestsHeap.AddRequest(deviceIdentifier)

	a.inflightMessagesMetadataLock.Unlock()
	a.sentMessages++
	return nil
}

// HandleResponses from apns
func (a *APNSMessageHandler) HandleResponses() {
	for response := range a.PushQueue.ResponseChannel() {
		a.handleAPNSResponse(response)
	}
}

// CleanMetadataCache clears expired requests from memory
func (a *APNSMessageHandler) CleanMetadataCache() {
	var deviceToken string
	var hasIndeed bool
	for {
		a.inflightMessagesMetadataLock.Lock()
		for deviceToken, hasIndeed = a.requestsHeap.HasExpiredRequest(); hasIndeed; {
			delete(a.InflightMessagesMetadata, deviceToken)
			deviceToken, hasIndeed = a.requestsHeap.HasExpiredRequest()
		}
		a.inflightMessagesMetadataLock.Unlock()

		duration := time.Duration(a.CacheCleaningInterval)
		time.Sleep(duration * time.Millisecond)
	}
}

// HandleMessages get messages from msgChan and send to APNS
func (a *APNSMessageHandler) HandleMessages(message interfaces.KafkaMessage) {
	a.sendMessage(message)
}

func (a *APNSMessageHandler) handleAPNSResponse(responseWithMetadata *structs.ResponseWithMetadata) error {
	defer func() {
		if a.pendingMessagesWG != nil {
			a.pendingMessagesWG.Done()
		}
	}()

	l := a.Logger.WithFields(log.Fields{
		"method": "handleAPNSResponse",
		"res":    responseWithMetadata,
	})
	l.Debug("got response from apns")
	apnsResMutex.Lock()
	a.responsesReceived++
	apnsResMutex.Unlock()
	parsedTopic := ParsedTopic{
		Game:     a.appName,
		Platform: "apns",
	}
	var err error
	a.inflightMessagesMetadataLock.Lock()
	if val, ok := a.InflightMessagesMetadata[responseWithMetadata.ApnsID]; ok {
		responseWithMetadata.Metadata = val.(map[string]interface{})
		responseWithMetadata.Timestamp = responseWithMetadata.Metadata["timestamp"].(int64)
		delete(responseWithMetadata.Metadata, "timestamp")
		delete(a.InflightMessagesMetadata, responseWithMetadata.ApnsID)
	}
	a.inflightMessagesMetadataLock.Unlock()

	if err != nil {
		l.WithError(err).Error("error sending feedback to reporter")
	}

	if responseWithMetadata.Reason != "" {
		apnsResMutex.Lock()
		a.failuresReceived++
		apnsResMutex.Unlock()
		reason := responseWithMetadata.Reason
		pErr := errors.NewPushError(a.mapErrorReason(reason), reason)
		responseWithMetadata.Err = pErr
		statsReporterHandleNotificationFailure(a.StatsReporters, a.appName, "apns", pErr)

		err = pErr
		switch reason {
		case apns2.ReasonMissingDeviceToken, apns2.ReasonBadDeviceToken:
			l.WithFields(log.Fields{
				"category":   "TokenError",
				log.ErrorKey: responseWithMetadata.Reason,
			}).Debug("received an error")
			if responseWithMetadata.Metadata != nil {
				responseWithMetadata.Metadata["deleteToken"] = true
			}
			handleInvalidToken(
				a.InvalidTokenHandlers, responseWithMetadata.DeviceToken,
				a.appName, "apns",
			)
		case apns2.ReasonBadCertificate, apns2.ReasonBadCertificateEnvironment, apns2.ReasonForbidden:
			l.WithFields(log.Fields{
				"category":   "CertificateError",
				log.ErrorKey: responseWithMetadata.Reason,
			}).Debug("received an error")
		case apns2.ReasonMissingTopic, apns2.ReasonTopicDisallowed, apns2.ReasonDeviceTokenNotForTopic:
			l.WithFields(log.Fields{
				"category":   "TopicError",
				log.ErrorKey: responseWithMetadata.Reason,
			}).Debug("received an error")
		case apns2.ReasonIdleTimeout, apns2.ReasonShutdown, apns2.ReasonInternalServerError, apns2.ReasonServiceUnavailable:
			l.WithFields(log.Fields{
				"category":   "AppleError",
				log.ErrorKey: responseWithMetadata.Reason,
			}).Debug("received an error")
		default:
			l.WithFields(log.Fields{
				"category":   "DefaultError",
				log.ErrorKey: responseWithMetadata.Reason,
			}).Debug("received an error")
		}
		sendFeedbackErr := sendToFeedbackReporters(a.feedbackReporters, responseWithMetadata, parsedTopic)
		if sendFeedbackErr != nil {
			l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
		}
		return err
	}
	sendFeedbackErr := sendToFeedbackReporters(a.feedbackReporters, responseWithMetadata, parsedTopic)

	if sendFeedbackErr != nil {
		l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
	}
	apnsResMutex.Lock()
	a.successesReceived++
	apnsResMutex.Unlock()
	statsReporterHandleNotificationSuccess(a.StatsReporters, a.appName, "apns")
	return nil
}

// LogStats from time to time
func (a *APNSMessageHandler) LogStats() {
	l := a.Logger.WithFields(log.Fields{
		"method":       "logStats",
		"interval(ns)": a.LogStatsInterval,
	})

	ticker := time.NewTicker(a.LogStatsInterval)
	for range ticker.C {
		apnsResMutex.Lock()
		l.WithFields(log.Fields{
			"sentMessages":      a.sentMessages,
			"ignoredMessages":   a.ignoredMessages,
			"responsesReceived": a.responsesReceived,
			"successesReceived": a.successesReceived,
			"failuresReceived":  a.failuresReceived,
		}).Info("flushing stats")
		a.sentMessages = 0
		a.responsesReceived = 0
		a.ignoredMessages = 0
		a.successesReceived = 0
		a.failuresReceived = 0
		apnsResMutex.Unlock()
	}
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
	default:
		return "unexpected"
	}
}

//Cleanup closes connections to APNS
func (a *APNSMessageHandler) Cleanup() error {
	a.PushQueue.Close()
	return nil
}
