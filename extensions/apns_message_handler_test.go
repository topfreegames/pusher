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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	mock_interfaces "github.com/topfreegames/pusher/mocks/interfaces"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/structs"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("APNS Message Handler", func() {
	var db interfaces.DB
	var feedbackClients []interfaces.FeedbackReporter
	var handler *APNSMessageHandler
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var mockPushQueue *mocks.APNSPushQueueMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter
	mockConsumptionManager := mock_interfaces.NewMockConsumptionManager()
	mockRateLimiter := mocks.NewRateLimiterMock()
	ctx := context.Background()

	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	config, _ := util.NewViperWithConfigFile(configFile)
	authKeyPath := "../tls/authkey.p8"
	keyID := "ABC123DEFG"
	teamID := "DEF123GHIJ"
	topic := "com.game.test"
	appName := "game"
	isProduction := false
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	Describe("[Unit]", func() {
		BeforeEach(func() {
			mockStatsDClient = mocks.NewStatsDClientMock()
			mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
			mockKafkaProducerClient.StartConsumingMessagesInProduceChannel()
			c, err := NewStatsD(config, logger, mockStatsDClient)
			Expect(err).NotTo(HaveOccurred())

			kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
			Expect(err).NotTo(HaveOccurred())

			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			db = mocks.NewPGMock(0, 1)

			mockPushQueue = mocks.NewAPNSPushQueueMock()
			handler, err = NewAPNSMessageHandler(
				authKeyPath,
				keyID,
				teamID,
				topic,
				appName,
				isProduction,
				config,
				logger,
				nil,
				statsClients,
				feedbackClients,
				mockPushQueue,
				mockConsumptionManager,
				mockRateLimiter,
			)
			Expect(err).NotTo(HaveOccurred())
			db.(*mocks.PGMock).RowsReturned = 0

			hook.Reset()
		})

		Describe("Creating new handler", func() {
			It("should return configured handler", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.Config).NotTo(BeNil())
				Expect(handler.IsProduction).To(Equal(isProduction))
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(0)))
				Expect(handler.sentMessages).To(Equal(int64(0)))
				apnsResMutex.Unlock()
			})
		})

		Describe("Handle APNS response", func() {
			It("if response has nil error", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     apnsID,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.successesReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ReasonUnregistered", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonUnregistered,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrBadDeviceToken", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonBadDeviceToken,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrBadCertificate", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonBadCertificate,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrBadCertificateEnvironment", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonBadCertificateEnvironment,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrForbidden", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonForbidden,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrMissingTopic", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonMissingTopic,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrTopicDisallowed", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonTopicDisallowed,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrDeviceTokenNotForTopic", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonDeviceTokenNotForTopic,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrIdleTimeout", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonIdleTimeout,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrShutdown", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonShutdown,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrInternalServerError", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonInternalServerError,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrServiceUnavailable", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonServiceUnavailable,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has untracked error", func() {
				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonMethodNotAllowed,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: apnsID,
						},
					},
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})
		})

		Describe("Send notification", func() {
			It("should add message to push queue", func() {
				m, err := handler.parseKafkaMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "some": "data" } }`),
				})
				Expect(err).To(BeNil())

				n, err := handler.buildAndValidateNotification(m)

				handler.sendNotification(n)
				res := mockPushQueue.PushedNotification
				Expect(res).NotTo(BeNil())
			})

			It("should have metadata on message sent to push queue", func() {
				m, err := handler.parseKafkaMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "some": "data" } }`),
				})
				Expect(err).To(BeNil())

				n, err := handler.buildAndValidateNotification(m)
				Expect(err).To(BeNil())

				handler.sendNotification(n)

				sentMessage := mockPushQueue.PushedNotification
				bytes, ok := sentMessage.Notification.Payload.([]byte)
				Expect(ok).To(BeTrue())
				var payload map[string]interface{}
				err = json.Unmarshal(bytes, &payload)
				Expect(err).To(BeNil())
				Expect(payload).NotTo(BeNil())
				Expect(payload["M"]).NotTo(BeNil())

				metadata, ok := payload["M"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(metadata).NotTo(BeNil())
				Expect(metadata["some"]).To(Equal("data"))
			})

			It("should merge metadata on message sent to push queue", func() {
				m, err := handler.parseKafkaMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" }, "M": { "metadata": "received" } }, "Metadata": { "some": "data" } }`),
				})
				Expect(err).To(BeNil())

				n, err := handler.buildAndValidateNotification(m)
				Expect(err).To(BeNil())

				handler.sendNotification(n)

				sentMessage := mockPushQueue.PushedNotification
				bytes, ok := sentMessage.Payload.([]byte)
				Expect(ok).To(BeTrue())
				var payload map[string]interface{}
				json.Unmarshal(bytes, &payload)

				Expect(payload).NotTo(BeNil())
				Expect(payload["M"]).NotTo(BeNil())

				metadata, ok := payload["M"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(metadata).NotTo(BeNil())
				Expect(metadata["metadata"]).To(Equal("received"))
				Expect(metadata["some"]).To(Equal("data"))
			})

			It("should have nested metadata on message sent to push queue", func() {
				m, err := handler.parseKafkaMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" }, "M": { "metadata": "received" } }, "Metadata": { "nested": { "some": "data"} } }`),
				})
				Expect(err).To(BeNil())

				n, err := handler.buildAndValidateNotification(m)
				Expect(err).To(BeNil())

				handler.sendNotification(n)

				sentMessage := mockPushQueue.PushedNotification
				bytes, ok := sentMessage.Payload.([]byte)
				Expect(ok).To(BeTrue())
				var payload map[string]interface{}
				json.Unmarshal(bytes, &payload)

				Expect(payload).NotTo(BeNil())
				Expect(payload["M"]).NotTo(BeNil())

				metadata, ok := payload["M"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(metadata).NotTo(BeNil())
				Expect(metadata["metadata"]).To(Equal("received"))
				nestedMetadata, ok := metadata["nested"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(nestedMetadata["some"]).To(Equal("data"))
			})
		})

		Describe("Log Stats", func() {
			It("should log and zero stats", func() {
				handler.sentMessages = 100
				handler.responsesReceived = 90
				handler.successesReceived = 60
				handler.failuresReceived = 30
				Expect(func() { go handler.LogStats() }).ShouldNot(Panic())
				time.Sleep(2 * handler.LogStatsInterval)

				apnsResMutex.Lock()
				Eventually(func() int64 { return handler.sentMessages }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.responsesReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.successesReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.failuresReceived }).Should(Equal(int64(0)))
				apnsResMutex.Unlock()
			})
		})

		Describe("Stats Reporter sent message", func() {
			It("should call HandleNotificationSent upon message sent to queue", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))
				kafkaMessage := interfaces.KafkaMessage{
					Game:  "game",
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				}
				handler.HandleMessages(ctx, kafkaMessage)
				handler.HandleMessages(ctx, kafkaMessage)

				Expect(mockStatsDClient.Counts["sent"]).To(Equal(int64(2)))
			})

			It("should call HandleNotificationSuccess upon message response received", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))

				apnsID := uuid.NewV4().String()
				res := &structs.ResponseWithMetadata{
					StatusCode:   200,
					ApnsID:       apnsID,
					Notification: &structs.ApnsNotification{Notification: apns2.Notification{ApnsID: apnsID}},
				}

				handler.handleAPNSResponse(res)
				handler.handleAPNSResponse(res)
				Expect(mockStatsDClient.Counts["ack"]).To(Equal(int64(2)))
			})

			It("should call HandleNotificationFailure upon message response received", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))

				res := &structs.ResponseWithMetadata{
					StatusCode:   400,
					ApnsID:       uuid.NewV4().String(),
					Reason:       apns2.ReasonMissingDeviceToken,
					Notification: &structs.ApnsNotification{},
				}
				handler.handleAPNSResponse(res)
				handler.handleAPNSResponse(res)

				Expect(mockStatsDClient.Counts["failed"]).To(Equal(int64(2)))
			})
		})

		Describe("Feedback Reporter sent message", func() {
			BeforeEach(func() {
				mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
				kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
				Expect(err).NotTo(HaveOccurred())

				feedbackClients = []interfaces.FeedbackReporter{kc}

				db = mocks.NewPGMock(0, 1)

				handler, err = NewAPNSMessageHandler(
					authKeyPath,
					keyID,
					teamID,
					topic,
					appName,
					isProduction,
					config,
					logger,
					nil,
					statsClients,
					feedbackClients,
					mockPushQueue,
					mockConsumptionManager,
					mockRateLimiter,
				)
				Expect(err).NotTo(HaveOccurred())
			})
			It("should include a timestamp in feedback root and the hostname in metadata", func() {
				timestampNow := time.Now().Unix()
				hostname, err := os.Hostname()
				Expect(err).NotTo(HaveOccurred())
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": timestampNow,
					"hostname":  hostname,
					"game":      "game",
					"platform":  "apns",
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     "idTest1",
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest1",
						},
						Metadata: metadata,
					},
				}

				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.Timestamp).To(Equal(timestampNow))
				Expect(fromKafka.Metadata["hostname"]).To(Equal(hostname))
			})

			It("should send feedback if success and metadata is present", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "apns",
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     "idTest1",
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest1",
						},
						Metadata: metadata,
					},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.ApnsID).To(Equal(res.ApnsID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
			})

			It("should send feedback if success and metadata is not present", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     "idTest1",
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest1",
						},
					},
					Metadata: map[string]interface{}{
						"timestamp": int64(0),
					},
				}

				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.ApnsID).To(Equal(res.ApnsID))
				Expect(fromKafka.Metadata["some"]).To(BeNil())
			})

			It("should send feedback if error and metadata is present and token should be deleted", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "apns",
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     "idTest1",
					Reason:     apns2.ReasonBadDeviceToken,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest1",
						},
						Metadata: metadata,
					},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.ApnsID).To(Equal(res.ApnsID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
				Expect(fromKafka.Metadata["deleteToken"]).To(BeTrue())
				Expect(string(msg.Value)).To(ContainSubstring("BadDeviceToken"))
			})

			It("should send feedback if error and metadata is present and token should not be deleted", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "apns",
				}

				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     "idTest1",
					Reason:     apns2.ReasonBadMessageID,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest1",
						},
						Metadata: metadata,
					},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.ApnsID).To(Equal(res.ApnsID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
				Expect(fromKafka.Metadata["deleteToken"]).To(BeNil())
				Expect(string(msg.Value)).To(ContainSubstring("BadMessageId"))
			})

			It("should send feedback if error and metadata is not present", func() {
				res := &structs.ResponseWithMetadata{
					DeviceToken:  uuid.NewV4().String(),
					StatusCode:   400,
					ApnsID:       "idTest1",
					Reason:       apns2.ReasonBadDeviceToken,
					Notification: &structs.ApnsNotification{},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.ApnsID).To(Equal(res.ApnsID))
				Expect(fromKafka.Metadata).To(BeNil())
				Expect(string(msg.Value)).To(ContainSubstring("BadDeviceToken"))
			})

			It("should not deadlock on handle retry for handle apns response", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "apns",
				}

				res := &structs.ResponseWithMetadata{
					StatusCode: 429,
					ApnsID:     "idTest1",
					Reason:     apns2.ReasonTooManyRequests,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest1",
						},
					},
					Metadata: metadata,
				}

				res2 := &structs.ResponseWithMetadata{
					StatusCode: 429,
					ApnsID:     "idTest2",
					Reason:     apns2.ReasonTooManyRequests,
					Notification: &structs.ApnsNotification{
						Notification: apns2.Notification{
							ApnsID: "idTest2",
						},
					},
					Metadata: metadata,
				}
				go func() {
					defer GinkgoRecover()
					err := handler.handleAPNSResponse(res)
					Expect(err).NotTo(HaveOccurred())
				}()
				go func() {
					defer GinkgoRecover()
					err := handler.handleAPNSResponse(res2)
					Expect(err).NotTo(HaveOccurred())
				}()
			})
		})

		Describe("Cleanup", func() {
			It("should close PushQueue without error", func() {
				err := handler.Cleanup()
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.PushQueue.(*mocks.APNSPushQueueMock).Closed).To(BeTrue())
			})
		})
	})
	Describe("[Integration]", func() {
		BeforeEach(func() {
			var err error
			handler, err = NewAPNSMessageHandler(
				authKeyPath,
				keyID,
				teamID,
				topic,
				appName,
				isProduction,
				config,
				logger,
				nil,
				nil,
				nil,
				nil,
				nil,
				mockRateLimiter,
			)
			Expect(err).NotTo(HaveOccurred())
			hook.Reset()
		})

		Describe("Send message", func() {
			It("should add message to push queue and increment sentMessages", func() {
				handler.HandleMessages(ctx, interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})

				apnsResMutex.Lock()
				Expect(handler.ignoredMessages).To(Equal(int64(0)))
				Eventually(handler.PushQueue.ResponseChannel(), 5*time.Second).Should(Receive())
				Expect(handler.sentMessages).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("should be able to call HandleMessages concurrently with no errors", func() {
				msg := interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello" } }`),
				}

				go handler.HandleMessages(context.Background(), msg)
				go handler.HandleMessages(context.Background(), msg)
				go handler.HandleMessages(context.Background(), msg)
			})
		})

		Describe("PushExpiry", func() {
			It("should not send message if PushExpiry is in the past", func() {
				handler.HandleMessages(ctx, interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(fmt.Sprintf(`{ "aps" : { "alert" : "Hello HTTP/2" }, "push_expiry": %d }`, MakeTimestamp()-int64(100))),
				})
				Eventually(handler.PushQueue.ResponseChannel(), 100*time.Millisecond).ShouldNot(Receive())
				apnsResMutex.Lock()
				Expect(handler.sentMessages).To(Equal(int64(0)))
				Expect(handler.ignoredMessages).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})
			It("should send message if PushExpiry is in the future", func() {
				handler.HandleMessages(ctx, interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(fmt.Sprintf(`{ "aps" : { "alert" : "Hello HTTP/2" }, "push_expiry": %d}`, MakeTimestamp()+int64(100))),
				})
				Eventually(handler.PushQueue.ResponseChannel(), 100*time.Millisecond).ShouldNot(Receive())

				apnsResMutex.Lock()
				Expect(handler.sentMessages).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})
		})

		Describe("Handle Responses", func() {
			It("should be called without panicking", func() {
				Expect(func() { go handler.HandleResponses() }).ShouldNot(Panic())
				handler.PushQueue.ResponseChannel() <- &structs.ResponseWithMetadata{
					Notification: &structs.ApnsNotification{},
				}
				time.Sleep(50 * time.Millisecond)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})
		})

	})
})
