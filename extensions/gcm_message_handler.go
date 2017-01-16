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
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	gcm "github.com/rounds/go-gcm"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
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
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// GCMMessageHandler implements the messagehandler interface
type GCMMessageHandler struct {
	apiKey                   string
	appName                  string
	Config                   *viper.Viper
	ConfigFile               string
	feedbackReporters        []interfaces.FeedbackReporter
	GCMClient                interfaces.GCMClient
	InflightMessagesMetadata map[string]interface{}
	IsProduction             bool
	Logger                   *log.Logger
	pendingMessages          chan bool
	pendingMessagesWG        *sync.WaitGroup
	PingInterval             int
	PingTimeout              int
	PushDB                   *PGClient
	responsesReceived        int64
	run                      bool
	senderID                 string
	sentMessages             int64
	StatsReporters           []interfaces.StatsReporter
}

// NewGCMMessageHandler returns a new instance of a GCMMessageHandler
func NewGCMMessageHandler(
	configFile, senderID, apiKey, appName string,
	isProduction bool,
	logger *log.Logger,
	pendingMessagesWG *sync.WaitGroup,
	statsReporters []interfaces.StatsReporter,
	feedbackReporters []interfaces.FeedbackReporter,
	client interfaces.GCMClient,
	db interfaces.DB,
) (*GCMMessageHandler, error) {
	l := logger.WithFields(log.Fields{
		"method":       "NewGCMMessageHandler",
		"configFile":   configFile,
		"senderID":     senderID,
		"apiKey":       apiKey,
		"appName":      appName,
		"isProduction": isProduction,
	})

	g := &GCMMessageHandler{
		apiKey:                   apiKey,
		appName:                  appName,
		ConfigFile:               configFile,
		InflightMessagesMetadata: map[string]interface{}{},
		IsProduction:             isProduction,
		Logger:                   logger,
		responsesReceived:        0,
		senderID:                 senderID,
		sentMessages:             0,
		pendingMessagesWG:        pendingMessagesWG,
		StatsReporters:           statsReporters,
		feedbackReporters:        feedbackReporters,
	}
	err := g.configure(client, db)
	if err != nil {
		l.Error("Failed to create a new GCM Message handler.")
		return nil, err
	}
	return g, nil
}

func (g *GCMMessageHandler) configure(client interfaces.GCMClient, db interfaces.DB) error {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	g.pendingMessages = make(chan bool, g.Config.GetInt("gcm.maxPendingMessages"))
	err := g.configurePushDatabase(db)
	if err != nil {
		return err
	}
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
}

func (g *GCMMessageHandler) configurePushDatabase(db interfaces.DB) error {
	l := g.Logger.WithField("method", "configurePushDatabase")
	var err error
	g.PushDB, err = NewPGClient("push.db", g.Config, db)
	if err != nil {
		l.WithError(err).Error("Failed to configure push database.")
		return err
	}
	return nil
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
	if g.responsesReceived%1000 == 0 {
		l.Infof("received responses: %d", g.responsesReceived)
	}
	gcmResMutex.Unlock()

	var err error
	ccsMessageWithMetadata := &CCSMessageWithMetadata{
		CCSMessage: cm,
	}
	inflightMessagesMetadataLock.Lock()
	if val, ok := g.InflightMessagesMetadata[cm.MessageID]; ok {
		ccsMessageWithMetadata.Metadata = val.(map[string]interface{})
		delete(g.InflightMessagesMetadata, cm.MessageID)
	}
	inflightMessagesMetadataLock.Unlock()

	err = sendToFeedbackReporters(g.feedbackReporters, ccsMessageWithMetadata)
	if err != nil {
		l.Errorf("error sending feedback to reporter: %v", err)
	}
	if cm.Error != "" {
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
			handleTokenError(cm.From, "gcm", g.appName, g.Logger, g.PushDB.DB)
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

		return err
	}

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
		if km.Metadata != nil && len(km.Metadata) > 0 {
			inflightMessagesMetadataLock.Lock()
			g.InflightMessagesMetadata[messageID] = km.Metadata
			inflightMessagesMetadataLock.Unlock()
		}
	}

	statsReporterHandleNotificationSent(g.StatsReporters)
	g.sentMessages++
	l.WithFields(log.Fields{
		"messageID": messageID,
		"bytes":     bytes,
	}).Debug("sent message")
	if g.sentMessages%1000 == 0 {
		l.Infof("sent messages: %d", g.sentMessages)
	}
	return nil
}

// HandleResponses from gcm
func (g *GCMMessageHandler) HandleResponses() {
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

//Cleanup closes connections to GCM
func (g *GCMMessageHandler) Cleanup() error {
	err := g.PushDB.Close()
	if err != nil {
		return err
	}

	err = g.GCMClient.Close()
	if err != nil {
		return err
	}
	return nil
}
