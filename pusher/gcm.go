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
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

// GCMPusher struct for GCM pusher
type GCMPusher struct {
	apiKey                  string
	Config                  *viper.Viper
	feedbackReporters       []interfaces.FeedbackReporter
	GracefulShutdownTimeout int
	InvalidTokenHandlers    []interfaces.InvalidTokenHandler
	IsProduction            bool
	Logger                  *logrus.Logger
	MessageHandler          map[string]interfaces.MessageHandler
	Queue                   interfaces.Queue
	run                     bool
	senderID                string
	StatsReporters          []interfaces.StatsReporter
	stopChannel             chan struct{}
}

// NewGCMPusher for getting a new GCMPusher instance
func NewGCMPusher(
	senderID,
	apiKey string,
	isProduction bool,
	config *viper.Viper,
	logger *logrus.Logger,
	statsdClientOrNil interfaces.StatsDClient,
	db interfaces.DB,
	clientOrNil ...interfaces.GCMClient,
) (*GCMPusher, error) {
	g := &GCMPusher{
		apiKey:       apiKey,
		Config:       config,
		IsProduction: isProduction,
		Logger:       logger,
		senderID:     senderID,
		stopChannel:  make(chan struct{}),
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

func (g *GCMPusher) loadConfigurationDefaults() {
	g.Config.SetDefault("gracefulShutdownTimeout", 10)
	g.Config.SetDefault("stats.reporters", []string{})
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
	if err = g.configureInvalidTokenHandlers(db); err != nil {
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
		handler, herr := extensions.NewGCMMessageHandler(
			senderID,
			apiKey,
			g.IsProduction,
			g.Config,
			g.Logger,
			g.Queue.PendingMessagesWaitGroup(),
			g.StatsReporters,
			g.feedbackReporters,
			g.InvalidTokenHandlers,
			client,
		)
		if herr != nil {
			return herr
		}
		g.MessageHandler[k] = handler
	}
	if err != nil {
		return err
	}
	return nil
}

func (g *GCMPusher) configureStatsReporters(clientOrNil interfaces.StatsDClient) error {
	reporters, err := configureStatsReporters(g.Config, g.Logger, clientOrNil)
	if err != nil {
		return err
	}
	g.StatsReporters = reporters
	return nil
}

func (g *GCMPusher) configureFeedbackReporters() error {
	reporters, err := configureFeedbackReporters(g.Config, g.Logger)
	if err != nil {
		return err
	}
	g.feedbackReporters = reporters
	return nil
}

func (g *GCMPusher) configureInvalidTokenHandlers(dbOrNil interfaces.DB) error {
	invalidTokenHandlers, err := configureInvalidTokenHandlers(g.Config, g.Logger, dbOrNil)
	if err != nil {
		return err
	}
	g.InvalidTokenHandlers = invalidTokenHandlers
	return nil
}

func (g *GCMPusher) routeMessages(msgChan *chan interfaces.KafkaMessage) {
	for g.run == true {
		select {
		case message := <-*msgChan:
			if handler, ok := g.MessageHandler[message.Game]; ok {
				handler.HandleMessages(message)
			} else {
				g.Logger.WithFields(logrus.Fields{
					"method": "routeMessages",
					"game":   message.Game,
				}).Error("Game not found")
			}
		}
	}
}

// Start starts pusher in apns mode
func (g *GCMPusher) Start() {
	g.run = true
	l := g.Logger.WithFields(logrus.Fields{
		"method":   "start",
		"senderID": g.senderID,
	})
	l.Info("starting pusher in gcm mode...")
	for _, v := range g.MessageHandler {
		go v.HandleResponses()
		go v.LogStats()
		msgHandler, _ := v.(*extensions.GCMMessageHandler)
		go msgHandler.CleanMetadataCache()
	}

	go g.Queue.ConsumeLoop()
	go g.reportGoStats()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	for g.run == true {
		select {
		case sig := <-sigchan:
			l.Warnf("caught signal %v: terminating\n", sig)
			g.run = false
		case <-g.stopChannel:
			l.Warn("Stop channel closed\n")
			g.run = false
		}
	}
	g.Queue.StopConsuming()
	GracefulShutdown(g.Queue.PendingMessagesWaitGroup(), time.Duration(g.GracefulShutdownTimeout)*time.Second)
}

func (g *GCMPusher) reportGoStats() {
	for {
		num := runtime.NumGoroutine()
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		gcTime := m.PauseNs[(m.NumGC+255)%256]
		for _, statsReporter := range g.StatsReporters {
			statsReporter.ReportGoStats(
				num,
				m.Alloc, m.HeapObjects, m.NextGC,
				gcTime,
			)
		}

		time.Sleep(30 * time.Second)
	}
}
