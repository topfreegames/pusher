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
	"sync"

	log "github.com/Sirupsen/logrus"
	gcm "github.com/rounds/go-gcm"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/util"
)

var gcmResMutex sync.Mutex

// GCMMessageHandler implements the messagehandler interface
type GCMMessageHandler struct {
	apiKey            string
	appName           string
	Config            *viper.Viper
	ConfigFile        string
	IsProduction      bool
	Logger            *log.Logger
	PushDB            *PGClient
	responsesReceived int64
	run               bool
	senderID          string
	sentMessages      int64
	GCMClient         gcm.Client
	PingInterval      int
	PingTimeout       int
	pendingMessagesWG *sync.WaitGroup
}

func (g *GCMMessageHandler) handleGCMResponse(cm gcm.CCSMessage) error {
	l := g.Logger.WithFields(log.Fields{
		"method":     "handleGCMResponse",
		"ccsMessage": cm,
	})
	l.Debug("got response from gcm")
	gcmResMutex.Lock()
	g.responsesReceived++
	if g.responsesReceived%1000 == 0 {
		l.Infof("received responses: %d", g.responsesReceived)
	}
	gcmResMutex.Unlock()
	if cm.Error != "" {
		switch cm.Error {
		// errors from https://developers.google.com/cloud-messaging/xmpp-server-ref table 4
		case "DEVICE_UNREGISTERED", "BAD_REGISTRATION":
			l.WithFields(log.Fields{
				"category":   "TokenError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Error("received an error")
			g.handleTokenError(cm.From)
		case "INVALID_JSON":
			l.WithFields(log.Fields{
				"category":   "JsonError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Error("received an error")
		case "SERVICE_UNAVAILABLE", "INTERNAL_SERVER_ERROR":
			l.WithFields(log.Fields{
				"category":   "GoogleError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Error("received an error")
		case "DEVICE_MESSAGE_RATE_EXCEEDED", "TOPICS_MESSAGE_RATE_EXCEEDED":
			l.WithFields(log.Fields{
				"category":   "RateExceededError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Error("received an error")
		default:
			l.WithFields(log.Fields{
				"category":   "DefaultError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Error("received an error")
		}
	}
	if g.pendingMessagesWG != nil {
		g.pendingMessagesWG.Done()
	}
	return nil
}

// NewGCMMessageHandler returns a new instance of a GCMMessageHandler
func NewGCMMessageHandler(configFile, senderID, apiKey, appName string, isProduction bool, logger *log.Logger, pendingMessagesWG *sync.WaitGroup) *GCMMessageHandler {
	g := &GCMMessageHandler{
		apiKey:            apiKey,
		appName:           appName,
		ConfigFile:        configFile,
		IsProduction:      isProduction,
		Logger:            logger,
		responsesReceived: 0,
		senderID:          senderID,
		sentMessages:      0,
		pendingMessagesWG: pendingMessagesWG,
	}
	g.configure()
	return g
}

func (g *GCMMessageHandler) handleTokenError(token string) {
	l := g.Logger.WithFields(log.Fields{
		"method": "handleTokenError",
		"token":  token,
	})
	// TODO: before deleting send deleted token info to another queue/db
	// TODO: if the above is not that specific move this to an util so it can be reused in apns
	// TODO: should we really delete the token? or move them to another table?
	l.Info("deleting token")
	query := fmt.Sprintf("DELETE FROM %s_gcm WHERE token = '%s';", g.appName, token)
	_, err := g.PushDB.DB.Exec(query)
	if err != nil && err.Error() != "pg: no rows in result set" {
		l.WithError(err).Error("error deleting token")
	}
}

func (g *GCMMessageHandler) loadConfigurationDefaults() {
	viper.SetDefault("gcm.pingInterval", 20)
	viper.SetDefault("gcm.pingTimeout", 30)
}

func (g *GCMMessageHandler) configure() {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	g.configurePushDatabase()
	g.configureGCMClient()
}

func (g *GCMMessageHandler) configureGCMClient() {
	g.PingInterval = viper.GetInt("gcm.pingInterval")
	g.PingTimeout = viper.GetInt("gcm.pingTimeout")
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
	g.GCMClient, err = gcm.NewClient(gcmConfig, g.handleGCMResponse)
	if err != nil {
		log.Panicf("error creating gcm client, error: %s", err)
	}
}

func (g *GCMMessageHandler) configurePushDatabase() {
	l := g.Logger.WithField("method", "configurePushDatabase")
	var err error
	g.PushDB, err = NewPGClient("push.db", g.Config)
	if err != nil {
		l.WithError(err).Panic("could not connect to push database")
	}
}

func (g *GCMMessageHandler) sendMessage(message []byte) error {
	l := g.Logger.WithField("method", "sendMessage")
	ttl := uint(0)
	m := gcm.XMPPMessage{
		TimeToLive:               &ttl,
		DelayWhileIdle:           false,
		DeliveryReceiptRequested: false,
		DryRun: true,
	}
	err := json.Unmarshal(message, &m)
	l.WithField("message", m).Debug("sending message to gcm")
	var messageID string
	var bytes int
	messageID, bytes, err = g.GCMClient.SendXMPP(m)

	//TODO tratar o erro?
	if err != nil {
		l.WithError(err).Error("error sending message")
	} else {
		g.sentMessages++
		l.WithFields(log.Fields{
			"messageID": messageID,
			"bytes":     bytes,
		}).Debug("sent message")
		if g.sentMessages%1000 == 0 {
			l.Infof("sent messages: %d", g.sentMessages)
		}
	}
	return err
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
