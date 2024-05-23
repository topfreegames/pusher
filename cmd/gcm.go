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
	"context"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/config"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/pusher"
)

var senderID string
var apiKey string

func startGcm(
	ctx context.Context,
	debug, json, production bool,
	vConfig *viper.Viper,
	config *config.Config,
	statsdClientOrNil interfaces.StatsDClient,
) (*pusher.GCMPusher, error) {
	var log = logrus.New()
	if json {
		log.Formatter = new(logrus.JSONFormatter)
	}
	if debug {
		log.Level = logrus.DebugLevel
	} else {
		log.Level = logrus.InfoLevel
	}
	return pusher.NewGCMPusher(ctx, production, vConfig, config, log, statsdClientOrNil)
}

// gcmCmd represents the gcm command
var gcmCmd = &cobra.Command{
	Use:   "gcm",
	Short: "starts pusher in gcm mode",
	Long:  `starts pusher in gcm mode`,
	Run: func(cmd *cobra.Command, args []string) {
		config, vConfig, err := config.NewConfigAndViper(cfgFile)
		if err != nil {
			panic(err)
		}

		ctx := context.Background()

		gcmPusher, err := startGcm(ctx, debug, json, production, vConfig, config, nil)
		if err != nil {
			panic(err)
		}
		gcmPusher.Start(ctx)
	},
}

func init() {
	RootCmd.AddCommand(gcmCmd)
}
