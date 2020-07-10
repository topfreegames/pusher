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

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
)

// Pusher struct for pusher
type Pusher struct {
	feedbackReporters       []interfaces.FeedbackReporter
	StatsReporters          []interfaces.StatsReporter
	Queue                   interfaces.Queue
	Config                  *viper.Viper
	GracefulShutdownTimeout int
	Logger                  *logrus.Logger
	MessageHandler          map[string]interfaces.MessageHandler
	stopChannel             chan struct{}
	IsProduction            bool
	run                     bool
}

func (p *Pusher) loadConfigurationDefaults() {
	p.Config.SetDefault("gracefulShutdownTimeout", 10)
	p.Config.SetDefault("stats.reporters", []string{})
}

func (p *Pusher) configureFeedbackReporters() error {
	reporters, err := configureFeedbackReporters(p.Config, p.Logger)
	if err != nil {
		return err
	}
	p.feedbackReporters = reporters
	return nil
}

func (p *Pusher) configureStatsReporters(clientOrNil interfaces.StatsDClient) error {
	reporters, err := configureStatsReporters(p.Config, p.Logger, clientOrNil)
	if err != nil {
		return err
	}
	p.StatsReporters = reporters
	return nil
}

func (p *Pusher) routeMessages(msgChan *chan interfaces.KafkaMessage) {
	for p.run {
		select {
		case message := <-*msgChan:
			if handler, ok := p.MessageHandler[message.Game]; ok {
				handler.HandleMessages(message)
			} else {
				p.Logger.WithFields(logrus.Fields{
					"method": "routeMessages",
					"game":   message.Game,
				}).Error("Game not found")
			}
		}
	}
}

// Start starts pusher
func (p *Pusher) Start() {
	p.run = true
	l := p.Logger.WithFields(logrus.Fields{
		"method": "start",
	})
	l.Info("starting pusher...")
	go p.routeMessages(p.Queue.MessagesChannel())
	for _, v := range p.MessageHandler {
		go v.HandleResponses()
		go v.LogStats()
		go v.CleanMetadataCache()
	}
	go p.Queue.ConsumeLoop()
	go p.reportGoStats()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	for p.run {
		select {
		case sig := <-sigchan:
			l.Warnf("caught signal %v: terminating\n", sig)
			p.run = false
		case <-p.stopChannel:
			l.Warn("Stop channel closed\n")
			p.run = false
		}
	}
	p.Queue.StopConsuming()
	GracefulShutdown(p.Queue.PendingMessagesWaitGroup(), time.Duration(p.GracefulShutdownTimeout)*time.Second)
}

func (p *Pusher) reportGoStats() {
	for {
		num := runtime.NumGoroutine()
		m := &runtime.MemStats{}
		runtime.ReadMemStats(m)
		gcTime := m.PauseNs[(m.NumGC+255)%256]
		for _, statsReporter := range p.StatsReporters {
			statsReporter.ReportGoStats(
				num,
				m.Alloc, m.HeapObjects, m.NextGC,
				gcTime,
			)
		}
		time.Sleep(30 * time.Second)
	}
}
