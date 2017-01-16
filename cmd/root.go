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
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string
var debug bool
var production bool
var json bool

// RootCmd for cli
var RootCmd = &cobra.Command{
	Use:   "pusher",
	Short: "A push service that reads from a queue and send a gcm or apns push",
	Long:  `A push service that reads from a queue and send a gcm or apns push`,
}

// Execute rootCmd command
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func init() {
	cobra.OnInitialize()

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "./config/default.yaml", "config file")
	RootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "debug mode switch")
	RootCmd.PersistentFlags().BoolVarP(&production, "production", "p", false, "production mode switch")
	RootCmd.PersistentFlags().BoolVarP(&json, "json", "j", false, "json output mode")
	RootCmd.PersistentFlags().StringVar(&app, "app", "", "the app name for the table in pushdb")
}
