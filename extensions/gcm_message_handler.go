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
	"github.com/google/go-gcm"
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
	Logger            *logrus.Logger
	PushDB            *PGClient
	responsesReceived int64
	run               bool
	senderID          string
	sentMessages      int64
}

// NewGCMMessageHandler returns a new instance of a GCMMessageHandler
func NewGCMMessageHandler(configFile, senderID, apiKey, appName string, logger *logrus.Logger) *GCMMessageHandler {
	g := &GCMMessageHandler{
		apiKey:            apiKey,
		appName:           appName,
		ConfigFile:        configFile,
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
	l.Debugf("deleting token")
	query := fmt.Sprintf("DELETE FROM %s_gcm WHERE token = '%s';", g.appName, token)
	_, err := g.PushDB.DB.Exec(query)
	if err != nil {
		l.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorf("error deleting token")
	}
}

func (g *GCMMessageHandler) handleGCMResponse(cm gcm.CcsMessage) error {
	l := g.Logger.WithFields(logrus.Fields{
		"ccsMessage": cm,
	})
	l.Debugf("got response from gcm")
	gcmResMutex.Lock()
	g.responsesReceived++
	if g.responsesReceived%1000 == 0 {
		l.Infof("received %d responses", g.responsesReceived)
	}
	gcmResMutex.Unlock()
	if cm.Error != "" {
		switch cm.Error {
		case "DEVICE_UNREGISTERED", "BAD_REGISTRATION":
			l.WithFields(logrus.Fields{
				"category": "TokenError",
			}).Errorf("received an error: %s. Description: %s.", cm.Error, cm.ErrorDescription)
			g.handleTokenError(cm.From)
		case "INVALID_JSON":
			l.WithFields(logrus.Fields{
				"category": "JsonError",
			}).Errorf("received an error: %s. Description: %s.", cm.Error, cm.ErrorDescription)
		case "SERVICE_UNAVAILABLE", "INTERNAL_SERVER_ERROR":
			l.WithFields(logrus.Fields{
				"category": "GoogleError",
			}).Errorf("received an error: %s. Description: %s.", cm.Error, cm.ErrorDescription)
		case "DEVICE_MESSAGE_RATE_EXCEEDED", "TOPICS_MESSAGE_RATE_EXCEEDED":
			l.WithFields(logrus.Fields{
				"category": "RateExceededError",
			}).Errorf("received an error: %s. Description: %s.", cm.Error, cm.ErrorDescription)
		default:
			l.WithFields(logrus.Fields{
				"category": "DefaultError",
			}).Errorf("received an error: %s. Description: %s.", cm.Error, cm.ErrorDescription)
		}
	}
	return nil
}

func (g *GCMMessageHandler) loadConfigurationDefaults() {
}

func (g *GCMMessageHandler) configure() {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	g.configurePushDatabase()
}

func (g *GCMMessageHandler) configurePushDatabase() {
	var err error
	g.PushDB, err = NewPGClient("push.db", g.Config)
	if err != nil {
		g.Logger.Panicf("could not connect to push database: %s", err.Error())
	}
}

func (g *GCMMessageHandler) sendMessage(message []byte) error {
	ttl := uint(0)
	m := gcm.XmppMessage{
		TimeToLive:               &ttl,
		DelayWhileIdle:           false,
		DeliveryReceiptRequested: false,
		DryRun: true,
	}
	err := json.Unmarshal(message, &m)
	l := g.Logger.WithFields(logrus.Fields{
		"message": m,
	})
	l.Debugf("sending message to gcm")
	messageID, bytes, err := gcm.SendXmpp(g.senderID, g.apiKey, m)
	//TODO tratar o erro?
	if err != nil {
		l.Errorf("error sending message: %s", err.Error())
	} else {
		g.sentMessages++
		l.Debugf("sendMessage return mid:%s bytes:%d err:%s", messageID, bytes)
		if g.sentMessages%1000 == 0 {
			l.Infof("sent %d messages", g.sentMessages)
		}
	}
	return err
}

// HandleResponses from gcm
func (g *GCMMessageHandler) HandleResponses() {
	gcm.Listen(g.senderID, g.apiKey, g.handleGCMResponse, nil)
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
