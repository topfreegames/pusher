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
	raven "github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/pusher"
	"github.com/topfreegames/pusher/util"
)

var app string

func startApns(
	debug, json, production bool,
	config *viper.Viper,
	statsdClientOrNil interfaces.StatsDClient,
	dbOrNil interfaces.DB,
	queueOrNil interfaces.APNSPushQueue,
) (*pusher.APNSPusher, error) {
	var log = logrus.New()
	if json {
		log.Formatter = new(logrus.JSONFormatter)
	}
	if debug {
		log.Level = logrus.DebugLevel
	} else {
		log.Level = logrus.InfoLevel
	}
	return pusher.NewAPNSPusher(production, config, log, statsdClientOrNil, dbOrNil, queueOrNil)
}

// apnsCmd represents the apns command
var apnsCmd = &cobra.Command{
	Use:   "apns",
	Short: "starts pusher in apns mode",
	Long:  `starts pusher in apns mode`,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := util.NewViperWithConfigFile(cfgFile)
		if err != nil {
			panic(err)
		}

		sentryURL := config.GetString("sentry.url")
		if sentryURL != "" {
			raven.SetDSN(sentryURL)
		}

		apnsPusher, err := startApns(debug, json, production, config, nil, nil, nil)
		if err != nil {
			raven.CaptureErrorAndWait(err, map[string]string{
				"version": util.Version,
				"cmd":     "apns",
			})
			panic(err)
		}
		apnsPusher.Start()
	},
}

func init() {
	RootCmd.AddCommand(apnsCmd)
}
