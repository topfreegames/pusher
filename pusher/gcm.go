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
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
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
	MessageHandler          interfaces.MessageHandler
	Queue                   interfaces.Queue
	run                     bool
	senderID                string
	StatsReporters          []interfaces.StatsReporter
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
	q, err := extensions.NewKafkaConsumer(g.Config, g.Logger)
	if err != nil {
		return err
	}
	g.Queue = q
	handler, err := extensions.NewGCMMessageHandler(
		g.senderID,
		g.apiKey,
		g.IsProduction,
		g.Config,
		g.Logger,
		g.Queue.PendingMessagesWaitGroup(),
		g.StatsReporters,
		g.feedbackReporters,
		g.InvalidTokenHandlers,
		client,
	)
	if err != nil {
		return err
	}
	g.MessageHandler = handler
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

// Start starts pusher in apns mode
func (g *GCMPusher) Start() {
	g.run = true
	l := g.Logger.WithFields(logrus.Fields{
		"method":   "start",
		"senderID": g.senderID,
	})
	l.Info("starting pusher in gcm mode...")
	go g.MessageHandler.HandleMessages(g.Queue.MessagesChannel())
	go g.MessageHandler.HandleResponses()
	go g.Queue.ConsumeLoop()
	go g.reportGoStats()
	go g.MessageHandler.LogStats()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	for g.run == true {
		select {
		case sig := <-sigchan:
			l.Warnf("caught signal %v: terminating\n", sig)
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
