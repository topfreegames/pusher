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
	"time"

	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
)

var _ = Describe("APNS Pusher", func() {
	appName := "testapp"
	certificatePath := "../tls/self_signed_cert.pem"
	configFile := "../config/test.yaml"
	isProduction := false
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		hook.Reset()
	})

	Describe("[Unit]", func() {
		var feedbackClients []interfaces.FeedbackReporter
		var mockDb *mocks.PGMock
		var mockKafkaConsumerClient *mocks.KafkaConsumerClientMock
		var mockKafkaProducerClient *mocks.KafkaProducerClientMock
		var mockPushQueue *mocks.APNSPushQueueMock
		var mockStatsDClient *mocks.StatsDClientMock
		var statsClients []interfaces.StatsReporter

		BeforeEach(func() {
			mockStatsDClient = mocks.NewStatsDClientMock()
			mockKafkaConsumerClient = mocks.NewKafkaConsumerClientMock()
			mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
			mockKafkaProducerClient.StartConsumingMessagesInProduceChannel()
			kc, err := extensions.NewKafkaProducer(configFile, logger, mockKafkaProducerClient)
			Expect(err).NotTo(HaveOccurred())

			c, err := extensions.NewStatsD(configFile, logger, appName, mockStatsDClient)
			Expect(err).NotTo(HaveOccurred())

			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			mockDb = mocks.NewPGMock(0, 1)

			mockPushQueue = mocks.NewAPNSPushQueueMock()
			hook.Reset()
		})

		Describe("Creating new apns pusher", func() {
			It("should return configured pusher", func() {
				pusher, err := NewAPNSPusher(configFile, certificatePath, appName, isProduction, logger, mockDb, mockPushQueue)
				Expect(err).NotTo(HaveOccurred())
				Expect(pusher).NotTo(BeNil())
				Expect(pusher.AppName).To(Equal(appName))
				Expect(pusher.ConfigFile).To(Equal(configFile))
				Expect(pusher.CertificatePath).To(Equal(certificatePath))
				Expect(pusher.IsProduction).To(Equal(isProduction))
				Expect(pusher.run).To(BeFalse())
				Expect(pusher.Queue).NotTo(BeNil())
				Expect(pusher.Config).NotTo(BeNil())
				Expect(pusher.MessageHandler).NotTo(BeNil())

				Expect(pusher.StatsReporters).To(HaveLen(1))
				Expect(pusher.MessageHandler.(*extensions.APNSMessageHandler).StatsReporters).To(HaveLen(1))
			})
		})
	})

	Describe("[Integration]", func() {
		// TODO: this could be an unit test is buford used a func Responses() instead of a Response field
		Describe("Start apns pusher", func() {
			It("should launch go routines and run forever", func() {
				pusher, err := NewAPNSPusher(configFile, certificatePath, appName, isProduction, logger, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(pusher).NotTo(BeNil())
				defer func() { pusher.run = false }()
				go pusher.Start()
				time.Sleep(50 * time.Millisecond)
			})
		})
	})
})
