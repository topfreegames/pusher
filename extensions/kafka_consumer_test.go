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

	"github.com/Sirupsen/logrus/hooks/test"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Kafka Extension", func() {
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		hook.Reset()
	})

	FDescribe("[Unit]", func() {
		var kafkaConsumerClientMock *mocks.KafkaConsumerClientMock
		var consumer *KafkaConsumer

		BeforeEach(func() {
			kafkaConsumerClientMock = mocks.NewKafkaConsumerClientMock()
			config := viper.New()
			config.Set("queue.topics", []string{"com.games.test"})
			config.Set("queue.brokers", "localhost:9941")
			config.Set("queue.group", "testGroup")
			config.Set("queue.sessionTimeout", 6000)
			config.Set("queue.offsetResetStrategy", "latest")
			config.Set("queue.handleAllMessagesBeforeExiting", true)

			consumer = NewKafkaConsumer(config, logger, kafkaConsumerClientMock)
		})

		Describe("Creating new client", func() {
			It("should return connected client", func() {
				Expect(consumer.Brokers).NotTo(HaveLen(0))
				Expect(consumer.Topics).To(HaveLen(1))
				Expect(consumer.ConsumerGroup).NotTo(BeNil())
				Expect(consumer.msgChan).NotTo(BeClosed())
				Expect(consumer.Consumer).To(Equal(kafkaConsumerClientMock))
			})
		})

		Describe("Stop consuming", func() {
			It("should stop consuming", func() {
				consumer.run = true
				consumer.StopConsuming()
				Expect(consumer.run).To(BeFalse())
			})
		})

		Describe("Consume loop", func() {
			It("should fail if subscribing to topic fails", func() {
				kafkaConsumerClientMock.Error = fmt.Errorf("could not subscribe")
				err := consumer.ConsumeLoop()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not subscribe"))
			})

			It("should subscribe to topic", func() {
				go func() {
					err := consumer.ConsumeLoop()
					Expect(err).NotTo(HaveOccurred())
				}()
				defer consumer.StopConsuming()
				Eventually(kafkaConsumerClientMock.SubscribedTopics, 5).Should(HaveKey("com.games.test"))
			})

			It("should assign partition", func() {
				topic := consumer.Config.GetStringSlice("queue.topics")[0]
				go func() {
					err := consumer.ConsumeLoop()
					Expect(err).NotTo(HaveOccurred())
				}()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}

				event := kafka.AssignedPartitions{
					Partitions: []kafka.TopicPartition{part},
				}
				consumer.Consumer.Events() <- event
				//This time.sleep is necessary to allow go's goroutines to perform work
				//Please do not remove
				time.Sleep(5 * time.Millisecond)
				Eventually(kafkaConsumerClientMock.AssignedPartitions, 5).Should(ContainElement(part))
			})

			It("should revoke partition", func() {
				topic := consumer.Config.GetStringSlice("queue.topics")[0]
				go func() {
					err := consumer.ConsumeLoop()
					Expect(err).NotTo(HaveOccurred())
				}()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}
				kafkaConsumerClientMock.AssignedPartitions = []kafka.TopicPartition{part}
				Expect(kafkaConsumerClientMock.AssignedPartitions).NotTo(BeEmpty())

				event := kafka.RevokedPartitions{}
				consumer.Consumer.Events() <- event
				//This time.sleep is necessary to allow go's goroutines to perform work
				//Please do not remove
				time.Sleep(5 * time.Millisecond)
				Eventually(kafkaConsumerClientMock.AssignedPartitions, 5).Should(BeEmpty())
			})

			It("should receive message", func() {
				topic := consumer.Config.GetStringSlice("queue.topics")[0]
				go func() {
					err := consumer.ConsumeLoop()
					Expect(err).NotTo(HaveOccurred())
				}()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}
				val := []byte("test")
				event := &kafka.Message{TopicPartition: part, Value: val}
				consumer.messagesReceived = 999

				consumer.Consumer.Events() <- event
				//This time.sleep is necessary to allow go's goroutines to perform work
				//Please do not remove
				time.Sleep(5 * time.Millisecond)
				Eventually(consumer.msgChan, 5).Should(Receive(&val))
				Expect(consumer.messagesReceived).To(BeEquivalentTo(1000))
			})

		})
	})

	Describe("[Integration]", func() {
		var config *viper.Viper
		BeforeEach(func() {
			config = util.NewViperWithConfigFile("../config/test.yaml")
		})

		Describe("Creating new client", func() {
			It("should return connected client", func() {
				client := NewKafkaConsumer(config, logger)

				Expect(client.Brokers).NotTo(HaveLen(0))
				Expect(client.Topics).To(HaveLen(1))
				Expect(client.ConsumerGroup).NotTo(BeNil())
				Expect(client.msgChan).NotTo(BeClosed())
			})
		})

		Describe("ConsumeLoop", func() {
			It("should consume message and add it to msgChan", func() {
				client := NewKafkaConsumer(config, logger)
				Expect(client).NotTo(BeNil())
				defer client.StopConsuming()
				go client.ConsumeLoop()

				time.Sleep(10 * time.Second)
				p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": client.Brokers})
				Expect(err).NotTo(HaveOccurred())
				err = p.Produce(
					&kafka.Message{
						TopicPartition: kafka.TopicPartition{
							Topic:     &client.Topics[0],
							Partition: kafka.PartitionAny,
						},
						Value: []byte("Hello Go!")},
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				Eventually(client.msgChan, 10*time.Second).Should(Receive(Equal([]byte("Hello Go!"))))
			})
		})
	})
})
