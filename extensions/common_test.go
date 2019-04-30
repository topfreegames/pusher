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
	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Common", func() {
	var config *viper.Viper
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var feedbackClients []interfaces.FeedbackReporter

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
			feedbackClients = []interfaces.FeedbackReporter{kc}
			hook.Reset()
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
