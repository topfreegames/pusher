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
	"github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/topfreegames/pusher/pusher"
)

var senderID string
var apiKey string

// gcmCmd represents the gcm command
var gcmCmd = &cobra.Command{
	Use:   "gcm",
	Short: "starts pusher in gcm mode",
	Long:  `starts pusher in gcm mode`,
	Run: func(cmd *cobra.Command, args []string) {
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
			"method": "gcmCmd",
			"debug":  debug,
		})
		if len(senderID) == 0 {
			l.Fatal("senderId must be set")
		}
		if len(apiKey) == 0 {
			l.Fatal("apiKey must be set")
		}
		if len(app) == 0 {
			l.Fatal("app must be set")
		}
		gcmPusher, err := pusher.NewGCMPusher(cfgFile, senderID, apiKey, app, production, log)
		if err != nil {
			l.WithError(err).Fatal("Failed to start GCM Pusher.")
		}
		gcmPusher.Start()
	},
}

func init() {
	gcmCmd.Flags().StringVar(&senderID, "senderId", "", "gcm senderID")
	gcmCmd.Flags().StringVar(&apiKey, "apiKey", "", "gcm apiKey")
	gcmCmd.Flags().StringVar(&app, "app", "", "the app name for the table in pushdb")
	RootCmd.AddCommand(gcmCmd)
}
