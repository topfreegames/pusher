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
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	gcm "github.com/topfreegames/go-gcm"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
)

var gcmResMutex sync.Mutex

// KafkaGCMMessage is a enriched XMPPMessage with a Metadata field
type KafkaGCMMessage struct {
	gcm.XMPPMessage
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CCSMessageWithMetadata is a enriched CCSMessage with a metadata field
type CCSMessageWithMetadata struct {
	gcm.CCSMessage
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// GCMMessageHandler implements the messagehandler interface
type GCMMessageHandler struct {
	apiKey                       string
	Config                       *viper.Viper
	failuresReceived             int64
	feedbackReporters            []interfaces.FeedbackReporter
	GCMClient                    interfaces.GCMClient
	InflightMessagesMetadata     map[string]interface{}
	InvalidTokenHandlers         []interfaces.InvalidTokenHandler
	IsProduction                 bool
	Logger                       *log.Logger
	LogStatsInterval             time.Duration
	pendingMessages              chan bool
	pendingMessagesWG            *sync.WaitGroup
	inflightMessagesMetadataLock *sync.Mutex
	PingInterval                 int
	PingTimeout                  int
	responsesReceived            int64
	run                          bool
	senderID                     string
	sentMessages                 int64
	StatsReporters               []interfaces.StatsReporter
	successesReceived            int64
	requestsHeap                 *TimeoutHeap
	CacheCleaningInterval        int
}

// NewGCMMessageHandler returns a new instance of a GCMMessageHandler
func NewGCMMessageHandler(
	senderID, apiKey string,
	isProduction bool,
	config *viper.Viper,
	logger *log.Logger,
	pendingMessagesWG *sync.WaitGroup,
	statsReporters []interfaces.StatsReporter,
	feedbackReporters []interfaces.FeedbackReporter,
	invalidTokenHandlers []interfaces.InvalidTokenHandler,
	client interfaces.GCMClient,
) (*GCMMessageHandler, error) {
	l := logger.WithFields(log.Fields{
		"method":       "NewGCMMessageHandler",
		"senderID":     senderID,
		"apiKey":       apiKey,
		"isProduction": isProduction,
	})

	g := &GCMMessageHandler{
		apiKey:                       apiKey,
		Config:                       config,
		failuresReceived:             0,
		feedbackReporters:            feedbackReporters,
		InflightMessagesMetadata:     map[string]interface{}{},
		InvalidTokenHandlers:         invalidTokenHandlers,
		IsProduction:                 isProduction,
		Logger:                       logger,
		pendingMessagesWG:            pendingMessagesWG,
		inflightMessagesMetadataLock: &sync.Mutex{},
		responsesReceived:            0,
		senderID:                     senderID,
		sentMessages:                 0,
		StatsReporters:               statsReporters,
		successesReceived:            0,
		requestsHeap:                 NewTimeoutHeap(config),
	}
	err := g.configure(client)
	if err != nil {
		l.Error("Failed to create a new GCM Message handler.")
		return nil, err
	}
	return g, nil
}

func (g *GCMMessageHandler) configure(client interfaces.GCMClient) error {
	g.loadConfigurationDefaults()
	g.pendingMessages = make(chan bool, g.Config.GetInt("gcm.maxPendingMessages"))
	interval := g.Config.GetInt("gcm.logStatsInterval")
	g.LogStatsInterval = time.Duration(interval) * time.Millisecond
	g.CacheCleaningInterval = g.Config.GetInt("feedback.cache.cleaningInterval")
	var err error
	if client != nil {
		err = nil
		g.GCMClient = client
	} else {
		err = g.configureGCMClient()
	}
	if err != nil {
		return err
	}
	return nil
}

func (g *GCMMessageHandler) loadConfigurationDefaults() {
	g.Config.SetDefault("gcm.pingInterval", 20)
	g.Config.SetDefault("gcm.pingTimeout", 30)
	g.Config.SetDefault("gcm.maxPendingMessages", 100)
	g.Config.SetDefault("gcm.logStatsInterval", 5000)
	g.Config.SetDefault("feedback.cache.cleaningInterval", 300000)
}

func (g *GCMMessageHandler) configureGCMClient() error {
	l := g.Logger.WithFields(log.Fields{
		"method": "configureGCMClient",
	})
	g.PingInterval = g.Config.GetInt("gcm.pingInterval")
	g.PingTimeout = g.Config.GetInt("gcm.pingTimeout")
	gcmConfig := &gcm.Config{
		SenderID:          g.senderID,
		APIKey:            g.apiKey,
		Sandbox:           !g.IsProduction,
		MonitorConnection: true,
		Debug:             false,
		PingInterval:      g.PingInterval,
		PingTimeout:       g.PingTimeout,
	}
	var err error
	cl, err := gcm.NewClient(gcmConfig, g.handleGCMResponse)
	if err != nil {
		l.Error("Failed to create gcm client.")
		return err
	}
	g.GCMClient = cl
	return nil
}

// WARNING: Be careful, code here needs to be thread safe!
func (g *GCMMessageHandler) handleGCMResponse(cm gcm.CCSMessage) error {
	defer func() {
		if g.pendingMessagesWG != nil {
			g.pendingMessagesWG.Done()
		}
	}()

	l := g.Logger.WithFields(log.Fields{
		"method":     "handleGCMResponse",
		"ccsMessage": cm,
	})
	l.Debug("Got response from gcm.")
	gcmResMutex.Lock()

	select {
	case <-g.pendingMessages:
		l.Debug("Freeing pendingMessages channel")
	default:
		l.Warn("No pending messages in channel but received response.")
	}

	g.responsesReceived++
	gcmResMutex.Unlock()

	var err error
	ccsMessageWithMetadata := &CCSMessageWithMetadata{
		CCSMessage: cm,
	}
	g.inflightMessagesMetadataLock.Lock()
	if val, ok := g.InflightMessagesMetadata[cm.MessageID]; ok {
		ccsMessageWithMetadata.Metadata = val.(map[string]interface{})
		ccsMessageWithMetadata.Timestamp = ccsMessageWithMetadata.Metadata["timestamp"].(int64)
		delete(ccsMessageWithMetadata.Metadata, "timestamp")
		delete(g.InflightMessagesMetadata, cm.MessageID)
	}
	g.inflightMessagesMetadataLock.Unlock()

	if cm.Error != "" {
		gcmResMutex.Lock()
		g.failuresReceived++
		gcmResMutex.Unlock()
		pErr := errors.NewPushError(strings.ToLower(cm.Error), cm.ErrorDescription)
		statsReporterHandleNotificationFailure(g.StatsReporters, pErr)

		err = pErr
		switch cm.Error {
		// errors from https://developers.google.com/cloud-messaging/xmpp-server-ref table 4
		case "DEVICE_UNREGISTERED", "BAD_REGISTRATION":
			l.WithFields(log.Fields{
				"category":   "TokenError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Debug("received an error")
			if ccsMessageWithMetadata.Metadata != nil {
				ccsMessageWithMetadata.Metadata["deleteToken"] = true
			}
			handleInvalidToken(g.InvalidTokenHandlers, cm.From)
		case "INVALID_JSON":
			l.WithFields(log.Fields{
				"category":   "JsonError",
				log.ErrorKey: cm.Error,
			}).Debug("received an error")
		case "SERVICE_UNAVAILABLE", "INTERNAL_SERVER_ERROR":
			l.WithFields(log.Fields{
				"category":   "GoogleError",
				log.ErrorKey: cm.Error,
			}).Debug("received an error")
		case "DEVICE_MESSAGE_RATE_EXCEEDED", "TOPICS_MESSAGE_RATE_EXCEEDED":
			l.WithFields(log.Fields{
				"category":   "RateExceededError",
				log.ErrorKey: cm.Error,
			}).Debug("received an error")
		default:
			l.WithFields(log.Fields{
				"category":   "DefaultError",
				log.ErrorKey: cm.Error,
			}).Debug("received an error")
		}
		sendFeedbackErr := sendToFeedbackReporters(g.feedbackReporters, ccsMessageWithMetadata)
		if sendFeedbackErr != nil {
			l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
		}
		return err
	}

	sendFeedbackErr := sendToFeedbackReporters(g.feedbackReporters, ccsMessageWithMetadata)
	if sendFeedbackErr != nil {
		l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
	}

	gcmResMutex.Lock()
	g.successesReceived++
	gcmResMutex.Unlock()
	statsReporterHandleNotificationSuccess(g.StatsReporters)

	return nil
}

func (g *GCMMessageHandler) sendMessage(message []byte) error {
	g.pendingMessages <- true
	l := g.Logger.WithField("method", "sendMessage")
	//ttl := uint(0)
	km := KafkaGCMMessage{}
	err := json.Unmarshal(message, &km)
	if err != nil {
		<-g.pendingMessages
		l.WithError(err).Error("Error unmarshaling message.")
		return err
	}
	l.WithField("message", km).Debug("sending message to gcm")
	var messageID string
	var bytes int

	messageID, bytes, err = g.GCMClient.SendXMPP(km.XMPPMessage)

	if err != nil {
		<-g.pendingMessages
		l.WithError(err).Error("Error sending message.")
		return err
	}

	if messageID != "" {
		if km.Metadata == nil {
			km.Metadata = map[string]interface{}{}
		}
		g.inflightMessagesMetadataLock.Lock()

		km.Metadata["timestamp"] = time.Now().Unix()
		hostname, err := os.Hostname()
		if err != nil {
			l.WithError(err).Error("error retrieving hostname")
		} else {
			km.Metadata["hostname"] = hostname
		}
		km.Metadata["msgid"] = uuid.NewV4().String()
		g.InflightMessagesMetadata[messageID] = km.Metadata
		g.requestsHeap.AddRequest(messageID)

		g.inflightMessagesMetadataLock.Unlock()
	}

	statsReporterHandleNotificationSent(g.StatsReporters)
	g.sentMessages++
	l.WithFields(log.Fields{
		"messageID": messageID,
		"bytes":     bytes,
	}).Debug("sent message")
	return nil
}

// HandleResponses from gcm
func (g *GCMMessageHandler) HandleResponses() {
}

// CleanMetadataCache clears cache after timeout
func (g *GCMMessageHandler) CleanMetadataCache() {
	var deviceToken string
	var hasIndeed bool
	for {
		g.inflightMessagesMetadataLock.Lock()
		for deviceToken, hasIndeed = g.requestsHeap.HasExpiredRequest(); hasIndeed; {
			delete(g.InflightMessagesMetadata, deviceToken)
			deviceToken, hasIndeed = g.requestsHeap.HasExpiredRequest()
		}
		g.inflightMessagesMetadataLock.Unlock()

		duration := time.Duration(g.CacheCleaningInterval)
		time.Sleep(duration * time.Millisecond)
	}
}

// HandleMessages get messages from msgChan and send to GCM
func (g *GCMMessageHandler) HandleMessages(msgChan *chan []byte) {
	g.run = true

	for g.run == true {
		select {
		case message := <-*msgChan:
			g.sendMessage(message)
		}
	}
}

// LogStats from time to time
func (g *GCMMessageHandler) LogStats() {
	l := g.Logger.WithFields(log.Fields{
		"method":       "logStats",
		"interval(ns)": g.LogStatsInterval,
	})

	ticker := time.NewTicker(g.LogStatsInterval)
	for range ticker.C {
		apnsResMutex.Lock()
		l.WithFields(log.Fields{
			"sentMessages":      g.sentMessages,
			"responsesReceived": g.responsesReceived,
			"successesReceived": g.successesReceived,
			"failuresReceived":  g.failuresReceived,
		}).Info("flushing stats")
		g.sentMessages = 0
		g.responsesReceived = 0
		g.successesReceived = 0
		g.failuresReceived = 0
		apnsResMutex.Unlock()
	}
}

//Cleanup closes connections to GCM
func (g *GCMMessageHandler) Cleanup() error {
	err := g.GCMClient.Close()
	if err != nil {
		return err
	}
	return nil
}
