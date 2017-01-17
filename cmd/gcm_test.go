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
	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("GCM", func() {
	cfg := "../config/test.yaml"
	apiKey := "api-key"
	senderID := "sender-id"
	logger, _ := test.NewNullLogger()

	var config *viper.Viper
	var mockClient *mocks.GCMClientMock
	var mockDb *mocks.PGMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	BeforeEach(func() {
		var err error
		config, err = util.NewViperWithConfigFile(cfg)
		Expect(err).NotTo(HaveOccurred())
		mockDb = mocks.NewPGMock(0, 1)
		mockClient = mocks.NewGCMClientMock()
		mockStatsDClient = mocks.NewStatsDClientMock()
		c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
		Expect(err).NotTo(HaveOccurred())
		statsClients = []interfaces.StatsReporter{c}
	})

	Describe("[Unit]", func() {
		It("Should return gcmPusher without errors", func() {
			gcmPusher, err := startGcm(false, false, false, cfg, senderID, apiKey, statsClients, mockDb, mockClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gcmPusher).NotTo(BeNil())
			Expect(gcmPusher.ConfigFile).To(Equal(cfg))
			Expect(gcmPusher.IsProduction).To(BeFalse())
			Expect(gcmPusher.Logger.Level).To(Equal(logrus.InfoLevel))
			Expect(fmt.Sprintf("%T", gcmPusher.Logger.Formatter)).To(Equal(fmt.Sprintf("%T", &logrus.TextFormatter{})))
		})

		It("Should set log to json format", func() {
			gcmPusher, err := startGcm(false, true, false, cfg, senderID, apiKey, statsClients, mockDb, mockClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gcmPusher).NotTo(BeNil())
			Expect(fmt.Sprintf("%T", gcmPusher.Logger.Formatter)).To(Equal(fmt.Sprintf("%T", &logrus.JSONFormatter{})))
		})

		It("Should set log to debug", func() {
			gcmPusher, err := startGcm(true, false, false, cfg, senderID, apiKey, statsClients, mockDb, mockClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gcmPusher).NotTo(BeNil())
			Expect(gcmPusher.Logger.Level).To(Equal(logrus.DebugLevel))
		})

		It("Should set log to production", func() {
			gcmPusher, err := startGcm(false, false, true, cfg, senderID, apiKey, statsClients, mockDb, mockClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gcmPusher).NotTo(BeNil())
			Expect(gcmPusher.IsProduction).To(BeTrue())
		})

		It("Should return error if senderId is not provided", func() {
			gcmPusher, err := startGcm(false, false, false, cfg, "", apiKey, statsClients, mockDb, mockClient)
			Expect(gcmPusher).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("senderId must be set"))
		})

		It("Should return error if apiKey is not provided", func() {
			gcmPusher, err := startGcm(false, false, false, cfg, senderID, "", statsClients, mockDb, mockClient)
			Expect(gcmPusher).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("apiKey must be set"))
		})
	})
})
