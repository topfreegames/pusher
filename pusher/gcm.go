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

// GCMPusher struct for apns pusher
type GCMPusher struct {
	apiKey         string
	AppName        string
	Config         *viper.Viper
	ConfigFile     string
	MessageHandler extifaces.MessageHandler
	Queue          extifaces.Queue
	run            bool
	senderID       string
}

// NewGCMPusher for getting a new GCMPusher instance
func NewGCMPusher(configFile, senderID, apiKey, appName string) *GCMPusher {
	g := &GCMPusher{
		apiKey:     apiKey,
		AppName:    appName,
		ConfigFile: configFile,
		senderID:   senderID,
	}
	g.configure()
	return g
}

func (g *GCMPusher) loadConfigurationDefaults() {

}

func (g *GCMPusher) configure() {
	g.Config = util.NewViperWithConfigFile(g.ConfigFile)
	g.loadConfigurationDefaults()
	g.Queue = extensions.NewKafka(g.ConfigFile)
	g.MessageHandler = extensions.NewGCMMessageHandler(g.ConfigFile, g.senderID, g.apiKey, g.AppName)
}

// Start starts pusher in apns mode
func (g GCMPusher) Start() {
	g.run = true
	l := log.WithFields(log.Fields{
		"configFile": g.ConfigFile,
		"senderID":   g.senderID,
	})
	l.Info("starting pusher in gcm mode...")
	go g.MessageHandler.HandleMessages(g.Queue.MessagesChannel())
	go g.MessageHandler.HandleResponses()
	go g.Queue.ConsumeLoop()

	sigchan := make(chan os.Signal)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	for g.run == true {
		select {
		case sig := <-sigchan:
			log.Warnf("caught signal %v: terminating\n", sig)
			g.run = false
		}
	}
	//TODO stop queue and message handler before exiting
	l.Info("exiting pusher...")
}
