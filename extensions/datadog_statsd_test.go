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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("StatsD Extension", func() {
	var config *viper.Viper
	var mockClient *mocks.StatsDClientMock
	logger, hook := test.NewNullLogger()
	BeforeEach(func() {
		var err error
		config, err = util.NewViperWithConfigFile("../config/test.yaml")
		Expect(err).NotTo(HaveOccurred())
		mockClient = mocks.NewStatsDClientMock()
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Handling Message Sent", func() {
			It("should increment counter in statsd", func() {
				statsd, err := NewStatsD(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				statsd.HandleNotificationSent("game", "apns")
				statsd.HandleNotificationSent("game", "apns")
				Expect(mockClient.Count["sent"]).To(Equal(2))
			})
		})

		Describe("Handling Message Successful", func() {
			It("should increment counter in statsd", func() {
				statsd, err := NewStatsD(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				statsd.HandleNotificationSuccess("game", "apns")
				statsd.HandleNotificationSuccess("game", "apns")
				Expect(mockClient.Count["ack"]).To(Equal(2))
			})
		})

		Describe("Reporting Go Stats", func() {
			It("should report go stats in statsd", func() {
				statsd, err := NewStatsD(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				statsd.ReportGoStats(1, 2, 3, 4, 5000000)
				statsd.ReportGoStats(2, 3, 4, 5, 6000000)

				Expect(mockClient.Gauges["num_goroutine"]).To(BeEquivalentTo(2))
				Expect(mockClient.Gauges["allocated_not_freed"]).To(BeEquivalentTo(3))
				Expect(mockClient.Gauges["heap_objects"]).To(BeEquivalentTo(4))
				Expect(mockClient.Gauges["next_gc_bytes"]).To(BeEquivalentTo(5))
			})
		})

		Describe("Handling Message Failure", func() {
			It("should increment counter in statsd", func() {
				statsd, err := NewStatsD(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				pErr := errors.NewPushError("some-key", "some description")

				statsd.HandleNotificationFailure("game", "apns", pErr)
				statsd.HandleNotificationFailure("game", "apns", pErr)

				Expect(mockClient.Count["failed"]).To(Equal(2))
			})
		})

		Describe("Reporting metric increment", func() {
			It("should report metric increment in statsd", func() {
				statsd, err := NewStatsD(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				statsd.ReportMetricIncrement("tokens_to_delete", "game", "apns")
				statsd.ReportMetricIncrement("tokens_to_delete", "game", "apns")
				statsd.ReportMetricIncrement("tokens_to_delete", "game", "apns")

				statsd.ReportMetricIncrement("tokens_deleted", "game", "apns")
				statsd.ReportMetricIncrement("tokens_deleted", "game", "apns")

				statsd.ReportMetricIncrement("tokens_deletion_failed", "game", "apns")

				Expect(mockClient.Count["tokens_to_delete"]).To(Equal(3))
				Expect(mockClient.Count["tokens_deleted"]).To(Equal(2))
				Expect(mockClient.Count["tokens_deletion_failed"]).To(Equal(1))
			})
		})
	})

	Describe("[Integration]", func() {
		Describe("Creating new client", func() {
			It("should return connected client", func() {
				statsd, err := NewStatsD(config, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(statsd).NotTo(BeNil())
				Expect(statsd.Client).NotTo(BeNil())
				defer statsd.Cleanup()

				Expect(statsd.Config).NotTo(BeNil())
				Expect(statsd.Logger).NotTo(BeNil())
			})
		})
	})
})
