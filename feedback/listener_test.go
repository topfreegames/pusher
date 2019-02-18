// +build integration

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
	"encoding/json"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	gcm "github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/structs"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Feedback Listener", func() {
	configFile := "../config/test.yaml"
	var config *viper.Viper
	var db interfaces.DB
	var game1, game2 string
	var platform string
	var err error

	BeforeSuite(func() {
		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())

		pgClient, err := extensions.NewPGClient("feedbackListeners.invalidToken.pg", config)
		Expect(err).NotTo(HaveOccurred())
		db = pgClient.DB

		game1 = "sniper"
		game2 = "warheroes"

		for _, platform := range []string{"gcm", "apns"} {
			for _, game := range []string{game1, game2} {
				_, err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s_%s(
					"id" uuid DEFAULT uuid_generate_v4(),
					"user_id" text NOT NULL,
					"token" text NOT NULL,
					"region" text NOT NULL,
					"locale" text NOT NULL,
					"tz" text NOT NULL,
					PRIMARY KEY ("id")
				)`, game, platform))
				Expect(err).NotTo(HaveOccurred())
			}
		}
	})

	AfterSuite(func() {
		for _, platform := range []string{"gcm", "apns"} {
			for _, game := range []string{game1, game2} {
				_, err := db.Exec(fmt.Sprintf(`DROP TABLE IF  EXISTS %s_%s;`, game, platform))
				Expect(err).NotTo(HaveOccurred())
			}
		}
	})

	Describe("[Integration2]", func() {
		Describe("Create a new instance of listener", func() {
			var logger *logrus.Logger

			BeforeEach(func() {
				logger, _ = test.NewNullLogger()
			})

			It("should return a configured listener", func() {
				listener, err := NewListener(config, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(listener).NotTo(BeNil())
				Expect(listener.Queue).NotTo(BeNil())
				Expect(listener.Broker).NotTo(BeNil())
				Expect(listener.InvalidTokenHandler).NotTo(BeNil())
			})
		})

		Describe("Listener Use", func() {
			Describe("From GCM", func() {
				var feedbacks map[string][]*gcm.CCSMessage

				BeforeEach(func() {
					platform = "gcm"

					brokers := config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					// to make sure that the consumer will be assigned to the necessary
					// topics since the beginning
					for _, game := range []string{game1, game2} {
						topic := "push-" + game + "-" + platform + "-feedbacks"
						eventChan := make(chan kafka.Event)

						p.Produce(&kafka.Message{
							TopicPartition: kafka.TopicPartition{
								Topic:     &topic,
								Partition: kafka.PartitionAny,
							},
							Value: []byte{}},
							eventChan)
						Eventually(eventChan, 5*time.Second).Should(Receive())
					}

					feedbacks = make(map[string][]*gcm.CCSMessage)
					tokens := make(map[string][]string)

					tokens[game1] = []string{
						"AAAA-AAAA-AAAA",
						"BBBB-BBBB-BBBB",
						"CCCC-CCCC-CCCC",
					}
					tokens[game2] = []string{
						"DDDD-DDDD-DDDD",
						"EEEE-EEEE-EEEE",
						"FFFF-FFFF-FFFF",
					}

					for _, game := range []string{game1, game2} {
						feedbacks[game] = make([]*gcm.CCSMessage, 3)
						for i := range feedbacks[game] {
							feedbacks[game][i] = &gcm.CCSMessage{
								From:  tokens[game][i],
								Error: "DEVICE_UNREGISTERED",
							}
						}
					}
				})

				It("should delete a single token from a game", func() {
					logger, _ := test.NewNullLogger()
					logger.Level = logrus.DebugLevel

					listener, err := NewListener(config, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(listener).NotTo(BeNil())
					Expect(listener.Queue).NotTo(BeNil())
					Expect(listener.Broker).NotTo(BeNil())
					Expect(listener.InvalidTokenHandler).NotTo(BeNil())

					brokers := listener.Config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					listener.Queue.(*KafkaConsumer).AssignedPartition = false
					go listener.Start()

					// wait consumer start to consume message before send it
					for listener.Queue.(*KafkaConsumer).AssignedPartition == false {
						time.Sleep(10 * time.Millisecond)
					}

					game := game1
					topic := "push-" + game + "-" + platform + "-feedbacks"

					deviceToken := feedbacks[game][0].From
					_, err = db.Exec(fmt.Sprintf(`
						INSERT INTO %s_%s (id, user_id, token, region, locale, tz)
						VALUES (?0, ?1, ?2, ?3, ?4,?5)
					`, game, platform),
						uuid.NewV4(), uuid.NewV4(), deviceToken, "br", "PT", "-300")
					Expect(err).NotTo(HaveOccurred())

					value, err := json.Marshal(feedbacks[game][0])
					Expect(err).NotTo(HaveOccurred())

					eventsChan := make(chan kafka.Event)
					err = p.Produce(
						&kafka.Message{
							TopicPartition: kafka.TopicPartition{
								Topic:     &topic,
								Partition: kafka.PartitionAny,
							},
							Value: value},
						eventsChan,
					)
					Expect(err).NotTo(HaveOccurred())
					<-eventsChan

					Eventually(func() int {
						res, err := db.Exec(fmt.Sprintf(`SELECT FROM %s_%s
						WHERE token = ?0`, game, platform), deviceToken)
						Expect(err).NotTo(HaveOccurred())
						return res.RowsReturned()
					}, 15*time.Second).Should(Equal(0))

					listener.Stop()
				})

				It("should delete a batch of tokens from a single game", func() {
					logger, _ := test.NewNullLogger()
					logger.Level = logrus.DebugLevel

					listener, err := NewListener(config, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(listener).NotTo(BeNil())
					Expect(listener.Queue).NotTo(BeNil())
					Expect(listener.Broker).NotTo(BeNil())
					Expect(listener.InvalidTokenHandler).NotTo(BeNil())

					brokers := listener.Config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					listener.Queue.(*KafkaConsumer).AssignedPartition = false
					go listener.Start()

					// wait consumer start to consume message before send it
					for listener.Queue.(*KafkaConsumer).AssignedPartition == false {
						time.Sleep(10 * time.Millisecond)
					}

					game := game1
					topic := "push-" + game + "-" + platform + "-feedbacks"

					for _, msg := range feedbacks[game] {
						deviceToken := msg.From

						_, err = db.Exec(fmt.Sprintf(`
						INSERT INTO %s_%s (id, user_id, token, region, locale, tz)
						VALUES (?0, ?1, ?2, ?3, ?4,?5)
						`, game, platform),
							uuid.NewV4(), uuid.NewV4(), deviceToken, "br", "PT", "-300")
						Expect(err).NotTo(HaveOccurred())
					}

					for _, msg := range feedbacks[game] {
						value, err := json.Marshal(msg)
						Expect(err).NotTo(HaveOccurred())

						eventsChan := make(chan kafka.Event)
						err = p.Produce(
							&kafka.Message{
								TopicPartition: kafka.TopicPartition{
									Topic:     &topic,
									Partition: kafka.PartitionAny,
								},
								Value: value},
							eventsChan,
						)
						Expect(err).NotTo(HaveOccurred())
						<-eventsChan
					}

					for _, msg := range feedbacks[game] {
						deviceToken := msg.From

						Eventually(func() int {
							res, err := db.Exec(fmt.Sprintf(`SELECT FROM %s_%s
							WHERE token = ?0`, game, platform), deviceToken)
							Expect(err).NotTo(HaveOccurred())
							return res.RowsReturned()
						}, 15*time.Second).Should(Equal(0))
					}

					listener.Stop()
				})

				It("should delete a batch of tokens from different games", func() {
					logger, _ := test.NewNullLogger()
					logger.Level = logrus.DebugLevel

					listener, err := NewListener(config, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(listener).NotTo(BeNil())
					Expect(listener.Queue).NotTo(BeNil())
					Expect(listener.Broker).NotTo(BeNil())
					Expect(listener.InvalidTokenHandler).NotTo(BeNil())

					brokers := listener.Config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					listener.Queue.(*KafkaConsumer).AssignedPartition = false
					go listener.Start()

					// wait consumer start to consume message before send it
					for listener.Queue.(*KafkaConsumer).AssignedPartition == false {
						time.Sleep(10 * time.Millisecond)
					}

					topics := make(map[string]string)
					topics[game1] = "push-" + game1 + "-" + platform + "-feedbacks"
					topics[game2] = "push-" + game2 + "-" + platform + "-feedbacks"

					for _, game := range []string{game1, game2} {
						for _, msg := range feedbacks[game] {
							deviceToken := msg.From

							_, err = db.Exec(fmt.Sprintf(`
							INSERT INTO %s_%s (id, user_id, token, region, locale, tz)
							VALUES (?0, ?1, ?2, ?3, ?4,?5)
							`, game, platform),
								uuid.NewV4(), uuid.NewV4(), deviceToken, "br", "PT", "-300")
							Expect(err).NotTo(HaveOccurred())
						}
					}

					msgs := make([]*kafka.Message, 0, len(feedbacks[game1])+len(feedbacks[game2]))
					for _, game := range []string{game1, game2} {
						topic := topics[game]
						for _, msg := range feedbacks[game] {
							value, err := json.Marshal(msg)
							Expect(err).NotTo(HaveOccurred())

							msgs = append(msgs, &kafka.Message{
								TopicPartition: kafka.TopicPartition{
									Topic:     &topic,
									Partition: kafka.PartitionAny,
								},
								Value: value,
							})
						}
					}

					for _, msg := range msgs {
						eventsChan := make(chan kafka.Event)

						err = p.Produce(msg, eventsChan)
						<-eventsChan
						Expect(err).NotTo(HaveOccurred())
					}

					for _, game := range []string{game1, game2} {
						for _, msg := range feedbacks[game] {
							deviceToken := msg.From

							Eventually(func() int {
								res, err := db.Exec(fmt.Sprintf(`SELECT FROM %s_%s
								WHERE token = ?0`, game, platform), deviceToken)
								Expect(err).NotTo(HaveOccurred())
								return res.RowsReturned()
							}, 30*time.Second).Should(Equal(0))
						}
					}
					listener.Stop()
				})
			})

			Describe("From APNS", func() {
				var feedbacks map[string][]*structs.ResponseWithMetadata

				BeforeEach(func() {
					platform = "apns"

					brokers := config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					// to make sure that the consumer will be assigned to the necessary
					// topics since the beginning
					for _, game := range []string{game1, game2} {
						topic := "push-" + game + "-" + platform + "-feedbacks"
						eventChan := make(chan kafka.Event)

						p.Produce(&kafka.Message{
							TopicPartition: kafka.TopicPartition{
								Topic:     &topic,
								Partition: kafka.PartitionAny,
							},
							Value: []byte{}},
							eventChan)
						Eventually(eventChan, 5*time.Second).Should(Receive())
					}

					feedbacks = make(map[string][]*structs.ResponseWithMetadata)
					tokens := make(map[string][]string)

					tokens[game1] = []string{
						"AAAA-AAAA-AAAA",
						"BBBB-BBBB-BBBB",
						"CCCC-CCCC-CCCC",
					}

					tokens[game2] = []string{
						"DDDD-DDDD-DDDD",
						"EEEE-EEEE-EEEE",
						"FFFF-FFFF-FFFF",
					}

					for _, game := range []string{game1, game2} {
						feedbacks[game] = make([]*structs.ResponseWithMetadata, 3)
						for i := range feedbacks[game] {
							feedbacks[game][i] = &structs.ResponseWithMetadata{
								StatusCode:  400,
								ApnsID:      uuid.NewV4().String(),
								Reason:      apns2.ReasonUnregistered,
								DeviceToken: tokens[game][i],
							}
						}
					}
				})

				It("should delete a single token from a game", func() {
					logger, _ := test.NewNullLogger()
					logger.Level = logrus.DebugLevel

					listener, err := NewListener(config, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(listener).NotTo(BeNil())
					Expect(listener.Queue).NotTo(BeNil())
					Expect(listener.Broker).NotTo(BeNil())
					Expect(listener.InvalidTokenHandler).NotTo(BeNil())

					brokers := listener.Config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					listener.Queue.(*KafkaConsumer).AssignedPartition = false
					go listener.Start()

					// wait consumer start to consume message before send it
					for listener.Queue.(*KafkaConsumer).AssignedPartition == false {
						time.Sleep(10 * time.Millisecond)
					}

					game := game1
					topic := "push-" + game + "-" + platform + "-feedbacks"

					deviceToken := feedbacks[game][0].DeviceToken
					_, err = db.Exec(fmt.Sprintf(`
						INSERT INTO %s_%s (id, user_id, token, region, locale, tz)
						VALUES (?0, ?1, ?2, ?3, ?4,?5)
					`, game, platform),
						uuid.NewV4(), uuid.NewV4(), deviceToken, "br", "PT", "-300")
					Expect(err).NotTo(HaveOccurred())

					value, err := json.Marshal(feedbacks[game][0])
					Expect(err).NotTo(HaveOccurred())

					eventsChan := make(chan kafka.Event)
					err = p.Produce(
						&kafka.Message{
							TopicPartition: kafka.TopicPartition{
								Topic:     &topic,
								Partition: kafka.PartitionAny,
							},
							Value: value},
						eventsChan,
					)
					Expect(err).NotTo(HaveOccurred())
					<-eventsChan

					Eventually(func() int {
						res, err := db.Exec(fmt.Sprintf(`SELECT FROM %s_%s
						WHERE token = ?0`, game, platform), deviceToken)
						Expect(err).NotTo(HaveOccurred())
						return res.RowsReturned()
					}, 15*time.Second).Should(Equal(0))

					listener.Stop()
				})

				It("should delete a batch of tokens from a single game", func() {
					logger, _ := test.NewNullLogger()
					logger.Level = logrus.DebugLevel

					listener, err := NewListener(config, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(listener).NotTo(BeNil())
					Expect(listener.Queue).NotTo(BeNil())
					Expect(listener.Broker).NotTo(BeNil())
					Expect(listener.InvalidTokenHandler).NotTo(BeNil())

					brokers := listener.Config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					listener.Queue.(*KafkaConsumer).AssignedPartition = false
					go listener.Start()

					// wait consumer start to consume message before send it
					for listener.Queue.(*KafkaConsumer).AssignedPartition == false {
						time.Sleep(10 * time.Millisecond)
					}

					game := game1
					topic := "push-" + game + "-" + platform + "-feedbacks"

					for _, msg := range feedbacks[game] {
						deviceToken := msg.DeviceToken

						_, err = db.Exec(fmt.Sprintf(`
						INSERT INTO %s_%s (id, user_id, token, region, locale, tz)
						VALUES (?0, ?1, ?2, ?3, ?4,?5)
						`, game, platform),
							uuid.NewV4(), uuid.NewV4(), deviceToken, "br", "PT", "-300")
						Expect(err).NotTo(HaveOccurred())
					}

					for _, msg := range feedbacks[game] {
						value, err := json.Marshal(msg)
						Expect(err).NotTo(HaveOccurred())

						eventsChan := make(chan kafka.Event)
						err = p.Produce(
							&kafka.Message{
								TopicPartition: kafka.TopicPartition{
									Topic:     &topic,
									Partition: kafka.PartitionAny,
								},
								Value: value},
							eventsChan,
						)
						Expect(err).NotTo(HaveOccurred())
						<-eventsChan
					}

					for _, msg := range feedbacks[game] {
						deviceToken := msg.DeviceToken

						Eventually(func() int {
							res, err := db.Exec(fmt.Sprintf(`SELECT FROM %s_%s
							WHERE token = ?0`, game, platform), deviceToken)
							Expect(err).NotTo(HaveOccurred())
							return res.RowsReturned()
						}, 15*time.Second).Should(Equal(0))
					}

					listener.Stop()
				})

				It("should delete a batch of tokens from different games", func() {
					logger, _ := test.NewNullLogger()
					logger.Level = logrus.DebugLevel

					listener, err := NewListener(config, logger)
					Expect(err).NotTo(HaveOccurred())
					Expect(listener).NotTo(BeNil())
					Expect(listener.Queue).NotTo(BeNil())
					Expect(listener.Broker).NotTo(BeNil())
					Expect(listener.InvalidTokenHandler).NotTo(BeNil())

					brokers := listener.Config.GetString("feedbackListeners.queue.brokers")
					p, err := kafka.NewProducer(&kafka.ConfigMap{
						"bootstrap.servers": brokers,
						"linger.ms":         1,
					})
					Expect(err).NotTo(HaveOccurred())
					defer func() {
						p.Close()
					}()

					listener.Queue.(*KafkaConsumer).AssignedPartition = false
					go listener.Start()

					// wait consumer start to consume message before send it
					for listener.Queue.(*KafkaConsumer).AssignedPartition == false {
						time.Sleep(10 * time.Millisecond)
					}

					topics := make(map[string]string)
					topics[game1] = "push-" + game1 + "-" + platform + "-feedbacks"
					topics[game2] = "push-" + game2 + "-" + platform + "-feedbacks"

					for _, game := range []string{game1, game2} {
						for _, msg := range feedbacks[game] {
							deviceToken := msg.DeviceToken

							_, err = db.Exec(fmt.Sprintf(`
							INSERT INTO %s_%s (id, user_id, token, region, locale, tz)
							VALUES (?0, ?1, ?2, ?3, ?4,?5)
							`, game, platform),
								uuid.NewV4(), uuid.NewV4(), deviceToken, "br", "PT", "-300")
							Expect(err).NotTo(HaveOccurred())
						}
					}

					msgs := make([]*kafka.Message, 0, len(feedbacks[game1])+len(feedbacks[game2]))
					for _, game := range []string{game1, game2} {
						topic := topics[game]
						for _, msg := range feedbacks[game] {
							value, err := json.Marshal(msg)
							Expect(err).NotTo(HaveOccurred())

							msgs = append(msgs, &kafka.Message{
								TopicPartition: kafka.TopicPartition{
									Topic:     &topic,
									Partition: kafka.PartitionAny,
								},
								Value: value,
							})
						}
					}

					for _, msg := range msgs {
						eventsChan := make(chan kafka.Event)

						err = p.Produce(msg, eventsChan)
						<-eventsChan
						Expect(err).NotTo(HaveOccurred())
					}

					for _, game := range []string{game1, game2} {
						for _, msg := range feedbacks[game] {
							deviceToken := msg.DeviceToken

							Eventually(func() int {
								res, err := db.Exec(fmt.Sprintf(`SELECT FROM %s_%s
								WHERE token = ?0`, game, platform), deviceToken)
								Expect(err).NotTo(HaveOccurred())
								return res.RowsReturned()
							}, 30*time.Second).Should(Equal(0))
						}
					}
					listener.Stop()
				})
			})
		})
	})
})
