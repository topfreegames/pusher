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
	Environment       string
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
func NewAPNSMessageHandler(configFile, certificatePath, environment, appName string, logger *logrus.Logger) *APNSMessageHandler {
	a := &APNSMessageHandler{
		appName:           appName,
		CertificatePath:   certificatePath,
		ConfigFile:        configFile,
		Environment:       environment,
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
	l.Debugf("deleting token")
	query := fmt.Sprintf("DELETE FROM %s_apns WHERE token = '%s';", a.appName, token)
	_, err := a.PushDB.DB.Exec(query)
	if err != nil {
		l.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorf("error deleting token")
	}
}

func (a *APNSMessageHandler) handleAPNSResponse(res push.Response) error {
	l := a.Logger.WithFields(logrus.Fields{
		"apns res": res,
	})
	l.Debugf("got response from apns")
	apnsResMutex.Lock()
	a.responsesReceived++
	if a.responsesReceived%1000 == 0 {
		l.Infof("received %d responses", a.responsesReceived)
	}
	apnsResMutex.Unlock()
	if res.Err != nil {
		switch res.Err.(*push.Error).Reason {
		case push.ErrMissingDeviceToken, push.ErrBadDeviceToken:
			l.WithFields(logrus.Fields{
				"category": "TokenError",
			}).Errorf("received an error: %s", res.Err.Error())
			a.handleTokenError(res.DeviceToken)
		case push.ErrBadCertificate, push.ErrBadCertificateEnvironment, push.ErrForbidden:
			l.WithFields(logrus.Fields{
				"category": "CertificateError",
			}).Errorf("received an error: %s", res.Err.Error())
		case push.ErrMissingTopic, push.ErrTopicDisallowed, push.ErrDeviceTokenNotForTopic:
			l.WithFields(logrus.Fields{
				"category": "TopicError",
			}).Errorf("received an error: %s", res.Err.Error())
		case push.ErrIdleTimeout, push.ErrShutdown, push.ErrInternalServerError, push.ErrServiceUnavailable:
			l.WithFields(logrus.Fields{
				"category": "AppleError",
			}).Errorf("received an error: %s", res.Err.Error())
		default:
			l.WithFields(logrus.Fields{
				"category": "DefaultError",
			}).Errorf("received an error: %s", res.Err.Error())
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
	c, err := certificate.FromPemFile(a.CertificatePath, "")
	if err != nil {
		a.Logger.Panicf("error loading pem certificate: %s", err.Error())
	}
	a.Logger.Debugf("loaded apns certificate: %s", c)
	a.certificate = c
	a.Topic = cert.TopicFromCert(c)
}

func (a *APNSMessageHandler) configureAPNSPushQueue() {
	client, err := push.NewClient(a.certificate)
	if err != nil {
		a.Logger.Panicf("could not create apns client: %s", err.Error())
	}
	// TODO is production flag
	var svc *push.Service
	switch a.Environment {
	case "production":
		svc = push.NewService(client, push.Production)
		break
	case "development":
		svc = push.NewService(client, push.Development)
		break
	default:
		a.Logger.Panicf("invalid environment")
	}
	// TODO needs to be configurable
	concurrentWorkers := a.Config.GetInt("apns.concurrentWorkers")
	a.Logger.Debugf("creating apns queue with %d workers", concurrentWorkers)
	workers := uint(concurrentWorkers)
	queue := push.NewQueue(svc, workers)
	a.PushQueue = queue
}

func (a *APNSMessageHandler) configurePushDatabase() {
	var err error
	a.PushDB, err = NewPGClient("push.db", a.Config)
	if err != nil {
		a.Logger.Panicf("could not connect to push database: %s", err.Error())
	}
}

func (a *APNSMessageHandler) sendMessage(message []byte) {
	a.Logger.WithFields(logrus.Fields{
		"message": string(message),
	}).Debugf("sending message to apns")
	h := &push.Headers{
		Topic: a.Topic,
	}
	n := &Notification{}
	json.Unmarshal(message, n)
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		a.Logger.Errorf("error marshaling message payload: %s", err.Error())
	}
	a.PushQueue.Push(n.DeviceToken, h, payload)
	a.sentMessages++
	if a.sentMessages%1000 == 0 {
		a.Logger.Infof("sent %d messages", a.sentMessages)
	}
}

// HandleResponses from apns
func (a *APNSMessageHandler) HandleResponses() {
	for resp := range a.PushQueue.Responses {
		a.handleAPNSResponse(resp)
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
