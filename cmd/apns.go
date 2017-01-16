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

package cmd

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/pusher"
)

var app string
var certificate string

func startApns(debug, json, production bool, cfgFile, certificate, app string, dbOrNil interfaces.DB, queueOrNil interfaces.APNSPushQueue) (*pusher.APNSPusher, error) {
	var log = logrus.New()
	if json {
		log.Formatter = new(logrus.JSONFormatter)
	}
	if debug {
		log.Level = logrus.DebugLevel
	} else {
		log.Level = logrus.InfoLevel
	}
	l := log.WithFields(logrus.Fields{
		"method": "apnsCmd",
		"debug":  debug,
	})
	if len(certificate) == 0 {
		err := fmt.Errorf("pem certificate must be set")
		l.Error(err)
		return nil, err
	}
	if len(app) == 0 {
		err := fmt.Errorf("app must be set")
		l.Error(err)
		return nil, err
	}
	return pusher.NewAPNSPusher(cfgFile, certificate, app, production, log, dbOrNil, queueOrNil)
}

// apnsCmd represents the apns command
var apnsCmd = &cobra.Command{
	Use:   "apns",
	Short: "starts pusher in apns mode",
	Long:  `starts pusher in apns mode`,
	Run: func(cmd *cobra.Command, args []string) {
		apnsPusher, err := startApns(debug, json, production, cfgFile, certificate, app, nil, nil)
		if err != nil {
			panic(err)
		}
		apnsPusher.Start()
	},
}

func init() {
	apnsCmd.Flags().StringVar(&certificate, "certificate", "", "pem certificate path")
	RootCmd.AddCommand(apnsCmd)
}
