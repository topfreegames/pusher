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

// APNSPusher struct for apns pusher
type APNSPusher struct {
	CertificatePath         string
	Config                  *viper.Viper
	feedbackReporters       []interfaces.FeedbackReporter
	GracefulShutdownTimeout int
	InvalidTokenHandlers    []interfaces.InvalidTokenHandler
	IsProduction            bool
	Logger                  *logrus.Logger
	MessageHandler          map[string]interfaces.MessageHandler
	Queue                   interfaces.Queue
	run                     bool
	StatsReporters          []interfaces.StatsReporter
	stopChannel             chan struct{}
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
		Config:       config,
		IsProduction: isProduction,
		Logger:       logger,
		stopChannel:  make(chan struct{}),
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

func (a *APNSPusher) loadConfigurationDefaults() {
	a.Config.SetDefault("gracefulShutdownTimeout", 10)
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
	if err = a.configureInvalidTokenHandlers(db); err != nil {
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
			a.IsProduction,
			a.Config,
			a.Logger,
			a.Queue.PendingMessagesWaitGroup(),
			a.StatsReporters,
			a.feedbackReporters,
			a.InvalidTokenHandlers,
			nil,
		)
		if err != nil {
			return err
		}
		a.MessageHandler[k] = handler
	}
	return nil
}

func (a *APNSPusher) configureFeedbackReporters() error {
	reporters, err := configureFeedbackReporters(a.Config, a.Logger)
	if err != nil {
		return err
	}
	a.feedbackReporters = reporters
	return nil
}

func (a *APNSPusher) configureStatsReporters(clientOrNil interfaces.StatsDClient) error {
	reporters, err := configureStatsReporters(a.Config, a.Logger, clientOrNil)
	if err != nil {
		return err
	}
	a.StatsReporters = reporters
	return nil
}

func (a *APNSPusher) configureInvalidTokenHandlers(dbOrNil interfaces.DB) error {
	invalidTokenHandlers, err := configureInvalidTokenHandlers(a.Config, a.Logger, dbOrNil)
	if err != nil {
		return err
	}
	a.InvalidTokenHandlers = invalidTokenHandlers
	return nil
}

func (a *APNSPusher) routeMessages(msgChan *chan interfaces.KafkaMessage) {
	for a.run == true {
		select {
		case message := <-*msgChan:
			if handler, ok := a.MessageHandler[message.Game]; ok {
				handler.HandleMessages(message)
			} else {
				a.Logger.WithFields(logrus.Fields{
					"method": "routeMessages",
					"game":   message.Game,
				}).Error("Game not found")
			}
		}
	}
}

// Start starts pusher in apns mode
func (a *APNSPusher) Start() {
	a.run = true
	l := a.Logger.WithFields(logrus.Fields{
		"method":          "start",
		"certificatePath": a.CertificatePath,
	})
	l.Info("starting pusher in apns mode...")
	go a.routeMessages(a.Queue.MessagesChannel())
	for _, v := range a.MessageHandler {
		go v.HandleResponses()
		go v.LogStats()
		msgHandler, _ := v.(*extensions.APNSMessageHandler)
		go msgHandler.CleanMetadataCache()
	}
	go a.Queue.ConsumeLoop()
	go a.reportGoStats()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	for a.run == true {
		select {
		case sig := <-sigchan:
			l.Warnf("caught signal %v: terminating\n", sig)
			a.run = false
		case <-a.stopChannel:
			l.Warn("Stop channel closed\n")
			a.run = false
		}
	}
	a.Queue.StopConsuming()
	GracefulShutdown(a.Queue.PendingMessagesWaitGroup(), time.Duration(a.GracefulShutdownTimeout)*time.Second)
}

func (a *APNSPusher) reportGoStats() {
	for {
		num := runtime.NumGoroutine()
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		gcTime := m.PauseNs[(m.NumGC+255)%256]
		for _, statsReporter := range a.StatsReporters {
			statsReporter.ReportGoStats(
				num,
				m.Alloc, m.HeapObjects, m.NextGC,
				gcTime,
			)
		}
		time.Sleep(30 * time.Second)
	}
}
