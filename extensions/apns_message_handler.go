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
	"crypto/tls"
	"encoding/json"
	"sync"
	"time"

	cert "github.com/RobotsAndPencils/buford/certificate"
	"github.com/RobotsAndPencils/buford/push"
	log "github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/certificate"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
)

var apnsResMutex sync.Mutex
var inflightMessagesMetadataLock sync.Mutex

// Notification is the notification base struct
type Notification struct {
	DeviceToken string
	Payload     interface{}
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	PushExpiry  int                    `json:"push_expiry,omitempty"`
}

// ResponseWithMetadata is a enriched Response with a Metadata field
type ResponseWithMetadata struct {
	push.Response
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// APNSMessageHandler implements the messagehandler interface
type APNSMessageHandler struct {
	certificate              tls.Certificate
	CertificatePath          string
	Config                   *viper.Viper
	failuresReceived         int64
	feedbackReporters        []interfaces.FeedbackReporter
	InflightMessagesMetadata map[string]interface{}
	InvalidTokenHandlers     []interfaces.InvalidTokenHandler
	IsProduction             bool
	Logger                   *log.Logger
	LogStatsInterval         time.Duration
	pendingMessagesWG        *sync.WaitGroup
	PushQueue                interfaces.APNSPushQueue
	responsesReceived        int64
	run                      bool
	sentMessages             int64
	StatsReporters           []interfaces.StatsReporter
	successesReceived        int64
	Topic                    string
}

// NewAPNSMessageHandler returns a new instance of a APNSMessageHandler
func NewAPNSMessageHandler(
	certificatePath string,
	isProduction bool,
	config *viper.Viper,
	logger *log.Logger,
	pendingMessagesWG *sync.WaitGroup,
	statsReporters []interfaces.StatsReporter,
	feedbackReporters []interfaces.FeedbackReporter,
	invalidTokenHandlers []interfaces.InvalidTokenHandler,
	queue interfaces.APNSPushQueue,
) (*APNSMessageHandler, error) {
	a := &APNSMessageHandler{
		CertificatePath:          certificatePath,
		Config:                   config,
		failuresReceived:         0,
		feedbackReporters:        feedbackReporters,
		InflightMessagesMetadata: map[string]interface{}{},
		InvalidTokenHandlers:     invalidTokenHandlers,
		IsProduction:             isProduction,
		Logger:                   logger,
		pendingMessagesWG:        pendingMessagesWG,
		responsesReceived:        0,
		sentMessages:             0,
		StatsReporters:           statsReporters,
		successesReceived:        0,
	}
	if err := a.configure(queue); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *APNSMessageHandler) configure(queue interfaces.APNSPushQueue) error {
	a.loadConfigurationDefaults()
	interval := a.Config.GetInt("apns.logStatsInterval")
	a.LogStatsInterval = time.Duration(interval) * time.Millisecond
	err := a.configureCertificate()
	if err != nil {
		return err
	}
	if queue != nil {
		err = nil
		a.PushQueue = queue
	} else {
		err = a.configureAPNSPushQueue()
	}
	if err != nil {
		return err
	}
	return nil
}

func (a *APNSMessageHandler) loadConfigurationDefaults() {
	a.Config.SetDefault("apns.concurrentWorkers", 10)
	a.Config.SetDefault("apns.logStatsInterval", 5000)
}

func (a *APNSMessageHandler) configureCertificate() error {
	l := a.Logger.WithField("method", "configureCertificate")
	c, err := certificate.FromPemFile(a.CertificatePath, "")
	if err != nil {
		l.WithError(err).Error("error loading pem certificate")
		return err
	}
	a.certificate = c
	a.Topic = cert.TopicFromCert(c)
	l.WithField("topic", a.Topic).Debug("loaded apns certificate")
	return nil
}

func (a *APNSMessageHandler) configureAPNSPushQueue() error {
	l := a.Logger.WithField("method", "configureAPNSPushQueue")
	client, err := push.NewClient(a.certificate)
	if err != nil {
		l.WithError(err).Error("could not create apns client")
		return err
	}
	var svc *push.Service
	if a.IsProduction {
		svc = push.NewService(client, push.Production)
	} else {
		svc = push.NewService(client, push.Development)
	}

	concurrentWorkers := a.Config.GetInt("apns.concurrentWorkers")
	l.WithField("concurrentWorkers", concurrentWorkers).Debug("creating apns queue")
	workers := uint(concurrentWorkers)
	queue := push.NewQueue(svc, workers)
	a.PushQueue = queue
	return nil
}

func (a *APNSMessageHandler) sendMessage(message []byte) error {
	l := a.Logger.WithField("method", "sendMessage")
	l.WithField("message", message).Debug("sending message to apns")
	h := &push.Headers{
		Topic: a.Topic,
	}
	n := &Notification{}
	json.Unmarshal(message, n)
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		l.WithError(err).Error("error marshaling message payload")
		return err
	}
	statsReporterHandleNotificationSent(a.StatsReporters)
	a.PushQueue.Push(n.DeviceToken, h, payload)
	inflightMessagesMetadataLock.Lock()
	a.InflightMessagesMetadata[n.DeviceToken] = n.Metadata
	inflightMessagesMetadataLock.Unlock()
	a.sentMessages++
	return nil
}

// HandleResponses from apns
func (a *APNSMessageHandler) HandleResponses() {
	q, ok := a.PushQueue.(*push.Queue)
	if ok {
		for resp := range q.Responses {
			a.handleAPNSResponse(resp)
		}
	}
}

