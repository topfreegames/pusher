/*
 * Copyright (c) 2019 TFG Co <backend@tfgco.com>
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

package feedback

import (
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	. "github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Kafka Consumer", func() {
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	BeforeEach(func() {
		hook.Reset()
	})

	Describe("[Unit]", func() {
		var kafkaConsumerClientMock *mocks.KafkaConsumerClientMock
		var consumer *KafkaConsumer

		startConsuming := func() {
			go func() {
				defer GinkgoRecover()
				consumer.ConsumeLoop()
			}()
			time.Sleep(5 * time.Millisecond)
		}

		publishEvent := func(ev kafka.Event) {
			consumer.Consumer.Events() <- ev
			//This time.sleep is necessary to allow go's goroutines to perform work
			//Please do not remove
			time.Sleep(5 * time.Millisecond)
		}

		BeforeEach(func() {
			kafkaConsumerClientMock = mocks.NewKafkaConsumerClientMock()
			config := viper.New()
			config.Set("feedbackListeners.queue.topics", []string{"com.games.test"})
			config.Set("feedbackListeners.queue.brokers", "localhost:9941")
			config.Set("feedbackListeners.queue.group", "testGroup")
			config.Set("feedbackListeners.queue.sessionTimeout", 6000)
			config.Set("feedbackListeners.queue.offsetResetStrategy", "latest")
			config.Set("feedbackListeners.queue.handleAllMessagesBeforeExiting", true)

			var err error
			stopChannel := make(chan struct{})
			consumer, err = NewKafkaConsumer(
				config, logger,
				&stopChannel, kafkaConsumerClientMock,
			)
			Expect(err).NotTo(HaveOccurred())
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
				startConsuming()
				defer consumer.StopConsuming()
				Eventually(kafkaConsumerClientMock.SubscribedTopics, 5).Should(HaveKey("com.games.test"))
			})

			It("should assign partition", func() {
				topic := consumer.Config.GetStringSlice("feedbackListeners.queue.topics")[0]
				startConsuming()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}

				event := kafka.AssignedPartitions{
					Partitions: []kafka.TopicPartition{part},
				}
				publishEvent(event)
				Eventually(kafkaConsumerClientMock.AssignedPartitions, 5).Should(ContainElement(part))
			})

			It("should log error if fails to assign partition", func() {
				topic := consumer.Config.GetStringSlice("feedbackListeners.queue.topics")[0]
				startConsuming()
				defer consumer.StopConsuming()

				time.Sleep(5 * time.Millisecond)

				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}

				event := kafka.AssignedPartitions{
					Partitions: []kafka.TopicPartition{part},
				}

				kafkaConsumerClientMock.Error = fmt.Errorf("Failed to assign partition")
				publishEvent(event)
				Expect(hook.Entries).To(ContainLogMessage("error assigning partitions"))
			})

			It("should revoke partitions", func() {
				topic := consumer.Config.GetStringSlice("feedbackListeners.queue.topics")[0]
				startConsuming()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}
				kafkaConsumerClientMock.AssignedPartitions = []kafka.TopicPartition{part}
				Expect(kafkaConsumerClientMock.AssignedPartitions).NotTo(BeEmpty())

				event := kafka.RevokedPartitions{}
				publishEvent(event)
				Eventually(kafkaConsumerClientMock.AssignedPartitions, 5).Should(BeEmpty())
			})

			It("should stop loop if fails to revoke partitions", func() {
				topic := consumer.Config.GetStringSlice("feedbackListeners.queue.topics")[0]
				startConsuming()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}
				kafkaConsumerClientMock.AssignedPartitions = []kafka.TopicPartition{part}
				Expect(kafkaConsumerClientMock.AssignedPartitions).NotTo(BeEmpty())

				kafkaConsumerClientMock.Error = fmt.Errorf("Failed to unassign partition")
				event := kafka.RevokedPartitions{}
				publishEvent(event)
				Expect(hook.Entries).To(ContainLogMessage("error revoking partitions"))
			})

			It("should receive message", func() {
				topic := "push-games-apns-feedback"
				startConsuming()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}
				val := []byte("test")
				event := &kafka.Message{TopicPartition: part, Value: val}
				consumer.messagesReceived = 999

				publishEvent(event)
				Eventually(consumer.msgChan, 5).Should(Receive(Equal(&KafkaMessage{
					Game:     "games",
					Platform: "apns",
					Value:    val,
				})))
				Expect(consumer.messagesReceived).To(BeEquivalentTo(1000))
			})

			It("should handle partition EOF", func() {
				startConsuming()
				defer consumer.StopConsuming()

				event := kafka.PartitionEOF{}
				publishEvent(event)

				Expect(hook.Entries).To(ContainLogMessage("Reached partition EOF."))
			})

			It("should handle offsets committed", func() {
				startConsuming()
				defer consumer.StopConsuming()

				event := kafka.OffsetsCommitted{}
				publishEvent(event)

				Expect(hook.Entries).To(ContainLogMessage("Offsets committed successfully."))
			})

			It("should handle error", func() {
				startConsuming()
				defer consumer.StopConsuming()

				event := kafka.Error{}
				publishEvent(event)

				Eventually(consumer.run, 5).Should(BeFalse())
				_, ok := (<-consumer.stopChannel)
				Expect(ok).Should(BeFalse())
				Expect(hook.Entries).To(ContainLogMessage("Error in Kafka connection."))
			})

			It("should handle unexpected message", func() {
				startConsuming()
				defer consumer.StopConsuming()

				event := &mocks.MockEvent{}
				publishEvent(event)

				Expect(hook.Entries).To(ContainLogMessage("Kafka event not recognized."))
			})
		})

		Describe("Configuration Defaults", func() {
			It("should configure defaults", func() {
				cnf := viper.New()
				stopChannel := make(chan struct{})
				cons, err := NewKafkaConsumer(cnf, logger, &stopChannel, kafkaConsumerClientMock)
				Expect(err).NotTo(HaveOccurred())
				cons.loadConfigurationDefaults()

				Expect(cnf.GetStringSlice("feedbackListeners.queue.topics")).To(Equal([]string{"com.games.test"}))
				Expect(cnf.GetString("feedbackListeners.queue.brokers")).To(Equal("localhost:9092"))
				Expect(cnf.GetString("feedbackListeners.queue.group")).To(Equal("test"))
				Expect(cnf.GetInt("feedbackListeners.queue.sessionTimeout")).To(Equal(6000))
				Expect(cnf.GetString("feedbackListeners.queue.offsetResetStrategy")).To(Equal("latest"))
				Expect(cnf.GetBool("feedbackListeners.queue.handleAllMessagesBeforeExiting")).To(BeTrue())
			})
		})

		Describe("Pending Messages Waiting Group", func() {
			It("should return the waiting group", func() {
				pmwg := consumer.PendingMessagesWaitGroup()
				Expect(pmwg).NotTo(BeNil())
			})
		})

		Describe("Cleanup", func() {
			It("should stop running upon cleanup", func() {
				consumer.run = true
				err := consumer.Cleanup()
				Expect(err).NotTo(HaveOccurred())
				Expect(consumer.run).To(BeFalse())
			})

			It("should close connection to kafka upon cleanup", func() {
				err := consumer.Cleanup()
				Expect(err).NotTo(HaveOccurred())
				Expect(kafkaConsumerClientMock.Closed).To(BeTrue())
			})

			It("should return error when closing connection to kafka upon cleanup", func() {
				kafkaConsumerClientMock.Error = fmt.Errorf("Could not close connection")
				err := consumer.Cleanup()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(kafkaConsumerClientMock.Error.Error()))
			})
		})
	})

	Describe("[Integration]", func() {
		var config *viper.Viper
		var err error

		BeforeEach(func() {
			config, err = util.NewViperWithConfigFile("../config/test.yaml")
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Creating new client", func() {
			It("should return connected client", func() {
				stopChannel := make(chan struct{})
				client, err := NewKafkaConsumer(config, logger, &stopChannel)
				Expect(err).NotTo(HaveOccurred())

				Expect(client.Brokers).NotTo(HaveLen(0))
				Expect(client.Topics).To(HaveLen(1))
				Expect(client.ConsumerGroup).NotTo(BeNil())
				Expect(client.msgChan).NotTo(BeClosed())
			})
		})

		Describe("ConsumeLoop", func() {
			//This test fails while running locally due to incompatible librdkafka version
			// it should work in Travis for librdkafka v0.11.5
			It("should consume message and add it to msgChan", func() {
				logger, hook := test.NewNullLogger()
				logger.Level = logrus.DebugLevel
				stopChannel := make(chan struct{})

				game := "gametest"
				platform := "apns"
				value := []byte("Hello Go!")

				config.Set("feedbackListeners.queue.group", "new_group")
				config.Set("feedbackListeners.queue.topics", []string{"push-" + game + "-" + platform + "-feedback"})
				client, err := NewKafkaConsumer(config, logger, &stopChannel)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).NotTo(BeNil())
				defer client.StopConsuming()
				go client.ConsumeLoop()

				Eventually(func() []*logrus.Entry {
					return hook.Entries
				}, 10*time.Second).Should(ContainLogMessage("Reached partition EOF."))

				p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": client.Brokers})
				Expect(err).NotTo(HaveOccurred())
				err = p.Produce(
					&kafka.Message{
						TopicPartition: kafka.TopicPartition{
							Topic:     &client.Topics[0],
							Partition: kafka.PartitionAny,
						},
						Value: value},
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				Eventually(client.msgChan, 10*time.Second).Should(Receive(Equal(&KafkaMessage{
					Game:     game,
					Platform: platform,
					Value:    value,
				})))
			})
		})
	})
})
