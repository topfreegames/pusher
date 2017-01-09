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
	"sync"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/extensions/extifaces"
	"github.com/topfreegames/pusher/util"
)

// APNSPusher struct for apns pusher
type APNSPusher struct {
	AppName           string
	CertificatePath   string
	Config            *viper.Viper
	ConfigFile        string
	IsProduction      bool
	Logger            *logrus.Logger
	MessageHandler    extifaces.MessageHandler
	PendingMessagesWG *sync.WaitGroup
	Queue             extifaces.Queue
	run               bool
}

// NewAPNSPusher for getting a new APNSPusher instance
func NewAPNSPusher(configFile, certificatePath, appName string, isProduction bool, logger *logrus.Logger) *APNSPusher {
	var wg sync.WaitGroup
	a := &APNSPusher{
		AppName:           appName,
		CertificatePath:   certificatePath,
		ConfigFile:        configFile,
		IsProduction:      isProduction,
		Logger:            logger,
		PendingMessagesWG: &wg,
	}
	a.configure()
	return a
}

func (a *APNSPusher) loadConfigurationDefaults() {}

func (a *APNSPusher) configure() {
	a.Config = util.NewViperWithConfigFile(a.ConfigFile)
	a.loadConfigurationDefaults()
	a.Queue = extensions.NewKafka(a.ConfigFile, a.Logger)
	a.MessageHandler = extensions.NewAPNSMessageHandler(a.ConfigFile, a.CertificatePath, a.AppName, a.IsProduction, a.Logger, a.Queue.PendingMessagesWaitGroup())
}

// Start starts pusher in apns mode
func (a *APNSPusher) Start() {
	a.run = true
	l := a.Logger.WithFields(logrus.Fields{
		"method":          "start",
		"configFile":      a.ConfigFile,
		"certificatePath": a.CertificatePath,
	})
	l.Info("starting pusher in apns mode...")
	go a.MessageHandler.HandleMessages(a.Queue.MessagesChannel())
	go a.MessageHandler.HandleResponses()
	go a.Queue.ConsumeLoop()
	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	for a.run == true {
		select {
		case sig := <-sigchan:
			l.Warnf("caught signal %v: terminating\n", sig)
			a.run = false
		}
	}
	//TODO stop queue and message handler before exiting
	//TODO maybe put a timeout here
	a.Queue.StopConsuming()
	l.Info("pusher is waiting for all inflight messages to receive feedback before exiting...")
	if a.Queue.PendingMessagesWaitGroup() != nil {
		a.Queue.PendingMessagesWaitGroup().Wait()
	}
}
