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
	GCMClient         interfaces.GCMClient
	PingInterval      int
	PingTimeout       int
	pendingMessagesWG *sync.WaitGroup
	StatsReporters    []interfaces.StatsReporter
	feedbackReporters []interfaces.FeedbackReporter
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
		apiKey:            apiKey,
		appName:           appName,
		ConfigFile:        configFile,
		IsProduction:      isProduction,
		Logger:            logger,
		responsesReceived: 0,
		senderID:          senderID,
		sentMessages:      0,
		pendingMessagesWG: pendingMessagesWG,
		StatsReporters:    statsReporters,
		feedbackReporters: feedbackReporters,
	}
	err := g.configure(client)
	if err != nil {
		l.Error("Failed to create a new GCM Message handler.")
		return nil, err
	}
	return g, nil
}

func (g *GCMMessageHandler) sendToFeedbackReporters(res *gcm.CCSMessage) error {
	jres, err := json.Marshal(res)
	if err != nil {
		return err
	}
	if g.feedbackReporters != nil {
		for _, feedbackReporter := range g.feedbackReporters {
			feedbackReporter.SendFeedback(jres)
		}
	}
	return nil
}

func (g *GCMMessageHandler) handleGCMResponse(cm gcm.CCSMessage) error {
	l := g.Logger.WithFields(log.Fields{
		"method":     "handleGCMResponse",
		"ccsMessage": cm,
	})
	l.Debug("Got response from gcm.")
	gcmResMutex.Lock()
	g.responsesReceived++
	if g.responsesReceived%1000 == 0 {
		l.Infof("received responses: %d", g.responsesReceived)
	}
	gcmResMutex.Unlock()

	var err error
	err = g.sendToFeedbackReporters(&cm)
	if err != nil {
		l.Errorf("error sending feedback to reporter: %v", err)
	}
	if cm.Error != "" {
		pErr := errors.NewPushError(strings.ToLower(cm.Error), cm.ErrorDescription)
		g.statsReporterHandleNotificationFailure(pErr)

		err = pErr
		switch cm.Error {
		// errors from https://developers.google.com/cloud-messaging/xmpp-server-ref table 4
		case "DEVICE_UNREGISTERED", "BAD_REGISTRATION":
			l.WithFields(log.Fields{
				"category":   "TokenError",
				log.ErrorKey: fmt.Errorf("%s (Description: %s)", cm.Error, cm.ErrorDescription),
			}).Debug("received an error")
			tErr := g.handleTokenError(cm.From)
			if tErr != nil {
				return tErr
			}
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

	g.statsReporterHandleNotificationSuccess()

	if g.pendingMessagesWG != nil {
		g.pendingMessagesWG.Done()
	}

	return nil
}

func (g *GCMMessageHandler) handleTokenError(token string) error {
	l := g.Logger.WithFields(log.Fields{
		"method": "handleTokenError",
		"token":  token,
	})
	// TODO: before deleting send deleted token info to another queue/db
	// TODO: if the above is not that specific move this to an util so it can be reused in apns
	l.Debug("Deleting token...")
	query := fmt.Sprintf("DELETE FROM %s_gcm WHERE token = ?0;", g.appName)
	_, err := g.PushDB.DB.Exec(query, token)
	if err != nil && err.Error() != "pg: no rows in result set" {
		l.WithError(err).Error("error deleting token")
		return err
	}
	return nil
}

func (g *GCMMessageHandler) loadConfigurationDefaults() {
	viper.SetDefault("gcm.pingInterval", 20)
	viper.SetDefault("gcm.pingTimeout", 30)
}

func (g *GCMMessageHandler) configure(client interfaces.GCMClient) error {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	err := g.configurePushDatabase()
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

func (g *GCMMessageHandler) configureGCMClient() error {
	l := g.Logger.WithFields(log.Fields{
		"method": "configureGCMClient",
	})

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
	cl, err := gcm.NewClient(gcmConfig, g.handleGCMResponse)
	if err != nil {
		l.Error("Failed to create gcm client.")
		return err
	}
	g.GCMClient = cl
	return nil
}

func (g *GCMMessageHandler) configurePushDatabase() error {
	l := g.Logger.WithField("method", "configurePushDatabase")
	var err error
	g.PushDB, err = NewPGClient("push.db", g.Config)
	if err != nil {
		l.WithError(err).Error("Failed to configure push database.")
		return err
	}
	return nil
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
	if err != nil {
		l.WithError(err).Error("Error unmarshaling message.")
		return err
	}
	l.WithField("message", m).Debug("sending message to gcm")
	var messageID string
	var bytes int
	messageID, bytes, err = g.GCMClient.SendXMPP(m)

	if err != nil {
		l.WithError(err).Error("Error sending message.")
		return err
	}

	g.statsReporterHandleNotificationSent()
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
	err := g.GCMClient.Close()
	if err != nil {
		return err
	}
	return nil
}

func (g *GCMMessageHandler) statsReporterHandleNotificationSent() {
	for _, statsReporter := range g.StatsReporters {
		statsReporter.HandleNotificationSent()
	}
}

func (g *GCMMessageHandler) statsReporterHandleNotificationSuccess() {
	for _, statsReporter := range g.StatsReporters {
		statsReporter.HandleNotificationSuccess()
	}
}

func (g *GCMMessageHandler) statsReporterHandleNotificationFailure(err *errors.PushError) {
	for _, statsReporter := range g.StatsReporters {
		statsReporter.HandleNotificationFailure(err)
	}
}
