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
	"errors"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

// GCMPusher struct for GCM pusher
type GCMPusher struct {
	Pusher
}

// NewGCMPusher for getting a new GCMPusher instance
func NewGCMPusher(
	isProduction bool,
	config *viper.Viper,
	logger *logrus.Logger,
	statsdClientOrNil interfaces.StatsDClient,
	db interfaces.DB,
	clientOrNil ...interfaces.GCMClient,
) (*GCMPusher, error) {
	g := &GCMPusher{
		Pusher: Pusher{
			Config:       config,
			IsProduction: isProduction,
			Logger:       logger,
			stopChannel:  make(chan struct{}),
		},
	}
	var client interfaces.GCMClient
	if len(clientOrNil) > 0 {
		client = clientOrNil[0]
	}
	err := g.configure(client, db, statsdClientOrNil)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (g *GCMPusher) configure(client interfaces.GCMClient, db interfaces.DB, statsdClientOrNil interfaces.StatsDClient) error {
	var err error
	l := g.Logger.WithFields(logrus.Fields{
		"method": "configure",
	})
	g.loadConfigurationDefaults()
	g.GracefulShutdownTimeout = g.Config.GetInt("gracefulShutdownTimeout")
	if err = g.configureStatsReporters(statsdClientOrNil); err != nil {
		return err
	}
	if err = g.configureFeedbackReporters(); err != nil {
		return err
	}
	q, err := extensions.NewKafkaConsumer(
		g.Config,
		g.Logger,
		&g.stopChannel,
	)
	if err != nil {
		return err
	}
	g.Queue = q
	g.MessageHandler = make(map[string]interfaces.MessageHandler)
	for _, k := range strings.Split(g.Config.GetString("gcm.apps"), ",") {
		senderID := g.Config.GetString("gcm.certs." + k + ".senderID")
		apiKey := g.Config.GetString("gcm.certs." + k + ".apiKey")
		l.Infof(
			"Configuring messageHandler for game %s with senderID %s and apiKey %s",
			k, senderID, apiKey,
		)
		handler, err := extensions.NewGCMMessageHandler(
			senderID,
			apiKey,
			g.IsProduction,
			g.Config,
			g.Logger,
			g.Queue.PendingMessagesWaitGroup(),
			g.StatsReporters,
			g.feedbackReporters,
			client,
		)
		if err == nil {
			g.MessageHandler[k] = handler
		} else {
			for _, statsReporter := range g.StatsReporters {
				statsReporter.InitializeFailure(k, "gcm")
			}
			l.WithFields(logrus.Fields{
				"method": "gcm",
				"game":   k,
			}).Error(err)
		}
	}
	if len(g.MessageHandler) == 0 {
		return errors.New("Could not initilize any app")
	}
	return nil
}
