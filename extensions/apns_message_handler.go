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

	"github.com/RobotsAndPencils/buford/push"
	log "github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/certificate"
	"github.com/topfreegames/pusher/util"
)

var apnsResMutex sync.Mutex

// APNSMessageHandler implements the messagehandler interface
type APNSMessageHandler struct {
	ConfigFile        string
	Topic             string
	run               bool
	CertificatePath   string
	PushQueue         *push.Queue
	certificate       tls.Certificate
	Config            *viper.Viper
	Environment       string
	sentMessages      int64
	responsesReceived int64
}

// Notification is the notification base struct
type Notification struct {
	DeviceToken string
	Payload     interface{}
}

// NewAPNSMessageHandler returns a new instance of a APNSMessageHandler
func NewAPNSMessageHandler(configFile string, topic string, certificatePath string, environment string) *APNSMessageHandler {
	a := &APNSMessageHandler{
		ConfigFile:        configFile,
		CertificatePath:   certificatePath,
		Topic:             topic,
		sentMessages:      0,
		responsesReceived: 0,
		Environment:       environment,
	}
	a.configure()
	return a
}

func (a *APNSMessageHandler) handleAPNSResponse(res push.Response) error {
	l := log.WithFields(log.Fields{
		"apns res": res,
	})
	l.Debugf("got response from apns")
	apnsResMutex.Lock()
	a.responsesReceived++
	if a.responsesReceived%1000 == 0 {
		log.Infof("received %d responses", a.responsesReceived)
	}
	apnsResMutex.Unlock()
	//TODO do something with the response
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
}

func (a *APNSMessageHandler) configureCertificate() {
	c, err := certificate.FromPemFile(a.CertificatePath, "")
	if err != nil {
		log.Panicf("error loading pem certificate: %s", err.Error())
	}
	log.Debugf("loaded apns certificate: %s", c)
	a.certificate = c
}

func (a *APNSMessageHandler) configureAPNSPushQueue() {
	client, err := push.NewClient(a.certificate)
	if err != nil {
		log.Panicf("could not create apns client: %s", err.Error())
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
		log.Panicf("invalid environment")
	}
	// TODO needs to be configurable
	concurrentWorkers := a.Config.GetInt("apns.concurrentWorkers")
	log.Debugf("creating apns queue with %d workers", concurrentWorkers)
	workers := uint(concurrentWorkers)
	queue := push.NewQueue(svc, workers)
	a.PushQueue = queue
}

func (a *APNSMessageHandler) sendMessage(message []byte) {
	log.WithFields(log.Fields{
		"message": string(message),
	}).Debugf("sending message to apns")
	h := &push.Headers{
		Topic: a.Topic,
	}
	n := &Notification{}
	json.Unmarshal(message, n)
	payload, err := json.Marshal(n.Payload)
	if err != nil {
		log.Errorf("error marshaling message payload: %s", err.Error())
	}
	a.PushQueue.Push(n.DeviceToken, h, payload)
	a.sentMessages++
	if a.sentMessages%1000 == 0 {
		log.Infof("sent %d messages", a.sentMessages)
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
