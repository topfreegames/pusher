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
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
)

var _ = Describe("APNS", func() {
	cfg := "../config/test.yaml"
	cert := "../tls/self_signed_cert.pem"
	logger, _ := test.NewNullLogger()

	var mockPushQueue *mocks.APNSPushQueueMock
	var mockDb *mocks.PGMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	BeforeEach(func() {
		mockDb = mocks.NewPGMock(0, 1)
		mockPushQueue = mocks.NewAPNSPushQueueMock()
		mockStatsDClient = mocks.NewStatsDClientMock()
		c, err := extensions.NewStatsD(cfg, logger, mockStatsDClient)
		Expect(err).NotTo(HaveOccurred())
		statsClients = []interfaces.StatsReporter{c}
	})

	Describe("[Unit]", func() {
		It("Should return apnsPusher without errors", func() {
			apnsPusher, err := startApns(false, false, false, cfg, cert, statsClients, mockDb, mockPushQueue)
			Expect(err).NotTo(HaveOccurred())
			Expect(apnsPusher).NotTo(BeNil())
			Expect(apnsPusher.CertificatePath).To(Equal(cert))
			Expect(apnsPusher.ConfigFile).To(Equal(cfg))
			Expect(apnsPusher.IsProduction).To(BeFalse())
			Expect(apnsPusher.Logger.Level).To(Equal(logrus.InfoLevel))
			Expect(fmt.Sprintf("%T", apnsPusher.Logger.Formatter)).To(Equal(fmt.Sprintf("%T", &logrus.TextFormatter{})))
		})

		It("Should set log to json format", func() {
			apnsPusher, err := startApns(false, true, false, cfg, cert, statsClients, mockDb, mockPushQueue)
			Expect(err).NotTo(HaveOccurred())
			Expect(apnsPusher).NotTo(BeNil())
			Expect(fmt.Sprintf("%T", apnsPusher.Logger.Formatter)).To(Equal(fmt.Sprintf("%T", &logrus.JSONFormatter{})))
		})

		It("Should set log to debug", func() {
			apnsPusher, err := startApns(true, false, false, cfg, cert, statsClients, mockDb, mockPushQueue)
			Expect(err).NotTo(HaveOccurred())
			Expect(apnsPusher).NotTo(BeNil())
			Expect(apnsPusher.Logger.Level).To(Equal(logrus.DebugLevel))
		})

		It("Should set log to production", func() {
			apnsPusher, err := startApns(false, false, true, cfg, cert, statsClients, mockDb, mockPushQueue)
			Expect(err).NotTo(HaveOccurred())
			Expect(apnsPusher).NotTo(BeNil())
			Expect(apnsPusher.IsProduction).To(BeTrue())
		})

		It("Should return error if certificate is not provided", func() {
			apnsPusher, err := startApns(false, true, false, cfg, "", statsClients, mockDb, mockPushQueue)
			Expect(apnsPusher).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("pem certificate must be set"))
		})
	})
})
