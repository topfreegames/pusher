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
	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/mocks"
)

var _ = Describe("StatsD Extension", func() {
	var mockClient *mocks.StatsDClientMock
	logger, hook := test.NewNullLogger()
	appName := "testapp"
	BeforeEach(func() {
		mockClient = mocks.NewStatsDClientMock()
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Handling Message Sent", func() {
			It("should increment counter in statsd", func() {
				statsd, err := NewStatsD("../config/test.yaml", logger, appName, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				statsd.HandleNotificationSent()
				statsd.HandleNotificationSent()

				Expect(mockClient.Count["sent"]).To(Equal(2))
			})
		})

		Describe("Handling Message Successful", func() {
			It("should increment counter in statsd", func() {
				statsd, err := NewStatsD("../config/test.yaml", logger, appName, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				statsd.HandleNotificationSuccess()
				statsd.HandleNotificationSuccess()

				Expect(mockClient.Count["ack"]).To(Equal(2))
			})
		})

		Describe("Handling Message Failure", func() {
			It("should increment counter in statsd", func() {
				statsd, err := NewStatsD("../config/test.yaml", logger, appName, mockClient)
				Expect(err).NotTo(HaveOccurred())
				defer statsd.Cleanup()

				pErr := errors.NewPushError("some-key", "some description")

				statsd.HandleNotificationFailure(pErr)
				statsd.HandleNotificationFailure(pErr)

				Expect(mockClient.Count["failed"]).To(Equal(2))
				Expect(mockClient.Count["some-key"]).To(Equal(2))
			})
		})
	})

	Describe("[Integration]", func() {
		Describe("Creating new client", func() {
			It("should return connected client", func() {
				statsd, err := NewStatsD("../config/test.yaml", logger, appName)
				Expect(err).NotTo(HaveOccurred())
				Expect(statsd).NotTo(BeNil())
				Expect(statsd.Client).NotTo(BeNil())
				defer statsd.Cleanup()

				Expect(statsd.appName).To(Equal(appName))
				Expect(statsd.Config).NotTo(BeNil())
				Expect(statsd.ConfigFile).To(Equal("../config/test.yaml"))
				Expect(statsd.Logger).NotTo(BeNil())
			})
		})
	})
})
