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
	"github.com/topfreegames/pusher/util"
)

// GCMPusher struct for GCM pusher
type GCMPusher struct {
	apiKey                  string
	AppName                 string
	Config                  *viper.Viper
	ConfigFile              string
	GracefulShutdownTimeout int
	IsProduction            bool
	Logger                  *logrus.Logger
	MessageHandler          interfaces.MessageHandler
	Queue                   interfaces.Queue
	run                     bool
	senderID                string
	StatsReporters          []interfaces.StatsReporter
	feedbackReporters       []interfaces.FeedbackReporter
}

// NewGCMPusher for getting a new GCMPusher instance
func NewGCMPusher(
	configFile,
	senderID,
	apiKey,
	appName string,
	isProduction bool,
	logger *logrus.Logger,
	db interfaces.DB,
	clientOrNil ...interfaces.GCMClient,
) (*GCMPusher, error) {
	g := &GCMPusher{
		apiKey:       apiKey,
		AppName:      appName,
		ConfigFile:   configFile,
		IsProduction: isProduction,
		Logger:       logger,
		senderID:     senderID,
	}
	var client interfaces.GCMClient
	if len(clientOrNil) > 0 {
		client = clientOrNil[0]
	}
	err := g.configure(client, db)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (g *GCMPusher) loadConfigurationDefaults() {
	g.Config.SetDefault("gracefullShutdownTimeout", 10)
}

func (g *GCMPusher) configure(client interfaces.GCMClient, db interfaces.DB) error {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	g.GracefulShutdownTimeout = g.Config.GetInt("gracefullShutdownTimeout")
	if err := g.configureStatsReporters(); err != nil {
		return err
	}
	if err := g.configureFeedbackReporters(); err != nil {
		return err
	}
	q, err := extensions.NewKafkaConsumer(g.Config, g.Logger)
	if err != nil {
		return err
	}
	g.Queue = q
	handler, err := extensions.NewGCMMessageHandler(
		g.ConfigFile,
		g.senderID,
		g.apiKey,
		g.AppName,
		g.IsProduction,
		g.Logger,
		g.Queue.PendingMessagesWaitGroup(),
		g.StatsReporters,
		g.feedbackReporters,
		client,
		db,
	)
	if err != nil {
		return err
	}
	g.MessageHandler = handler
	return nil
}

func (g *GCMPusher) configureStatsReporters() error {
	reporters, err := configureStatsReporters(g.ConfigFile, g.Logger, g.AppName, g.Config)
	if err != nil {
		return err
	}
	g.StatsReporters = reporters
	return nil
}

func (g *GCMPusher) configureFeedbackReporters() error {
	reporters, err := configureFeedbackReporters(g.ConfigFile, g.Logger, g.Config)
	if err != nil {
		return err
	}
	g.feedbackReporters = reporters
	return nil
}

// Start starts pusher in apns mode
func (g *GCMPusher) Start() {
	g.run = true
	l := g.Logger.WithFields(logrus.Fields{
		"method":     "start",
		"configFile": g.ConfigFile,
		"senderID":   g.senderID,
	})
	l.Info("starting pusher in gcm mode...")
	go g.MessageHandler.HandleMessages(g.Queue.MessagesChannel())
	go g.MessageHandler.HandleResponses()
	go g.Queue.ConsumeLoop()
	go g.reportGoStats()

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