// HandleMessages get messages from msgChan and send to APNS
func (a *APNSMessageHandler) HandleMessages(msgChan *chan []byte) {
	a.run = true

	for a.run == true {
		select {
		case message := <-*msgChan:
			a.sendMessage(message)
		}
	}
}

func (a *APNSMessageHandler) handleAPNSResponse(res push.Response) error {
	defer func() {
		if a.pendingMessagesWG != nil {
			a.pendingMessagesWG.Done()
		}
	}()

	l := a.Logger.WithFields(log.Fields{
		"method": "handleAPNSResponse",
		"res":    res,
	})
	l.Debug("got response from apns")
	apnsResMutex.Lock()
	a.responsesReceived++
	apnsResMutex.Unlock()
	var err error
	responseWithMetadata := &ResponseWithMetadata{
		Response: res,
	}
	inflightMessagesMetadataLock.Lock()
	if val, ok := a.InflightMessagesMetadata[res.DeviceToken]; ok {
		responseWithMetadata.Metadata = val.(map[string]interface{})
		delete(a.InflightMessagesMetadata, res.DeviceToken)
	}
	inflightMessagesMetadataLock.Unlock()

	if err != nil {
		l.WithError(err).Error("error sending feedback to reporter")
	}
	if res.Err != nil {
		apnsResMutex.Lock()
		a.failuresReceived++
		apnsResMutex.Unlock()
		pushError, ok := res.Err.(*push.Error)
		if !ok {
			l.WithFields(log.Fields{
				"category":   "UnexpectedError",
				log.ErrorKey: res.Err,
			}).Error("received an error")
			return res.Err
		}
		reason := pushError.Reason
		pErr := errors.NewPushError(a.mapErrorReason(reason), pushError.Error())
		statsReporterHandleNotificationFailure(a.StatsReporters, pErr)

		err = pErr
		switch reason {
		case push.ErrMissingDeviceToken, push.ErrBadDeviceToken:
			l.WithFields(log.Fields{
				"category":   "TokenError",
				log.ErrorKey: res.Err,
			}).Debug("received an error")
			if responseWithMetadata.Metadata != nil {
				responseWithMetadata.Metadata["deleteToken"] = true
			}
			handleInvalidToken(a.InvalidTokenHandlers, res.DeviceToken)
		case push.ErrBadCertificate, push.ErrBadCertificateEnvironment, push.ErrForbidden:
			l.WithFields(log.Fields{
				"category":   "CertificateError",
				log.ErrorKey: res.Err,
			}).Debug("received an error")
		case push.ErrMissingTopic, push.ErrTopicDisallowed, push.ErrDeviceTokenNotForTopic:
			l.WithFields(log.Fields{
				"category":   "TopicError",
				log.ErrorKey: res.Err,
			}).Debug("received an error")
		case push.ErrIdleTimeout, push.ErrShutdown, push.ErrInternalServerError, push.ErrServiceUnavailable:
			l.WithFields(log.Fields{
				"category":   "AppleError",
				log.ErrorKey: res.Err,
			}).Debug("received an error")
		default:
			l.WithFields(log.Fields{
				"category":   "DefaultError",
				log.ErrorKey: res.Err,
			}).Debug("received an error")
		}
		responseWithMetadata.Err = pErr
		sendFeedbackErr := sendToFeedbackReporters(a.feedbackReporters, responseWithMetadata)
		if sendFeedbackErr != nil {
			l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
		}
		return err
	}
	sendFeedbackErr := sendToFeedbackReporters(a.feedbackReporters, responseWithMetadata)

	if sendFeedbackErr != nil {
		l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
	}
	apnsResMutex.Lock()
	a.successesReceived++
	apnsResMutex.Unlock()
	statsReporterHandleNotificationSuccess(a.StatsReporters)
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
			"responsesReceived": a.responsesReceived,
			"successesReceived": a.successesReceived,
			"failuresReceived":  a.failuresReceived,
		}).Info("flushing stats")
		a.sentMessages = 0
		a.responsesReceived = 0
		a.successesReceived = 0
		a.failuresReceived = 0
		apnsResMutex.Unlock()
	}
}

func (a *APNSMessageHandler) mapErrorReason(reason error) string {
	switch reason {
	case push.ErrPayloadEmpty:
		return "payload-empty"
	case push.ErrPayloadTooLarge:
		return "payload-too-large"
	case push.ErrMissingDeviceToken:
		return "missing-device-token"
	case push.ErrBadDeviceToken:
		return "bad-device-token"
	case push.ErrTooManyRequests:
		return "too-many-requests"
	case push.ErrBadMessageID:
		return "bad-message-id"
	case push.ErrBadExpirationDate:
		return "bad-expiration-date"
	case push.ErrBadPriority:
		return "bad-priority"
	case push.ErrBadTopic:
		return "bad-topic"
	case push.ErrBadCertificate:
		return "bad-certificate"
	case push.ErrBadCertificateEnvironment:
		return "bad-certificate-environment"
	case push.ErrForbidden:
		return "forbidden"
	case push.ErrMissingTopic:
		return "missing-topic"
	case push.ErrTopicDisallowed:
		return "topic-disallowed"
	case push.ErrUnregistered:
		return "unregistered"
	case push.ErrDeviceTokenNotForTopic:
		return "device-token-not-for-topic"
	case push.ErrDuplicateHeaders:
		return "duplicate-headers"
	case push.ErrBadPath:
		return "bad-path"
	case push.ErrMethodNotAllowed:
		return "method-not-allowed"
	case push.ErrIdleTimeout:
		return "idle-timeout"
	case push.ErrShutdown:
		return "shutdown"
	case push.ErrInternalServerError:
		return "internal-server-error"
	case push.ErrServiceUnavailable:
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
