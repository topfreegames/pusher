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
	"github.com/sideshow/apns2"
	token "github.com/sideshow/apns2/token"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/structs"
)

// APNSPushQueue implements interfaces.APNSPushQueue
type APNSPushQueue struct {
	authKeyPath     string
	keyID           string
	teamID          string
	token           *token.Token
	pushChannel     chan *structs.ApnsNotification
	responseChannel chan *structs.ResponseWithMetadata
	Logger          *log.Logger
	Config          *viper.Viper
	clients         chan *apns2.Client
	IsProduction    bool
	Closed          bool
}

// NewAPNSPushQueue returns a new instance of a APNSPushQueue
func NewAPNSPushQueue(
	authKeyPath, keyID,
	teamID string,
	isProduction bool,
	logger *log.Logger,
	config *viper.Viper,
) *APNSPushQueue {
	return &APNSPushQueue{
		authKeyPath:  authKeyPath,
		keyID:        keyID,
		teamID:       teamID,
		Logger:       logger,
		Config:       config,
		IsProduction: isProduction,
	}
}

func (p *APNSPushQueue) loadConfigDefault() {
	p.Config.SetDefault("apns.connectionPoolSize", 1)
	p.Config.SetDefault("apns.pushQueueSize", 100)
	p.Config.SetDefault("apns.responseChannelSize", 100)
}

// Configure configures queues and token
func (p *APNSPushQueue) Configure() error {
	l := p.Logger.WithFields(log.Fields{
		"source": "APNSPushQueue",
		"method": "configure",
	})
	err := p.configureCertificate()
	if err != nil {
		return err
	}
	p.Closed = false
	p.loadConfigDefault()
	connectionPoolSize := p.Config.GetInt("apns.connectionPoolSize")
	pushQueueSize := p.Config.GetInt("apns.pushQueueSize")
	respChannelSize := p.Config.GetInt("apns.responseChannelSize")
	p.clients = make(chan *apns2.Client, connectionPoolSize)
	for i := 0; i < connectionPoolSize; i++ {
		client := apns2.NewTokenClient(p.token)
		if p.IsProduction {
			l.Debug("using production")
			client = client.Production()
		} else {
			l.Debug("using development")
			client = client.Development()
		}
		p.clients <- client
	}
	l.Debug("clients configured")
	p.pushChannel = make(chan *structs.ApnsNotification, pushQueueSize)
	p.responseChannel = make(chan *structs.ResponseWithMetadata, respChannelSize)

	for i := 0; i < p.Config.GetInt("apns.concurrentWorkers"); i++ {
		go p.pushWorker()
	}
	return nil
}

func (p *APNSPushQueue) configureCertificate() error {
	l := p.Logger.WithField("method", "configureCertificate")
	authKey, err := token.AuthKeyFromFile(p.authKeyPath)
	if err != nil {
		l.WithError(err).Error("token error")
		return err
	}
	p.token = &token.Token{
		AuthKey: authKey,
		// KeyID from developer account (Certificates, Identifiers & Profiles -> Keys)
		KeyID: p.keyID,
		// TeamID from developer account (View Account -> Membership)
		TeamID: p.teamID,
	}
	l.Debug("token loaded")
	return nil
}

// ResponseChannel returns the response channel
func (p *APNSPushQueue) ResponseChannel() chan *structs.ResponseWithMetadata {
	return p.responseChannel
}

func (p *APNSPushQueue) pushWorker() {
	l := p.Logger.WithField("method", "pushWorker")

	for notification := range p.pushChannel {
		client := <-p.clients
		p.clients <- client

		apnsRes, err := client.Push(&notification.Notification)
		if err != nil {
			l.WithError(err).Error("push error")
		}
		if apnsRes == nil {
			continue
		}
		res := &structs.ResponseWithMetadata{
			StatusCode:   apnsRes.StatusCode,
			Reason:       apnsRes.Reason,
			ApnsID:       apnsRes.ApnsID,
			Sent:         apnsRes.Sent(),
			DeviceToken:  notification.DeviceToken,
			Notification: notification,
		}

		p.responseChannel <- res
	}
}

// Push sends the notification
func (p *APNSPushQueue) Push(notification *structs.ApnsNotification) {
	p.pushChannel <- notification
}

// Close closes all the open channels
func (p *APNSPushQueue) Close() {
	close(p.pushChannel)
	close(p.responseChannel)
	p.Closed = true
}
