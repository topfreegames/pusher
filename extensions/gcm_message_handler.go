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

	"github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/go-gcm"
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
	Logger            *logrus.Logger
	PushDB            *PGClient
	responsesReceived int64
	run               bool
	senderID          string
	sentMessages      int64
}

func (g *GCMMessageHandler) handleGCMResponse(pendingMessagesWG *sync.WaitGroup) func(gcm.CcsMessage) error {
	return func(cm gcm.CcsMessage) error {
		l := g.Logger.WithFields(logrus.Fields{
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
				l.WithFields(logrus.Fields{
					"category":      "TokenError",
					logrus.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
				}).Error("received an error")
				g.handleTokenError(cm.From)
			case "INVALID_JSON":
				l.WithFields(logrus.Fields{
					"category":      "JsonError",
					logrus.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
				}).Error("received an error")
			case "SERVICE_UNAVAILABLE", "INTERNAL_SERVER_ERROR":
				l.WithFields(logrus.Fields{
					"category":      "GoogleError",
					logrus.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
				}).Error("received an error")
			case "DEVICE_MESSAGE_RATE_EXCEEDED", "TOPICS_MESSAGE_RATE_EXCEEDED":
				l.WithFields(logrus.Fields{
					"category":      "RateExceededError",
					logrus.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
				}).Error("received an error")
			default:
				l.WithFields(logrus.Fields{
					"category":      "DefaultError",
					logrus.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
				}).Error("received an error")
			}
		}
		if pendingMessagesWG != nil {
			pendingMessagesWG.Done()
		}
		return nil
	}
}

// NewGCMMessageHandler returns a new instance of a GCMMessageHandler
func NewGCMMessageHandler(configFile, senderID, apiKey, appName string, isProduction bool, logger *logrus.Logger) *GCMMessageHandler {
	g := &GCMMessageHandler{
		apiKey:            apiKey,
		appName:           appName,
		ConfigFile:        configFile,
		IsProduction:      isProduction,
		Logger:            logger,
		responsesReceived: 0,
		senderID:          senderID,
		sentMessages:      0,
	}
	g.configure()
	return g
}

func (g *GCMMessageHandler) handleTokenError(token string) {
	l := g.Logger.WithFields(logrus.Fields{
		"method": "handleTokenError",
		"token":  token,
	})
	// TODO: before deleting send deleted token info to another queue/db
	// TODO: if the above is not that specific move this to an util so it can be reused in apns
	l.Info("deleting token")
	query := fmt.Sprintf("DELETE FROM %s_gcm WHERE token = '%s';", g.appName, token)
	_, err := g.PushDB.DB.Exec(query)
	if err != nil && err.Error() != "pg: no rows in result set" {
		l.WithError(err).Error("error deleting token")
	}
}

func (g *GCMMessageHandler) loadConfigurationDefaults() {
}

func (g *GCMMessageHandler) configure() {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	g.configurePushDatabase()
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
	m := gcm.XmppMessage{
		TimeToLive:               &ttl,
		DelayWhileIdle:           false,
		DeliveryReceiptRequested: false,
		DryRun: true,
	}
	err := json.Unmarshal(message, &m)
	l.WithField("message", m).Debug("sending message to gcm")
	var messageID string
	var bytes int
	if g.IsProduction {
		messageID, bytes, err = gcm.SendXmpp(g.senderID, g.apiKey, m)
	} else {
		messageID, bytes, err = gcm.SendXmppStaging(g.senderID, g.apiKey, m)
	}

	//TODO tratar o erro?
	if err != nil {
		l.WithError(err).Error("error sending message")
	} else {
		g.sentMessages++
		l.WithFields(logrus.Fields{
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
func (g *GCMMessageHandler) HandleResponses(pendingMessagesWG *sync.WaitGroup) {
	if g.IsProduction {
		gcm.Listen(g.senderID, g.apiKey, g.handleGCMResponse(pendingMessagesWG), nil)
	} else {
		gcm.ListenStaging(g.senderID, g.apiKey, g.handleGCMResponse(pendingMessagesWG), nil)
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
