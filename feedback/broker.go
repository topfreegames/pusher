/*
 * Copyright (c) 2019 TFG Co <backend@tfgco.com>
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

package feedback

import (
	"encoding/json"
	"sync"

	"github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/structs"

	"github.com/sideshow/apns2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Message is a struct that will decode an apns or gcm feedback message.
type Message struct {
	From             string                 `json:"from"`
	MessageID        string                 `json:"message_id"`
	MessageType      string                 `json:"message_type"`
	Error            string                 `json:"error"`
	ErrorDescription string                 `json:"error_description"`
	DeviceToken      string                 `json:"DeviceToken"`
	ID               string                 `json:"id"`
	Err              map[string]interface{} `json:"Err"`
	Metadata         map[string]interface{} `json:"metadata"`
	Reason           string                 `json:"reason"`
}

// Broker receives kafka messages in its InChan, unmarshal them according to the
// platform and routes them to the correct out channel after examining their content.
type Broker struct {
	StatsReporters      []interfaces.StatsReporter
	Logger              *log.Logger
	Config              *viper.Viper
	InChan              chan QueueMessage
	pendingMessagesWG   *sync.WaitGroup
	InvalidTokenOutChan chan *InvalidToken
	stopChannel         chan struct{}
	InvalidTokenEnabled bool
	run                 bool
}

// NewBroker creates a new Broker instance
func NewBroker(
	logger *log.Logger, cfg *viper.Viper, statsReporters []interfaces.StatsReporter,
	inChan chan QueueMessage,
	pendingMessagesWG *sync.WaitGroup,
) (*Broker, error) {
	b := &Broker{
		Logger:            logger,
		Config:            cfg,
		StatsReporters:    statsReporters,
		InChan:            inChan,
		pendingMessagesWG: pendingMessagesWG,
		stopChannel:       make(chan struct{}),
	}

	b.configure()

	return b, nil
}

func (b *Broker) loadConfigurationDefaults() {
	b.Config.SetDefault("feedbackListeners.broker.invalidTokenChan.size", 1000)
	b.Config.SetDefault("feedbackListeners.broker.invalidTokenEnabled", true)
}

func (b *Broker) configure() {
	b.loadConfigurationDefaults()

	b.InvalidTokenEnabled = b.Config.GetBool("feedbackListeners.broker.invalidTokenEnabled")
	b.InvalidTokenOutChan = make(chan *InvalidToken, b.Config.GetInt("feedbackListeners.broker.invalidTokenChan.size"))
}

// Start starts a routine to process the Broker in channel.
func (b *Broker) Start() {
	l := b.Logger.WithField(
		"method", "start",
	)
	l.Info("starting broker")

	b.run = true
	go b.processMessages()
}

// Stop stops all routines from processing the in channel and closes all output channels.
func (b *Broker) Stop() {
	close(b.stopChannel)
	close(b.InvalidTokenOutChan)
}

func (b *Broker) processMessages() {
	l := b.Logger.WithField(
		"method", "processMessages",
	)

	for {
		select {
		case msg, ok := <-b.InChan:
			if ok {
				switch msg.GetPlatform() {
				case APNSPlatform:
					var res structs.ResponseWithMetadata
					err := json.Unmarshal(msg.GetValue(), &res)
					if err != nil {
						l.WithError(err).Error(ErrAPNSUnmarshal.Error())
					}
					b.routeAPNSMessage(&res, msg.GetGame())

				case GCMPlatform:
					var res gcm.CCSMessage
					err := json.Unmarshal(msg.GetValue(), &res)
					if err != nil {
						l.WithError(err).Error(ErrGCMUnmarshal.Error())
					}
					b.routeGCMMessage(&res, msg.GetGame())
				}

				b.confirmMessage()
			}

		case <-b.stopChannel:
			l.Info("stop processing Broker's in channel")
			return
		}
	}

}

func (b *Broker) routeAPNSMessage(msg *structs.ResponseWithMetadata, game string) {
	switch msg.Reason {
	case apns2.ReasonBadDeviceToken, apns2.ReasonUnregistered, apns2.ReasonTopicDisallowed, apns2.ReasonDeviceTokenNotForTopic:
		if b.InvalidTokenEnabled {
			tk := &InvalidToken{
				Token:    msg.DeviceToken,
				Game:     game,
				Platform: APNSPlatform,
			}

			b.InvalidTokenOutChan <- tk
		}
	}
}

func (b *Broker) routeGCMMessage(msg *gcm.CCSMessage, game string) {
	switch msg.Error {
	case "DEVICE_UNREGISTERED", "BAD_REGISTRATION":
		if b.InvalidTokenEnabled {
			tk := &InvalidToken{
				Token:    msg.From,
				Game:     game,
				Platform: GCMPlatform,
			}

			b.InvalidTokenOutChan <- tk
		}
	}
}

func (b *Broker) confirmMessage() {
	if b.pendingMessagesWG != nil {
		b.pendingMessagesWG.Done()
	}
}
