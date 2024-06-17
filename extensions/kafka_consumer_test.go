package extensions

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Kafka Extension", func() {
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
				goFuncErr := consumer.ConsumeLoop(context.Background())
				Expect(goFuncErr).NotTo(HaveOccurred())
			}()
			time.Sleep(5 * time.Millisecond)
		}

		publishEvent := func(ev *kafka.Message) {
			consumer.Consumer.(*mocks.KafkaConsumerClientMock).MessagesChan <- ev
			//This time.sleep is necessary to allow go's goroutines to perform work
			//Please do not remove
			time.Sleep(5 * time.Millisecond)
		}

		BeforeEach(func() {
			kafkaConsumerClientMock = mocks.NewKafkaConsumerClientMock()
			config := viper.New()
			config.Set("queue.topics", []string{"com.games.test"})
			config.Set("queue.brokers", "localhost:9941")
			config.Set("queue.group", "testGroup")
			config.Set("queue.sessionTimeout", 6000)
			config.Set("queue.offsetResetStrategy", "latest")
			config.Set("queue.handleAllMessagesBeforeExiting", true)

			var err error
			stopChannel := make(chan struct{})
			consumer, err = NewKafkaConsumer(
				config, logger,
				stopChannel, kafkaConsumerClientMock,
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
				consumer.StopConsuming()
				Expect(consumer.stopChannel).To(BeClosed())
			})
		})

		Describe("Consume loop", func() {
			It("should fail if subscribing to topic fails", func() {
				kafkaConsumerClientMock.Error = fmt.Errorf("could not subscribe")
				err := consumer.ConsumeLoop(context.Background())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not subscribe"))
			})

			It("should subscribe to topic", func() {
				startConsuming()

				time.Sleep(100 * time.Millisecond)
				consumer.StopConsuming()
				time.Sleep(100 * time.Millisecond)

				Expect(kafkaConsumerClientMock.SubscribedTopics).To(HaveKey("com.games.test"))
			})

			It("should receive message", func() {
				topic := "push-games_apns-single"
				startConsuming()
				defer consumer.StopConsuming()
				part := kafka.TopicPartition{
					Topic:     &topic,
					Partition: 1,
				}
				val := []byte("test")
				event := &kafka.Message{TopicPartition: part, Value: val}

				publishEvent(event)
				Eventually(consumer.msgChan, 5).Should(Receive(&interfaces.KafkaMessage{
					Topic: topic,
					Value: val,
				}))
			})
		})

		Describe("Configuration Defaults", func() {
			It("should configure defaults", func() {
				cnf := viper.New()
				stopChannel := make(chan struct{})
				cons, err := NewKafkaConsumer(cnf, logger, stopChannel, kafkaConsumerClientMock)
				Expect(err).NotTo(HaveOccurred())
				cons.loadConfigurationDefaults()

				Expect(cnf.GetStringSlice("queue.topics")).To(Equal([]string{"com.games.test"}))
				Expect(cnf.GetString("queue.brokers")).To(Equal("localhost:9092"))
				Expect(cnf.GetString("queue.group")).To(Equal("test"))
				Expect(cnf.GetInt("queue.sessionTimeout")).To(Equal(6000))
				Expect(cnf.GetString("queue.offsetResetStrategy")).To(Equal("latest"))
				Expect(cnf.GetBool("queue.handleAllMessagesBeforeExiting")).To(BeTrue())
			})
		})

		Describe("Pending Messages Waiting Group", func() {
			It("should return the waiting group", func() {
				pendingMessagesWaitGroup := consumer.PendingMessagesWaitGroup()
				Expect(pendingMessagesWaitGroup).NotTo(BeNil())
			})
		})

		Describe("Cleanup", func() {
			It("should stop running upon cleanup", func() {
				err := consumer.Cleanup()
				Expect(err).NotTo(HaveOccurred())
			})

			It("should close connection to kafka upon cleanup", func() {
				err := consumer.Cleanup()
				Expect(err).NotTo(HaveOccurred())
				Expect(kafkaConsumerClientMock.Closed).To(BeTrue())
			})

			It("should return error when closing connection to kafka upon cleanup", func() {
				kafkaConsumerClientMock.Error = fmt.Errorf("could not close connection")
				err := consumer.Cleanup()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(kafkaConsumerClientMock.Error.Error()))
			})
		})
	})

	Describe("[Integration]", func() {
		var config *viper.Viper
		var err error
		configFile := os.Getenv("CONFIG_FILE")
		if configFile == "" {
			configFile = "../config/test.yaml"
		}

		BeforeEach(func() {
			config, err = util.NewViperWithConfigFile(configFile)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("Creating new client", func() {
			It("should return connected client", func() {
				stopChannel := make(chan struct{})
				client, err := NewKafkaConsumer(config, logger, stopChannel)
				Expect(err).NotTo(HaveOccurred())

				Expect(client.Brokers).NotTo(HaveLen(0))
				Expect(client.Topics).To(HaveLen(1))
				Expect(client.ConsumerGroup).NotTo(BeNil())
				Expect(client.msgChan).NotTo(BeClosed())
			})
		})

		PDescribe("ConsumeLoop", func() {
			It("should consume message and add it to msgChan", func() {
				stopChannel := make(chan struct{})
				client, err := NewKafkaConsumer(config, logger, stopChannel)
				Expect(err).NotTo(HaveOccurred())
				Expect(client).NotTo(BeNil())
				defer client.StopConsuming()
				go func() {
					goFuncErr := client.ConsumeLoop(context.Background())
					Expect(goFuncErr).NotTo(HaveOccurred())
				}()

				// Required to assure the consumer to be ready before producing a message
				time.Sleep(5 * time.Second)

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
				Eventually(client.msgChan, 100*time.Millisecond).WithTimeout(5 * time.Second).Should(Receive(Equal([]byte("Hello Go!"))))
			})
		})
	})
})
