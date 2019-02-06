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

package extensions

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	testing "github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Common", func() {
	var config *viper.Viper
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var feedbackClients []interfaces.FeedbackReporter
	var invalidTokenHandlers []interfaces.InvalidTokenHandler
	var db *mocks.PGMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	configFile := "../config/test.yaml"
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	Describe("[Unit]", func() {
		BeforeEach(func() {
			var err error
			config, err = util.NewViperWithConfigFile(configFile)
			Expect(err).NotTo(HaveOccurred())
			mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
			kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
			Expect(err).NotTo(HaveOccurred())

			mockStatsDClient = mocks.NewStatsDClientMock()
			c, err := NewStatsD(config, logger, mockStatsDClient)
			Expect(err).NotTo(HaveOccurred())

			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			db = mocks.NewPGMock(0, 1)
			it, err := NewTokenPG(config, logger, statsClients, db)
			Expect(err).NotTo(HaveOccurred())
			invalidTokenHandlers = []interfaces.InvalidTokenHandler{it}

			hook.Reset()
		})

		Describe("Handle token error", func() {
			It("should be successful", func() {
				token := uuid.NewV4().String()
				handleInvalidToken(invalidTokenHandlers, token, "test", "apns")
				query := "DELETE FROM test_apns WHERE token = ?0;"

				Eventually(func() [][]interface{} { return db.Execs }).
					Should(HaveLen(2))
				Eventually(func() interface{} { return db.Execs[1][0] }).
					Should(BeEquivalentTo(query))
				Eventually(func() interface{} { return db.Execs[1][1] }).
					Should(BeEquivalentTo([]interface{}{token}))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeleted] }).
					Should(Equal(1))
			})

			It("should fail silently", func() {
				token := uuid.NewV4().String()
				db.Error = fmt.Errorf("pg: error")
				handleInvalidToken(invalidTokenHandlers, token, "test", "apns")

				Eventually(func() [][]interface{} { return db.Execs }).
					Should(HaveLen(2))
				query := "DELETE FROM test_apns WHERE token = ?0;"
				Eventually(func() interface{} { return db.Execs[1][0] }).
					Should(BeEquivalentTo(query))
				Eventually(func() interface{} { return db.Execs[1][1] }).
					Should(BeEquivalentTo([]interface{}{token}))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeletionError] }).
					Should(Equal(1))
			})

			Describe("should stop handler gracefully", func() {
				It("if there's no more job to do", func() {
					token := uuid.NewV4().String()
					handleInvalidToken(invalidTokenHandlers, token, "test", "apns")

					timeout := 1 * time.Second
					StopInvalidTokenHandlers(logger, invalidTokenHandlers, timeout)
					Consistently(func() []*logrus.Entry { return hook.Entries }, 2*timeout).
						ShouldNot(testing.ContainLogMessage("timeout reached - invalidTokenHandler was closed with jobs in queue"))

					Eventually(func() [][]interface{} { return db.Execs }).
						Should(HaveLen(2))
					query := "DELETE FROM test_apns WHERE token = ?0;"
					Eventually(func() interface{} { return db.Execs[1][0] }).
						Should(BeEquivalentTo(query))
					Eventually(func() interface{} { return db.Execs[1][1] }).
						Should(BeEquivalentTo([]interface{}{token}))
				})

				It("timeout reached", func() {
					size := 1000
					for i := 0; i < size; i++ {
						token := uuid.NewV4().String()
						handleInvalidToken(invalidTokenHandlers, token, "test", "apns")
					}

					timeout := 1 * time.Nanosecond
					StopInvalidTokenHandlers(logger, invalidTokenHandlers, timeout)
					Eventually(func() []*logrus.Entry { return hook.Entries }).
						Should(testing.ContainLogMessage("timeout reached - invalidTokenHandler was closed with jobs in queue"))
				})
			})
		})

		Describe("Send feedback to reporters", func() {
			It("should return an error if res cannot be marshaled", func() {
				badContent := make(chan int)
				err := sendToFeedbackReporters(feedbackClients, badContent, ParsedTopic{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("json: unsupported type: chan int"))
			})
		})
	})
})
