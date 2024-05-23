/*
 * Copyright (c) 2017 TFG Co
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
	"os"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("KafkaProducer Extension", func() {
	var config *viper.Viper
	var mockProducer *mocks.KafkaProducerClientMock
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}

	BeforeEach(func() {
		var err error
		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())
		mockProducer = mocks.NewKafkaProducerClientMock()
		mockProducer.StartConsumingMessagesInProduceChannel()
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Handling Message Sent", func() {
			It("should send message", func() {
				KafkaProducer, err := NewKafkaProducer(config, logger, mockProducer)
				Expect(err).NotTo(HaveOccurred())
				KafkaProducer.SendFeedback("testgame", "apns", []byte("test message"))
				Eventually(func() int {
					return KafkaProducer.Producer.(*mocks.KafkaProducerClientMock).SentMessages
				}).Should(Equal(1))
			})

			It("should log kafka responses", func() {
				KafkaProducer, err := NewKafkaProducer(config, logger, mockProducer)
				Expect(err).NotTo(HaveOccurred())
				testTopic := "ttopic"
				KafkaProducer.Producer.Events() <- &kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic:     &testTopic,
						Partition: 0,
						Offset:    0,
						Error:     nil,
					},
				}
				Eventually(func() string {
					return hook.LastEntry().Message
				}).Should(ContainSubstring("delivered feedback to topic"))
			})
		})
	})

	Describe("[Integration]", func() {
		Describe("Creating new producer", func() {
			It("should return connected client", func() {
				kafkaProducer, err := NewKafkaProducer(config, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(kafkaProducer).NotTo(BeNil())
				Expect(kafkaProducer.Producer).NotTo(BeNil())
				Expect(kafkaProducer.Config).NotTo(BeNil())
				Expect(kafkaProducer.Logger).NotTo(BeNil())
			})
		})
	})

})
