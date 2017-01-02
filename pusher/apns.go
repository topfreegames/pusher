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
	"syscall"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/extensions/extifaces"
	"github.com/topfreegames/pusher/util"
)

// APNSPusher struct for apns pusher
type APNSPusher struct {
	AppName         string
	ConfigFile      string
	Queue           extifaces.Queue
	Config          *viper.Viper
	MessageHandler  extifaces.MessageHandler
	CertificatePath string
	Environment     string
	run             bool
}

// NewAPNSPusher for getting a new APNSPusher instance
func NewAPNSPusher(configFile, certificatePath, environment, appName string) *APNSPusher {
	a := &APNSPusher{
		ConfigFile:      configFile,
		CertificatePath: certificatePath,
		Environment:     environment,
		AppName:         appName,
	}
	a.configure()
	return a
}

func (a *APNSPusher) loadConfigurationDefaults() {}

func (a *APNSPusher) configure() {
	a.Config = util.NewViperWithConfigFile(a.ConfigFile)
	a.loadConfigurationDefaults()
	a.Queue = extensions.NewKafka(a.ConfigFile)
	a.MessageHandler = extensions.NewAPNSMessageHandler(a.ConfigFile, a.CertificatePath, a.Environment, a.AppName)
}

// Start starts pusher in apns mode
func (a APNSPusher) Start() {
	a.run = true
	l := log.WithFields(log.Fields{
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
			log.Warnf("caught signal %v: terminating\n", sig)
			a.run = false
		}
	}
	//TODO stop queue and message handler before exiting
	l.Info("exiting pusher...")
}
