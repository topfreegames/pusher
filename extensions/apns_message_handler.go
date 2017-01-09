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
	"fmt"
	"sync"

	cert "github.com/RobotsAndPencils/buford/certificate"
	"github.com/RobotsAndPencils/buford/push"
	"github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/certificate"
	"github.com/topfreegames/pusher/util"
)

var apnsResMutex sync.Mutex

// APNSMessageHandler implements the messagehandler interface
type APNSMessageHandler struct {
	appName           string
	certificate       tls.Certificate
	CertificatePath   string
	Config            *viper.Viper
	ConfigFile        string
	IsProduction      bool
	Logger            *logrus.Logger
	PushDB            *PGClient
	PushQueue         *push.Queue
	responsesReceived int64
	run               bool
	sentMessages      int64
	Topic             string
}

// Notification is the notification base struct
type Notification struct {
	DeviceToken string
	Payload     interface{}
}

// NewAPNSMessageHandler returns a new instance of a APNSMessageHandler
func NewAPNSMessageHandler(configFile, certificatePath, appName string, isProduction bool, logger *logrus.Logger) *APNSMessageHandler {
	a := &APNSMessageHandler{
		appName:           appName,
		CertificatePath:   certificatePath,
		ConfigFile:        configFile,
		IsProduction:      isProduction,
		Logger:            logger,
		responsesReceived: 0,
		sentMessages:      0,
	}
	a.configure()
	return a
}

func (a *APNSMessageHandler) handleTokenError(token string) {
	l := a.Logger.WithFields(logrus.Fields{
		"method": "handleTokenError",
		"token":  token,
	})
	// TODO: before deleting send deleted token info to another queue/db
	l.Info("deleting token")
	query := fmt.Sprintf("DELETE FROM %s_apns WHERE token = '%s';", a.appName, token)
	_, err := a.PushDB.DB.Exec(query)
	if err != nil && err.Error() != "pg: no rows in result set" {
		l.WithError(err).Error("error deleting token")
	}
}

func (a *APNSMessageHandler) handleAPNSResponse(res push.Response) error {
	l := a.Logger.WithFields(logrus.Fields{
		"method": "handleAPNSResponse",
		"res":    res,
	})
	l.Debug("got response from apns")
	apnsResMutex.Lock()
	a.responsesReceived++
	if a.responsesReceived%1000 == 0 {
		l.Infof("received responses: %d", a.responsesReceived)
	}
	apnsResMutex.Unlock()
	if res.Err != nil {
		switch res.Err.(*push.Error).Reason {
		case push.ErrMissingDeviceToken, push.ErrBadDeviceToken:
			l.WithFields(logrus.Fields{
				"category":      "TokenError",
				logrus.ErrorKey: res.Err,
			}).Error("received an error")
			a.handleTokenError(res.DeviceToken)
		case push.ErrBadCertificate, push.ErrBadCertificateEnvironment, push.ErrForbidden:
			l.WithFields(logrus.Fields{
				"category":      "CertificateError",
				logrus.ErrorKey: res.Err,
			}).Error("received an error")
		case push.ErrMissingTopic, push.ErrTopicDisallowed, push.ErrDeviceTokenNotForTopic:
			l.WithFields(logrus.Fields{
				"category":      "TopicError",
				logrus.ErrorKey: res.Err,
			}).Error("received an error")
		case push.ErrIdleTimeout, push.ErrShutdown, push.ErrInternalServerError, push.ErrServiceUnavailable:
			l.WithFields(logrus.Fields{
				"category":      "AppleError",
				logrus.ErrorKey: res.Err,
			}).Error("received an error")
		default:
			l.WithFields(logrus.Fields{
				"category":      "DefaultError",
				logrus.ErrorKey: res.Err,
			}).Error("received an error")
		}
	}
	return nil
}

func (a *APNSMessageHandler) loadConfigurationDefaults() {
	a.Config.SetDefault("apns.concurrentWorkers", 10)
}

func (a *APNSMessageHandler) configure() {
	a.Config = util.NewViperWithConfigFile(a.ConfigFile)
	a.loadConfigurationDefaults()
	a.configureCertificate()
	a.configureAPNSPushQueue()
	a.configurePushDatabase()
}

func (a *APNSMessageHandler) configureCertificate() {
	l := a.Logger.WithField("method", "configureCertificate")
	c, err := certificate.FromPemFile(a.CertificatePath, "")
	if err != nil {
		l.WithError(err).Panic("error loading pem certificate")
	}
	a.certificate = c
	a.Topic = cert.TopicFromCert(c)
	l.WithField("topic", a.Topic).Debug("loaded apns certificate")
}

func (a *APNSMessageHandler) configureAPNSPushQueue() {
	l := a.Logger.WithField("method", "configureAPNSPushQueue")
	client, err := push.NewClient(a.certificate)
	if err != nil {
		l.WithError(err).Panic("could not create apns client")
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
}

func (a *APNSMessageHandler) configurePushDatabase() {
	l := a.Logger.WithField("method", "configurePushDatabase")
	var err error
	a.PushDB, err = NewPGClient("push.db", a.Config)
	if err != nil {
		l.WithError(err).Panic("could not connect to push database")
	}
}

func (a *APNSMessageHandler) sendMessage(message []byte) {
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
	}
	a.PushQueue.Push(n.DeviceToken, h, payload)
	a.sentMessages++
	if a.sentMessages%1000 == 0 {
		l.Infof("sent messages: %d", a.sentMessages)
	}
}

// HandleResponses from apns
func (a *APNSMessageHandler) HandleResponses(pendingMessagesWG *sync.WaitGroup) {
	for resp := range a.PushQueue.Responses {
		a.handleAPNSResponse(resp)
		if pendingMessagesWG != nil {
			pendingMessagesWG.Done()
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
