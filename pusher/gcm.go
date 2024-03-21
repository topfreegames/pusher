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
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/config"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/extensions/client"
	"github.com/topfreegames/pusher/extensions/handler"
	"github.com/topfreegames/pusher/interfaces"
)

// GCMPusher struct for GCM pusher
type GCMPusher struct {
	Pusher
}

// NewGCMPusher for getting a new GCMPusher instance
func NewGCMPusher(
	isProduction bool,
	viperConfig *viper.Viper,
	config *config.Config,
	logger *logrus.Logger,
	statsdClientOrNil interfaces.StatsDClient,
) (*GCMPusher, error) {
	g := &GCMPusher{
		Pusher: Pusher{
			ViperConfig:  viperConfig,
			Config:       config,
			IsProduction: isProduction,
			Logger:       logger,
			stopChannel:  make(chan struct{}),
		},
	}
	l := g.Logger.WithFields(logrus.Fields{
		"method": "NewGCMPusher",
	})

	g.loadConfigurationDefaults()
	g.loadConfiguration()

	if err := g.configureStatsReporters(statsdClientOrNil); err != nil {
		l.WithError(err).Error("could not configure stats reporters")
		return nil, fmt.Errorf("could not configure stats reporters: %w", err)
	}

	if err := g.configureFeedbackReporters(); err != nil {
		l.WithError(err).Error("could not configure feedback reporters")
		return nil, fmt.Errorf("could not configure feedback reporters: %w", err)
	}

	q, err := extensions.NewKafkaConsumer(g.ViperConfig, g.Logger, &g.stopChannel)
	if err != nil {
		l.WithError(err).Error("could not create kafka consumer")
		return nil, fmt.Errorf("could not create kafka consumer: %w", err)
	}
	g.Queue = q

	err = g.createMessageHandlerForApps()
	if err != nil {
		l.WithError(err).Error("could not create message handlers")
		return nil, fmt.Errorf("could not create message handlers: %w", err)
	}
	return g, nil
}

func (g *GCMPusher) createMessageHandlerForApps() error {
	l := g.Logger.WithFields(logrus.Fields{
		"method": "GCMPusher.createMessageHandlerForApps",
	})

	g.MessageHandler = make(map[string]interfaces.MessageHandler)
	for _, app := range g.Config.GetAppsArray() {
		credentials, ok := g.Config.GCM.FirebaseCredentials[app]

		if ok { // Firebase is configured, use new handler
			pushClient, err := client.NewFirebaseClient(credentials, g.Logger)
			if err != nil {
				l.WithError(err).WithFields(logrus.Fields{
					"app": app,
				}).Error("could not create firebase client")
				return fmt.Errorf("could not create firebase pushClient for all apps: %w", err)
			}
			g.MessageHandler[app] = handler.NewMessageHandler(
				app,
				pushClient,
				g.feedbackReporters,
				g.StatsReporters,
				g.Logger,
			)
		} else { // Firebase credentials not yet configured, use legacy XMPP client
			handler, err := extensions.NewGCMMessageHandler(
				app,
				g.IsProduction,
				g.ViperConfig,
				g.Logger,
				g.Queue.PendingMessagesWaitGroup(),
				g.StatsReporters,
				g.feedbackReporters,
			)

			if err != nil {
				l.WithError(err).WithFields(logrus.Fields{
					"app": app,
				}).Error("could not create gcm message handler")
				return fmt.Errorf("could not create gcm message handler for all apps: %w", err)
			}

			g.MessageHandler[app] = handler
		}
	}
	return nil
}
