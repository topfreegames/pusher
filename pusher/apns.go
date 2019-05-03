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

package pusher

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

// APNSPusher struct for apns pusher
type APNSPusher struct {
	Pusher
}

// NewAPNSPusher for getting a new APNSPusher instance
func NewAPNSPusher(
	isProduction bool,
	config *viper.Viper,
	logger *logrus.Logger,
	statsdClientOrNil interfaces.StatsDClient,
	db interfaces.DB,
	queueOrNil ...interfaces.APNSPushQueue,
) (*APNSPusher, error) {
	a := &APNSPusher{
		Pusher: Pusher{
			Config:       config,
			IsProduction: isProduction,
			Logger:       logger,
			stopChannel:  make(chan struct{}),
		},
	}
	var queue interfaces.APNSPushQueue
	if len(queueOrNil) > 0 {
		queue = queueOrNil[0]
	}
	err := a.configure(queue, db, statsdClientOrNil)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (a *APNSPusher) configure(queue interfaces.APNSPushQueue, db interfaces.DB, statsdClientOrNil interfaces.StatsDClient) error {
	var err error
	l := a.Logger.WithFields(logrus.Fields{
		"method": "configure",
	})
	a.loadConfigurationDefaults()
	a.GracefulShutdownTimeout = a.Config.GetInt("gracefulShutdownTimeout")
	if err = a.configureStatsReporters(statsdClientOrNil); err != nil {
		return err
	}
	if err = a.configureFeedbackReporters(); err != nil {
		return err
	}

	q, err := extensions.NewKafkaConsumer(
		a.Config,
		a.Logger,
		&a.stopChannel,
	)
	if err != nil {
		return err
	}
	a.MessageHandler = make(map[string]interfaces.MessageHandler)
	a.Queue = q
	l.Info("Configuring messageHandler")
	for _, k := range strings.Split(a.Config.GetString("apns.apps"), ",") {
		authKeyPath := a.Config.GetString("apns.certs." + k + ".authKeyPath")
		keyID := a.Config.GetString("apns.certs." + k + ".keyID")
		teamID := a.Config.GetString("apns.certs." + k + ".teamID")
		topic := a.Config.GetString("apns.certs." + k + ".topic")
		l.Infof(
			"Configuring messageHandler for game %s with key: %s",
			k, authKeyPath,
		)
		handler, err := extensions.NewAPNSMessageHandler(
			authKeyPath,
			keyID,
			teamID,
			topic,
			k,
			a.IsProduction,
			a.Config,
			a.Logger,
			a.Queue.PendingMessagesWaitGroup(),
			a.StatsReporters,
			a.feedbackReporters,
			nil,
		)
		if err != nil {
			return err
		}
		a.MessageHandler[k] = handler
	}
	return nil
}
