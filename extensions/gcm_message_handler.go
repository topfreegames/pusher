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
	"github.com/spf13/viper"
	"github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
)

var gcmResMutex sync.Mutex

// KafkaGCMMessage is a enriched XMPPMessage with a Metadata field
type KafkaGCMMessage struct {
	gcm.XMPPMessage
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	PushExpiry int64                  `json:"push_expiry,omitempty"`
}

// CCSMessageWithMetadata is a enriched CCSMessage with a metadata field
type CCSMessageWithMetadata struct {
	gcm.CCSMessage
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// GCMMessageHandler implements the messagehandler interface
type GCMMessageHandler struct {
	feedbackReporters            []interfaces.FeedbackReporter
	StatsReporters               []interfaces.StatsReporter
	apiKey                       string
	game                         string
	GCMClient                    interfaces.GCMClient
	senderID                     string
	Config                       *viper.Viper
	failuresReceived             int64
	InflightMessagesMetadata     map[string]interface{}
	Logger                       *log.Entry
	LogStatsInterval             time.Duration
	pendingMessages              chan bool
	pendingMessagesWG            *sync.WaitGroup
	ignoredMessages              int64
	inflightMessagesMetadataLock *sync.Mutex
	PingInterval                 int
	PingTimeout                  int
	responsesReceived            int64
	sentMessages                 int64
	successesReceived            int64
	requestsHeap                 *TimeoutHeap
	CacheCleaningInterval        int
	IsProduction                 bool
}

// NewGCMMessageHandler returns a new instance of a GCMMessageHandler
func NewGCMMessageHandler(
	senderID, apiKey, game string,
	isProduction bool,
	config *viper.Viper,
	logger *log.Logger,
	pendingMessagesWG *sync.WaitGroup,
	statsReporters []interfaces.StatsReporter,
	feedbackReporters []interfaces.FeedbackReporter,
	client interfaces.GCMClient,
) (*GCMMessageHandler, error) {
	log := logger.WithFields(log.Fields{
		"game":         game,
		"isProduction": isProduction,
	})
	l := log.WithField("method", "NewGCMMessageHandler")
	config.SetDefault("gcm.client.initialization.retries", 3)

	g := &GCMMessageHandler{
		game:                         game,
		apiKey:                       apiKey,
		Config:                       config,
		failuresReceived:             0,
		feedbackReporters:            feedbackReporters,
		InflightMessagesMetadata:     map[string]interface{}{},
		IsProduction:                 isProduction,
		Logger:                       log,
		pendingMessagesWG:            pendingMessagesWG,
		ignoredMessages:              0,
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
		l.WithError(err).Error("Failed to create a new GCM Message handler.")
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
	l := g.Logger.WithField("method", "configureGCMClient")
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
	var cl interfaces.GCMClient
	for retries := g.Config.GetInt("gcm.client.initialization.retries"); retries > 0; retries-- {
		cl, err = gcm.NewClient(gcmConfig, g.handleGCMResponse)
		if err != nil && retries-1 != 0 {
			l.WithError(err).Warnf("failed to create gcm client. %d attempts left.", retries-1)
		} else {
			break
		}
	}
	if err != nil {
		l.WithError(err).Error("failed to create gcm client.")
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
	parsedTopic := ParsedTopic{}
	g.inflightMessagesMetadataLock.Lock()
	if val, ok := g.InflightMessagesMetadata[cm.MessageID]; ok {
		ccsMessageWithMetadata.Metadata = val.(map[string]interface{})
		ccsMessageWithMetadata.Timestamp = ccsMessageWithMetadata.Metadata["timestamp"].(int64)
		parsedTopic.Game = ccsMessageWithMetadata.Metadata["game"].(string)
		parsedTopic.Platform = ccsMessageWithMetadata.Metadata["platform"].(string)
		delete(ccsMessageWithMetadata.Metadata, "timestamp")
		delete(g.InflightMessagesMetadata, cm.MessageID)
	}
	g.inflightMessagesMetadataLock.Unlock()

	if cm.Error != "" {
		gcmResMutex.Lock()
		g.failuresReceived++
		gcmResMutex.Unlock()
		pErr := errors.NewPushError(strings.ToLower(cm.Error), cm.ErrorDescription)
		statsReporterHandleNotificationFailure(g.StatsReporters, parsedTopic.Game, "gcm", pErr)

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
		case "INVALID_JSON":
			l.WithFields(log.Fields{
				"category":   "JsonError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
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
		case "CONNECTION_DRAINING":
			l.WithFields(log.Fields{
				"category":   "ConnectionDrainingError",
				log.ErrorKey: cm.Error,
			}).Debug("received an error")
		default:
			l.WithFields(log.Fields{
				"category":   "DefaultError",
				log.ErrorKey: cm.Error,
			}).Debug("received an error")
		}
		sendFeedbackErr := sendToFeedbackReporters(g.feedbackReporters, ccsMessageWithMetadata, parsedTopic)
		if sendFeedbackErr != nil {
			l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
		}
		return err
	}

	sendFeedbackErr := sendToFeedbackReporters(g.feedbackReporters, ccsMessageWithMetadata, parsedTopic)
	if sendFeedbackErr != nil {
		l.WithError(sendFeedbackErr).Error("error sending feedback to reporter")
	}

	gcmResMutex.Lock()
	g.successesReceived++
	gcmResMutex.Unlock()
	statsReporterHandleNotificationSuccess(g.StatsReporters, parsedTopic.Game, "gcm")

	return nil
}

func (g *GCMMessageHandler) sendMessage(message interfaces.KafkaMessage) error {
	l := g.Logger.WithField("method", "sendMessage")
	//ttl := uint(0)
	km := KafkaGCMMessage{}
	err := json.Unmarshal(message.Value, &km)
	if err != nil {
		l.WithError(err).Error("Error unmarshaling message.")
		return err
	}
	if km.PushExpiry > 0 && km.PushExpiry < makeTimestamp() {
		l.Warnf("ignoring push message because it has expired: %s", km.Data)
		g.ignoredMessages++
		if g.pendingMessagesWG != nil {
			g.pendingMessagesWG.Done()
		}
		return nil
	}

	if km.Metadata != nil {
		if km.XMPPMessage.Data == nil {
			km.XMPPMessage.Data = map[string]interface{}{}
		}

		for k, v := range km.Metadata {
			if km.XMPPMessage.Data[k] == nil {
				km.XMPPMessage.Data[k] = v
			}
		}
	}

	l.WithField("message", km).Debug("sending message to gcm")
	var messageID string
	var bytes int

	g.pendingMessages <- true
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

		km.Metadata["timestamp"] = time.Now().Unix()
		hostname, err := os.Hostname()
		if err != nil {
			l.WithError(err).Error("error retrieving hostname")
		} else {
			km.Metadata["hostname"] = hostname
		}

		km.Metadata["game"] = message.Game
		km.Metadata["platform"] = "gcm"

		g.inflightMessagesMetadataLock.Lock()
		g.InflightMessagesMetadata[messageID] = km.Metadata
		g.requestsHeap.AddRequest(messageID)
		g.inflightMessagesMetadataLock.Unlock()
	}

	statsReporterHandleNotificationSent(g.StatsReporters, message.Game, "gcm")
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
func (g *GCMMessageHandler) HandleMessages(msg interfaces.KafkaMessage) {
	g.sendMessage(msg)
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
		if g.sentMessages > 0 || g.responsesReceived > 0 || g.ignoredMessages > 0 || g.successesReceived > 0 || g.failuresReceived > 0 {
			l.WithFields(log.Fields{
				"sentMessages":      g.sentMessages,
				"responsesReceived": g.responsesReceived,
				"ignoredMessages":   g.ignoredMessages,
				"successesReceived": g.successesReceived,
				"failuresReceived":  g.failuresReceived,
			}).Info("flushing stats")
			g.sentMessages = 0
			g.responsesReceived = 0
			g.successesReceived = 0
			g.ignoredMessages = 0
			g.failuresReceived = 0
		}
		apnsResMutex.Unlock()
	}
}

// Cleanup closes connections to GCM
func (g *GCMMessageHandler) Cleanup() error {
	err := g.GCMClient.Close()
	if err != nil {
		return err
	}
	return nil
}
