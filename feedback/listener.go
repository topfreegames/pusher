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
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
)

// Listener will consume push feedbacks from a queue and use a broker to route
// the messages to a convenient handler
type Listener struct {
	Config         *viper.Viper
	Logger         *log.Logger
	StatsReporters []interfaces.StatsReporter
	statsFlushTime time.Duration

	Queue                   Queue
	Broker                  *Broker
	InvalidTokenHandler     *InvalidTokenHandler
	GracefulShutdownTimeout int

	run         bool
	stopChannel chan struct{}
}

// NewListener creates and return a new Listener instance
func NewListener(
	config *viper.Viper, logger *log.Logger,
	statsdClientOrNil interfaces.StatsDClient,
) (*Listener, error) {
	l := &Listener{
		Config:      config,
		Logger:      logger,
		stopChannel: make(chan struct{}),
	}

	err := l.configure(statsdClientOrNil)
	if err != nil {
		return nil, err
	}

	return l, nil
}

func (l *Listener) loadConfigurationDefaults() {
	l.Config.SetDefault("feedbackListeners.gracefulShutdownTimeout", 1)
	l.Config.SetDefault("stats.flush.s", 5)
}

func (l *Listener) configure(statsdClientrOrNil interfaces.StatsDClient) error {
	l.loadConfigurationDefaults()

	l.GracefulShutdownTimeout = l.Config.GetInt("feedbackListeners.gracefulShutdownTimeout")
	if err := l.configureStatsReporters(statsdClientrOrNil); err != nil {
		return fmt.Errorf("error configuring statsReporters")
	}

	q, err := NewKafkaConsumer(
		l.Config, l.Logger,
		&l.stopChannel, nil,
	)
	if err != nil {
		return fmt.Errorf("error creating new queue: %s", err.Error())
	}
	l.Queue = q

	broker, err := NewBroker(l.Logger, l.Config, l.StatsReporters, q.MessagesChannel(), l.Queue.PendingMessagesWaitGroup())
	if err != nil {
		return fmt.Errorf("error creating new broker: %s", err.Error())
	}
	l.Broker = broker

	handler, err := NewInvalidTokenHandler(l.Logger, l.Config, l.StatsReporters, l.Broker.InvalidTokenOutChan)
	if err != nil {
		return fmt.Errorf("error creating new invalid token handler: %s", err.Error())
	}
	l.InvalidTokenHandler = handler

	return nil
}

func (l *Listener) configureStatsReporters(clientOrNil interfaces.StatsDClient) error {
	reporters, err := configureStatsReporters(l.Config, l.Logger, clientOrNil)
	if err != nil {
		return err
	}
	l.StatsReporters = reporters
	l.statsFlushTime = time.Duration(l.Config.GetInt("stats.flush.s")) * time.Second

	return nil
}

// Start starts the listener
func (l *Listener) Start() {
	l.run = true
	log := l.Logger.WithField(
		"method", "start",
	)
	log.Info("starting the feedback listener...")

	go l.Queue.ConsumeLoop()
	l.Broker.Start()
	l.InvalidTokenHandler.Start()

	statsReporterReportMetricCount(l.StatsReporters,
		"feedback_listener_restart", 1, "", "")

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	flushTicker := time.NewTicker(l.statsFlushTime)
	defer flushTicker.Stop()

	for l.run {
		select {
		case sig := <-sigchan:
			log.WithField("signal", sig.String()).Warn("terminating due to caught signal")
			l.run = false
		case <-l.stopChannel:
			log.Warn("Stop channel closed\n")
			l.run = false
		case <-flushTicker.C:
			l.flushStats()
		}
	}

	l.Cleanup()
}

func (l *Listener) flushStats() {
	statsReporterReportMetricGauge(l.StatsReporters,
		"queue_out_channel", float64(len(l.Queue.MessagesChannel())), "", "")
	statsReporterReportMetricGauge(l.StatsReporters,
		"broker_in_channel", float64(len(l.Broker.InChan)), "", "")
	statsReporterReportMetricGauge(l.StatsReporters,
		"broker_invalid_token_channel", float64(len(l.Broker.InvalidTokenOutChan)), "", "")
	statsReporterReportMetricGauge(l.StatsReporters,
		"invalid_token_handler_buffer", float64(len(l.InvalidTokenHandler.Buffer)), "", "")
}

// Cleanup ends the Listener execution
func (l *Listener) Cleanup() {
	l.flushStats()

	l.Queue.StopConsuming()
	l.Queue.Cleanup()
	l.Broker.Stop()
	l.InvalidTokenHandler.Stop()
	l.gracefulShutdown(l.Queue.PendingMessagesWaitGroup(), time.Duration(l.GracefulShutdownTimeout)*time.Second)
}

// GracefulShutdown waits for wg to complete then exits
func (l *Listener) gracefulShutdown(wg *sync.WaitGroup, timeout time.Duration) {
	log := l.Logger.WithFields(log.Fields{
		"method":  "gracefulShutdown",
		"timeout": int(timeout.Seconds()),
	})

	if wg != nil {
		log.Info("listener is waiting to exit...")
		e := WaitTimeout(wg, timeout)
		if e {
			log.Warn("exited listener because of graceful shutdown timeout")
		} else {
			log.Info("exited listener gracefully")
		}
	}
}

// WaitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
// got from http://stackoverflow.com/a/32843750/3987733
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}
