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
	mock_interfaces "github.com/topfreegames/pusher/mocks/interfaces"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/structs"
	. "github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = FDescribe("APNS Message Handler", func() {
	var db interfaces.DB
	var feedbackClients []interfaces.FeedbackReporter
	var handler *APNSMessageHandler
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var mockPushQueue *mocks.APNSPushQueueMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter
	mockConsumptionManager := mock_interfaces.NewMockConsumptionManager()
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
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     apnsID,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.successesReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ReasonUnregistered", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonUnregistered,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrBadDeviceToken", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonBadDeviceToken,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrBadCertificate", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 403,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonBadCertificate,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrBadCertificateEnvironment", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 403,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonBadCertificateEnvironment,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrForbidden", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 403,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonForbidden,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrMissingTopic", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonMissingTopic,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrTopicDisallowed", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonTopicDisallowed,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrDeviceTokenNotForTopic", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonDeviceTokenNotForTopic,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrIdleTimeout", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonIdleTimeout,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrShutdown", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 503,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonShutdown,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrInternalServerError", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 500,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonInternalServerError,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has error push.ErrServiceUnavailable", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 503,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonServiceUnavailable,
				}
				handler.handleAPNSResponse(res)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})

			It("if response has untracked error", func() {
				apnsID := uuid.NewV4().String()
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 405,
					ApnsID:     apnsID,
					Reason:     apns2.ReasonMethodNotAllowed,
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
				n, err := handler.buildNotification(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "some": "data" } }`),
				})
				Expect(err).To(BeNil())

				err = handler.sendNotification(n)
				Expect(err).To(BeNil())
			})

			It("should have metadata on message sent to push queue", func() {
				n, err := handler.buildNotification(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "some": "data" } }`),
				})
				Expect(err).To(BeNil())

				err = handler.sendNotification(n)
				Expect(err).To(BeNil())

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
				Expect(metadata["some"]).To(Equal("data"))
			})

			It("should merge metadata on message sent to push queue", func() {
				n, err := handler.buildNotification(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" }, "M": { "metadata": "received" } }, "Metadata": { "some": "data" } }`),
				})
				Expect(err).To(BeNil())

				err = handler.sendNotification(n)
				Expect(err).To(BeNil())

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
				n, err := handler.buildNotification(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" }, "M": { "metadata": "received" } }, "Metadata": { "nested": { "some": "data"} } }`),
				})
				Expect(err).To(BeNil())

				err = handler.sendNotification(n)
				Expect(err).To(BeNil())

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

		Describe("Clean Cache", func() {
			It("should remove from push queue after timeout", func() {
				handler.HandleMessages(ctx, interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				time.Sleep(500 * time.Millisecond)
				handler.inFlightNotificationsMapLock.Lock()
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InFlightNotificationsMap).To(BeEmpty())
				handler.inFlightNotificationsMapLock.Unlock()
			})

			It("should not panic if a request got a response", func() {
				handler.HandleMessages(ctx, interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     uuid.NewV4().String(),
				}

				handler.handleAPNSResponse(res)
				time.Sleep(500 * time.Millisecond)
				handler.inFlightNotificationsMapLock.Lock()
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InFlightNotificationsMap).To(BeEmpty())
				handler.inFlightNotificationsMapLock.Unlock()
			})

			It("should handle all responses or remove them after timeout", func() {
				var n int = 10
				sendRequests := func() {
					for i := 0; i < n; i++ {
						handler.HandleMessages(ctx, interfaces.KafkaMessage{
							Topic: "push-game_apns",
							Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
						})
					}
				}

				handleResponses := func() {
					for i := 0; i < n/2; i++ {
						res := &structs.ResponseWithMetadata{
							StatusCode: 200,
							ApnsID:     uuid.NewV4().String(),
						}

						handler.handleAPNSResponse(res)
					}
				}

				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				Expect(func() { go sendRequests() }).ShouldNot(Panic())
				time.Sleep(10 * time.Millisecond)
				Expect(func() { go handleResponses() }).ShouldNot(Panic())
				time.Sleep(500 * time.Millisecond)

				handler.inFlightNotificationsMapLock.Lock()
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InFlightNotificationsMap).To(BeEmpty())
				handler.inFlightNotificationsMapLock.Unlock()
			})
		})

		Describe("Log Stats", func() {
			It("should log and zero stats", func() {
				handler.sentMessages = 100
				handler.responsesReceived = 90
				handler.successesReceived = 60
				handler.failuresReceived = 30
				Expect(func() { go handler.LogStats() }).ShouldNot(Panic())
				Eventually(func() []logrus.Entry { return hook.Entries }).Should(ContainLogMessage("flushing stats"))

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
				handler.InFlightNotificationsMap[apnsID] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     apnsID,
				}

				handler.handleAPNSResponse(res)
				handler.handleAPNSResponse(res)
				Expect(mockStatsDClient.Counts["ack"]).To(Equal(int64(2)))
			})

			It("should call HandleNotificationFailure upon message response received", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))

				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonMissingDeviceToken,
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
				handler.inFlightNotificationsMapLock.Lock()
				handler.InFlightNotificationsMap["idTest1"] = &inFlightNotification{notification: &Notification{Metadata: metadata}}
				handler.inFlightNotificationsMapLock.Unlock()
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     "idTest1",
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
				handler.inFlightNotificationsMapLock.Lock()
				handler.InFlightNotificationsMap["idTest1"] = &inFlightNotification{notification: &Notification{Metadata: metadata}}
				handler.inFlightNotificationsMapLock.Unlock()
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     "idTest1",
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &structs.ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.ApnsID).To(Equal(res.ApnsID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
			})

			It("should send feedback if success and metadata is not present", func() {
				handler.inFlightNotificationsMapLock.Lock()
				handler.InFlightNotificationsMap["idTest1"] = &inFlightNotification{
					notification: &Notification{
						Metadata: map[string]interface{}{
							"timestamp": int64(0),
						},
					},
				}
				handler.inFlightNotificationsMapLock.Unlock()
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     "idTest1",
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
				handler.inFlightNotificationsMapLock.Lock()
				handler.InFlightNotificationsMap["idTest1"] = &inFlightNotification{notification: &Notification{Metadata: metadata}}
				handler.inFlightNotificationsMapLock.Unlock()

				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     "idTest1",
					Reason:     apns2.ReasonBadDeviceToken,
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

				handler.inFlightNotificationsMapLock.Lock()
				handler.InFlightNotificationsMap["idTest1"] = &inFlightNotification{notification: &Notification{Metadata: metadata}}
				handler.inFlightNotificationsMapLock.Unlock()

				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     "idTest1",
					Reason:     apns2.ReasonBadMessageID,
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
					DeviceToken: uuid.NewV4().String(),
					StatusCode:  400,
					ApnsID:      "idTest1",
					Reason:      apns2.ReasonBadDeviceToken,
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
				handler.inFlightNotificationsMapLock.Lock()
				handler.InFlightNotificationsMap["idTest1"] = &inFlightNotification{notification: &Notification{Metadata: metadata}}
				handler.InFlightNotificationsMap["idTest2"] = &inFlightNotification{notification: &Notification{Metadata: metadata}}
				handler.inFlightNotificationsMapLock.Unlock()

				res := &structs.ResponseWithMetadata{
					StatusCode: 429,
					ApnsID:     "idTest1",
					Reason:     apns2.ReasonTooManyRequests,
				}

				res2 := &structs.ResponseWithMetadata{
					StatusCode: 429,
					ApnsID:     "idTest2",
					Reason:     apns2.ReasonTooManyRequests,
				}
				go func() {
					err := handler.handleAPNSResponse(res)
					Expect(err).NotTo(HaveOccurred())
				}()
				time.Sleep(200 * time.Millisecond)

				go func() {
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
		})

		Describe("PushExpiry", func() {
			It("should not send message if PushExpiry is in the past", func() {
				handler.HandleMessages(ctx, interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(fmt.Sprintf(`{ "aps" : { "alert" : "Hello HTTP/2" }, "push_expiry": %d }`, MakeTimestamp()-int64(100))),
				})
				Eventually(handler.PushQueue.ResponseChannel(), 5*time.Second).ShouldNot(Receive())
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
				Eventually(handler.PushQueue.ResponseChannel(), 5*time.Second).ShouldNot(Receive())

				apnsResMutex.Lock()
				Expect(handler.sentMessages).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})
		})

		Describe("Handle Responses", func() {
			It("should be called without panicking", func() {
				Expect(func() { go handler.HandleResponses() }).ShouldNot(Panic())
				handler.PushQueue.ResponseChannel() <- &structs.ResponseWithMetadata{}
				time.Sleep(50 * time.Millisecond)
				apnsResMutex.Lock()
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				apnsResMutex.Unlock()
			})
		})

	})
})
