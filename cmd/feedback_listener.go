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

package cmd

import (
	raven "github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/feedback"
	"github.com/topfreegames/pusher/util"
)

func newFeedbackListener(
	debug, json bool,
	config *viper.Viper,
) (*feedback.Listener, error) {
	log := logrus.New()
	if json {
		log.Formatter = new(logrus.JSONFormatter)
	}
	if debug {
		log.Level = logrus.DebugLevel
	} else {
		log.Level = logrus.InfoLevel
	}

	return feedback.NewListener(config, log)
}

// starFeedbackListenerCmd represents the start-feedback-listener command
var startFeedbackListenerCmd = &cobra.Command{
	Use:   "start-feedback-listener",
	Short: "starts the feedback listener",
	Long: `starts the feedback listener that will read from kafka topics,
		process the messages and route them for a convenient handler`,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := util.NewViperWithConfigFile(cfgFile)
		if err != nil {
			panic(err)
		}

		sentryURL := config.GetString("sentry.url")
		if sentryURL != "" {
			raven.SetDSN(sentryURL)
		}

		listener, err := newFeedbackListener(debug, json, config)
		if err != nil {
			raven.CaptureErrorAndWait(err, map[string]string{
				"version": util.Version,
				"cmd":     "apns",
			})
			panic(err)
		}
		listener.Start()
	},
}

func init() {
	RootCmd.AddCommand(startFeedbackListenerCmd)
}
