/*
 * Copyright (c) 2017 TFG Co <backend@tfgco.com>
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
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/topfreegames/pusher/feedback"
	"go.uber.org/zap"
)

// starFeedbackListenerCmd represents the start-feedback-listener command
var startFeedbackListenerCmd = &cobra.Command{
	Use:   "start-feedback-listener",
	Short: "starts the feedback listener",
	Long: `starts the feedback listener that will read from kafka topics and
					 update job feedback column in pg, brokers and topics must be configured
					 in the config file`,
	Run: func(cmd *cobra.Command, args []string) {
		var l = log.New()
		l.Level = log.InfoLevel
		if debug {
			l.Level = log.DebugLevel
		}

		l.Info("configuring feedback listener...")
		f, err := feedback.NewListener(cfgFile, l)
		if err != nil {
			l.Panic("error starting feedback listener", zap.Error(err))
		}
		l.Info("starting feedback listener...")
		f.Start()
	},
}

func init() {
	RootCmd.AddCommand(startFeedbackListenerCmd)
}
