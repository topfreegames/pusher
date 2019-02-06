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
	"encoding/json"
	"fmt"
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
	var invalidTokenHandlers []interfaces.InvalidTokenHandler
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var mockPushQueue *mocks.APNSPushQueueMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	configFile := "../config/test.yaml"
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
			it, err := NewTokenPG(config, logger, statsClients, db)
			Expect(err).NotTo(HaveOccurred())
			invalidTokenHandlers = []interfaces.InvalidTokenHandler{it}

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
				invalidTokenHandlers,
				mockPushQueue,
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
				Expect(handler.responsesReceived).To(Equal(int64(0)))
				Expect(handler.sentMessages).To(Equal(int64(0)))
			})
		})

		Describe("Handle APNS response", func() {
			It("if response has nil error", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     uuid.NewV4().String(),
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.successesReceived).To(Equal(int64(1)))
			})

			It("if response has error push.ReasonUnregistered", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonUnregistered,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(ContainLogMessage("deleting token"))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeleted] }).
					Should(Equal(1))
				//Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
			})

			It("if response has error push.ErrBadDeviceToken", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonBadDeviceToken,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(ContainLogMessage("deleting token"))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeleted] }).
					Should(Equal(1))
				//Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
			})

			It("if response has error push.ErrBadCertificate", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 403,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonBadCertificate,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
			})

			It("if response has error push.ErrBadCertificateEnvironment", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 403,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonBadCertificateEnvironment,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
			})

			It("if response has error push.ErrForbidden", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 403,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonForbidden,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
			})

			It("if response has error push.ErrMissingTopic", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonMissingTopic,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
			})

			It("if response has error push.ErrTopicDisallowed", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonTopicDisallowed,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
			})

			It("if response has error push.ErrDeviceTokenNotForTopic", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonDeviceTokenNotForTopic,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
			})

			It("if response has error push.ErrIdleTimeout", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 400,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonIdleTimeout,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if response has error push.ErrShutdown", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 503,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonShutdown,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if response has error push.ErrInternalServerError", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 500,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonInternalServerError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if response has error push.ErrServiceUnavailable", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 503,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonServiceUnavailable,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if response has untracked error", func() {
				res := &structs.ResponseWithMetadata{
					StatusCode: 405,
					ApnsID:     uuid.NewV4().String(),
					Reason:     apns2.ReasonMethodNotAllowed,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("DefaultError"))
			})
		})

		Describe("Send message", func() {
			It("should add message to push queue and increment sentMessages", func() {
				handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(handler.sentMessages).To(Equal(int64(1)))
			})
		})

		Describe("Clean Cache", func() {
			It("should remove from push queue after timeout", func() {
				handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				time.Sleep(500 * time.Millisecond)
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InflightMessagesMetadata).To(BeEmpty())
			})

			It("should not panic if a request got a response", func() {
				handler.sendMessage(interfaces.KafkaMessage{
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
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InflightMessagesMetadata).To(BeEmpty())
			})

			It("should handle all responses or remove them after timeout", func() {
				var n int = 10
				sendRequests := func() {
					for i := 0; i < n; i++ {
						handler.sendMessage(interfaces.KafkaMessage{
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

				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InflightMessagesMetadata).To(BeEmpty())
			})
		})

		Describe("Log Stats", func() {
			It("should log and zero stats", func() {
				handler.sentMessages = 100
				handler.responsesReceived = 90
				handler.successesReceived = 60
				handler.failuresReceived = 30
				Expect(func() { go handler.LogStats() }).ShouldNot(Panic())
				Eventually(func() []*logrus.Entry { return hook.Entries }).Should(ContainLogMessage("flushing stats"))
				Eventually(func() int64 { return handler.sentMessages }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.responsesReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.successesReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.failuresReceived }).Should(Equal(int64(0)))
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
				handler.sendMessage(kafkaMessage)
				handler.sendMessage(kafkaMessage)

				Expect(mockStatsDClient.Count["sent"]).To(Equal(2))
			})

			It("should call HandleNotificationSuccess upon message response received", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))

				res := &structs.ResponseWithMetadata{
					StatusCode: 200,
					ApnsID:     uuid.NewV4().String(),
				}

				handler.handleAPNSResponse(res)
				handler.handleAPNSResponse(res)
				Expect(mockStatsDClient.Count["ack"]).To(Equal(2))
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

				Expect(mockStatsDClient.Count["failed"]).To(Equal(2))
			})
		})

		Describe("Feedback Reporter sent message", func() {
			BeforeEach(func() {
				mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()

				kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
				Expect(err).NotTo(HaveOccurred())

				feedbackClients = []interfaces.FeedbackReporter{kc}

				db = mocks.NewPGMock(0, 1)
				it, err := NewTokenPG(config, logger, statsClients, db)
				Expect(err).NotTo(HaveOccurred())
				invalidTokenHandlers = []interfaces.InvalidTokenHandler{it}
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
					invalidTokenHandlers,
					mockPushQueue,
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
				handler.InflightMessagesMetadata["idTest1"] = metadata
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
				handler.InflightMessagesMetadata["idTest1"] = metadata
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
				handler.InflightMessagesMetadata["idTest1"] = metadata
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
				handler.InflightMessagesMetadata["idTest1"] = metadata
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
				err := handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(err).NotTo(HaveOccurred())
				Eventually(handler.PushQueue.ResponseChannel(), 5*time.Second).Should(Receive())
				Expect(handler.sentMessages).To(Equal(int64(1)))
			})
		})

		Describe("PushExpiry", func() {
			It("should not send message if PushExpiry is in the past", func() {
				handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(fmt.Sprintf(`{ "aps" : { "alert" : "Hello HTTP/2" }, "push_expiry": %d }`, makeTimestamp()-int64(100))),
				})
				Eventually(handler.PushQueue.ResponseChannel(), 5*time.Second).ShouldNot(Receive())
				Expect(handler.sentMessages).To(Equal(int64(0)))
				Expect(handler.ignoredMessages).To(Equal(int64(1)))
			})
			It("should send message if PushExpiry is in the future", func() {
				handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_apns",
					Value: []byte(fmt.Sprintf(`{ "aps" : { "alert" : "Hello HTTP/2" }, "push_expiry": %d}`, makeTimestamp()+int64(100))),
				})
				Eventually(handler.PushQueue.ResponseChannel(), 5*time.Second).ShouldNot(Receive())
				Expect(handler.sentMessages).To(Equal(int64(1)))
			})
		})

		Describe("Handle Responses", func() {
			It("should be called without panicking", func() {
				Expect(func() { go handler.HandleResponses() }).ShouldNot(Panic())
				handler.PushQueue.ResponseChannel() <- &structs.ResponseWithMetadata{}
				time.Sleep(50 * time.Millisecond)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})
		})

	})
})
