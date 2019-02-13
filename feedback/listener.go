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
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	raven "github.com/getsentry/raven-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// Listener will consume push feedbacks from a queue and update job feedbacks column
type Listener struct {
	Config     *viper.Viper
	ConfigFile string
	Logger     *log.Logger
	Queue      Queue
	// FeedbackHandler         *Handler
	Broker                  *Broker
	InvalidTokenHandler     Handler
	GracefulShutdownTimeout int
	run                     bool
	stopChannel             chan struct{}
}

// NewListener creates and return a new instance of feedback.Listener
func NewListener(configFile string, logger *log.Logger) (*Listener, error) {
	l := &Listener{
		ConfigFile:  configFile,
		Logger:      logger,
		stopChannel: make(chan struct{}),
	}
	err := l.configure()
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Listener) loadConfigurationDefaults() {
	l.Config.SetDefault("gracefulShutdownTimeout", 10)
}

func (l *Listener) configure() error {
	l.Config = viper.New()
	l.Config.SetConfigFile(l.ConfigFile)
	l.Config.SetEnvPrefix("pusher")
	l.Config.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	l.Config.AutomaticEnv()
	err := l.Config.ReadInConfig()
	if err != nil {
		return err
	}
	l.loadConfigurationDefaults()
	l.configureSentry()
	l.GracefulShutdownTimeout = l.Config.GetInt("gracefulShutdownTimeout")
	q, err := NewKafkaConsumer(
		l.Config, l.Logger,
		&l.stopChannel, nil,
	)
	if err != nil {
		return err
	}
	l.Queue = q
	// h, err := NewHandler(l.Config, l.Logger, l.Queue.PendingMessagesWaitGroup())
	// if err != nil {
	// 	return err
	// }
	// l.FeedbackHandler = h
	l.InvalidTokenHandler = NewInvalidTokenHandler()
	l.Broker = NewBroker(l.Logger, l.Config, q.MessagesChannel())
	return nil
}

func (l *Listener) configureSentry() {
	ll := l.Logger.WithFields(log.Fields{
		"source":    "listener",
		"operation": "configureSentry",
	})

	sentryURL := l.Config.GetString("sentry.url")
	if sentryURL != "" {
		raven.SetDSN(sentryURL)
	}

	ll.Info("Configured sentry successfully.")
}

// Start starts the listener
func (l *Listener) Start() {
	l.run = true
	log := l.Logger.WithField(
		"method", "start",
	)
	log.Info("starting the feedbacks listener...")

	go l.Queue.ConsumeLoop()
	// go l.FeedbackHandler.HandleMessages(l.Queue.MessagesChannel())
	go l.Broker.Start()
	// go func(msgChan *chan []byte) {

	// 	for l.run == true {
	// 		select {
	// 		case message := <-*msgChan:
	// 			l.handleMessage(message)
	// 		}
	// 	}
	// }(l.Queue.MessagesChannel())

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	for l.run == true {
		select {
		case sig := <-sigchan:
			log.WithField("signal", sig.String()).Warn("terminading due to caught signal")
			l.run = false
		case <-l.stopChannel:
			log.Warn("Stop channel closed\n")
			l.run = false
		}
	}

	l.Queue.StopConsuming()
	l.gracefulShutdown(l.Queue.PendingMessagesWaitGroup(), time.Duration(l.GracefulShutdownTimeout)*time.Second)
}

// func (l *Listener) handleMessage(msg []byte) {
// 	// log := l.Logger.WithField(
// 	// 	"method", "feedback.handler.handleMessage",
// 	// )

// 	// var message Message
// 	// err := json.Unmarshal(msg, &message)
// 	fmt.Println("Printed Message: ", msg)
// }

// GracefulShutdown waits for wg do complete then exits
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
