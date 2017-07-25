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
	"os"
	"time"

	"github.com/RobotsAndPencils/buford/push"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	. "github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("APNS Message Handler", func() {
	var db interfaces.DB
	var feedbackClients []interfaces.FeedbackReporter
	var handler *APNSMessageHandler
	var invalidTokenHandlers []interfaces.InvalidTokenHandler
	var mockKafkaConsumerClient *mocks.KafkaConsumerClientMock
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var mockPushQueue *mocks.APNSPushQueueMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	configFile := "../config/test.yaml"
	config, _ := util.NewViperWithConfigFile(configFile)
	certificatePath := "../tls/self_signed_cert.pem"
	isProduction := false
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	Describe("[Unit]", func() {
		BeforeEach(func() {
			mockStatsDClient = mocks.NewStatsDClientMock()
			mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
			mockKafkaConsumerClient = mocks.NewKafkaConsumerClientMock()
			mockKafkaProducerClient.StartConsumingMessagesInProduceChannel()
			c, err := NewStatsD(config, logger, mockStatsDClient)
			Expect(err).NotTo(HaveOccurred())

			kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
			Expect(err).NotTo(HaveOccurred())

			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			db = mocks.NewPGMock(0, 1)
			it, err := NewTokenPG(config, logger, db)
			Expect(err).NotTo(HaveOccurred())
			invalidTokenHandlers = []interfaces.InvalidTokenHandler{it}

			mockPushQueue = mocks.NewAPNSPushQueueMock()
			handler, err = NewAPNSMessageHandler(
				certificatePath,
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

		Describe("Configuring Handler", func() {
			It("should fail if invalid pem file", func() {
				handler.CertificatePath = "./invalid-certficate.pem"
				err := handler.configure(nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("open ./invalid-certficate.pem: no such file or directory"))
			})
		})

		Describe("Configuring Certificate", func() {
			It("should configure from pem file", func() {
				err := handler.configureCertificate()
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.certificate).NotTo(BeNil())
				Expect(handler.Topic).To(Equal(""))
			})

			It("should fail if invalid pem file", func() {
				handler.CertificatePath = "./invalid-certficate.pem"
				err := handler.configureCertificate()
				Expect(err).To(HaveOccurred())
				Expect(handler.Topic).To(Equal(""))
			})
		})

		Describe("Configuring APNS Push Queue", func() {
			It("should configure APNS Push Queue", func() {
				err := handler.configureAPNSPushQueue()
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.PushQueue).NotTo(BeNil())
				Expect(handler.Topic).To(Equal(""))
			})

			XIt("should fail if invalid push queue - how to test this?", func() {
				err := handler.configureAPNSPushQueue()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("qwe"))
			})
		})

		Describe("Handle APNS response", func() {
			It("if reponse has nil error", func() {
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.successesReceived).To(Equal(int64(1)))
			})

			It("if reponse has error push.ErrMissingDeviceToken", func() {
				pushError := &push.Error{
					Reason: push.ErrMissingDeviceToken,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				Expect(hook.Entries).To(ContainLogMessage("deleting token"))
				//Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
			})

			It("if reponse has error push.ErrBadDeviceToken", func() {
				pushError := &push.Error{
					Reason: push.ErrBadDeviceToken,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				Expect(hook.Entries).To(ContainLogMessage("deleting token"))
				//Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
			})

			It("if reponse has error push.ErrBadCertificate", func() {
				pushError := &push.Error{
					Reason: push.ErrBadCertificate,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
			})

			It("if reponse has error push.ErrBadCertificateEnvironment", func() {
				pushError := &push.Error{
					Reason: push.ErrBadCertificateEnvironment,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
			})

			It("if reponse has error push.ErrForbidden", func() {
				pushError := &push.Error{
					Reason: push.ErrForbidden,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
			})

			It("if reponse has error push.ErrMissingTopic", func() {
				pushError := &push.Error{
					Reason: push.ErrMissingTopic,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
			})

			It("if reponse has error push.ErrTopicDisallowed", func() {
				pushError := &push.Error{
					Reason: push.ErrTopicDisallowed,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
			})

			It("if reponse has error push.ErrDeviceTokenNotForTopic", func() {
				pushError := &push.Error{
					Reason: push.ErrDeviceTokenNotForTopic,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
			})

			It("if reponse has error push.ErrIdleTimeout", func() {
				pushError := &push.Error{
					Reason: push.ErrIdleTimeout,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if reponse has error push.ErrShutdown", func() {
				pushError := &push.Error{
					Reason: push.ErrShutdown,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if reponse has error push.ErrInternalServerError", func() {
				pushError := &push.Error{
					Reason: push.ErrInternalServerError,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if reponse has error push.ErrServiceUnavailable", func() {
				pushError := &push.Error{
					Reason: push.ErrServiceUnavailable,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
			})

			It("if reponse has untracked error", func() {
				pushError := &push.Error{
					Reason: push.ErrMethodNotAllowed,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
				//Expect(hook.LastEntry().Data["category"]).To(Equal("DefaultError"))
			})
		})

		Describe("Send message", func() {
			It("should add message to push queue and increment sentMessages", func() {
				handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
				Expect(handler.sentMessages).To(Equal(int64(1)))
			})
		})

		Describe("Handle Messages", func() {
			It("should start without panicking and set run to true", func() {
				queue, err := NewKafkaConsumer(handler.Config, logger, mockKafkaConsumerClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(func() { go handler.HandleMessages(queue.MessagesChannel()) }).ShouldNot(Panic())
				time.Sleep(50 * time.Millisecond)
				Expect(handler.run).To(BeTrue())
			})
		})

		Describe("Clean Cache", func() {
			It("should remove from push queue after timeout", func() {
				handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				time.Sleep(500 * time.Millisecond)
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InflightMessagesMetadata).To(BeEmpty())
			})

			It("should not panic if a request got a response", func() {
				handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
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
						handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
					}
				}

				handleResponses := func() {
					for i := 0; i < n/2; i++ {
						res := push.Response{
							DeviceToken: uuid.NewV4().String(),
							ID:          uuid.NewV4().String(),
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

				handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
				handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))

				Expect(mockStatsDClient.Count["sent"]).To(Equal(2))
			})

			It("should call HandleNotificationSuccess upon message response received", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))

				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
				}
				handler.handleAPNSResponse(res)
				handler.handleAPNSResponse(res)

				Expect(mockStatsDClient.Count["ack"]).To(Equal(2))
			})

			It("should call HandleNotificationFailure upon message response received", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.StatsReporters).To(Equal(statsClients))

				pushError := &push.Error{
					Reason: push.ErrMissingDeviceToken,
				}
				res := push.Response{
					DeviceToken: uuid.NewV4().String(),
					ID:          uuid.NewV4().String(),
					Err:         pushError,
				}
				handler.handleAPNSResponse(res)
				handler.handleAPNSResponse(res)

				Expect(mockStatsDClient.Count["failed"]).To(Equal(2))
				Expect(mockStatsDClient.Count["missing-device-token"]).To(Equal(2))
			})
		})

		Describe("Feedback Reporter sent message", func() {
			BeforeEach(func() {
				mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()

				kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
				Expect(err).NotTo(HaveOccurred())

				feedbackClients = []interfaces.FeedbackReporter{kc}

				db = mocks.NewPGMock(0, 1)
				it, err := NewTokenPG(config, logger, db)
				Expect(err).NotTo(HaveOccurred())
				invalidTokenHandlers = []interfaces.InvalidTokenHandler{it}
				handler, err = NewAPNSMessageHandler(
					certificatePath,
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

			It("should include a timestamp in feedback root and the hostname and msgid in metadata", func() {
				timestampNow := time.Now().Unix()
				msgID := uuid.NewV4().String()
				hostname, err := os.Hostname()
				Expect(err).NotTo(HaveOccurred())
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": timestampNow,
					"hostname":  hostname,
					"msgid":     msgID,
				}
				handler.InflightMessagesMetadata["testToken1"] = metadata
				res := push.Response{
					DeviceToken: "testToken1",
					ID:          "idTest1",
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.Timestamp).To(Equal(timestampNow))
				Expect(fromKafka.Metadata["msgid"]).To(Equal(msgID))
				Expect(fromKafka.Metadata["hostname"]).To(Equal(hostname))
			})

			It("should send feedback if success and metadata is present", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
				}
				handler.InflightMessagesMetadata["testToken1"] = metadata
				res := push.Response{
					DeviceToken: "testToken1",
					ID:          "idTest1",
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.DeviceToken).To(Equal(res.DeviceToken))
				Expect(fromKafka.ID).To(Equal(res.ID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
			})

			It("should send feedback if success and metadata is not present", func() {
				res := push.Response{
					DeviceToken: "testToken1",
					ID:          "idTest1",
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.DeviceToken).To(Equal(res.DeviceToken))
				Expect(fromKafka.ID).To(Equal(res.ID))
				Expect(fromKafka.Metadata["some"]).To(BeNil())
			})

			It("should send feedback if error and metadata is present and token should be deleted", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
				}
				handler.InflightMessagesMetadata["testToken1"] = metadata
				res := push.Response{
					DeviceToken: "testToken1",
					ID:          "idTest1",
					Err: &push.Error{
						Reason: push.ErrBadDeviceToken,
					},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.DeviceToken).To(Equal(res.DeviceToken))
				Expect(fromKafka.ID).To(Equal(res.ID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
				Expect(fromKafka.Metadata["deleteToken"]).To(BeTrue())
				Expect(string(msg.Value)).To(ContainSubstring("bad device token"))
			})

			It("should send feedback if error and metadata is present and token should not be deleted", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
				}
				handler.InflightMessagesMetadata["testToken1"] = metadata
				res := push.Response{
					DeviceToken: "testToken1",
					ID:          "idTest1",
					Err: &push.Error{
						Reason: push.ErrBadMessageID,
					},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.DeviceToken).To(Equal(res.DeviceToken))
				Expect(fromKafka.ID).To(Equal(res.ID))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
				Expect(fromKafka.Metadata["deleteToken"]).To(BeNil())
				Expect(string(msg.Value)).To(ContainSubstring("ID header value is bad"))
			})

			It("should send feedback if error and metadata is not present", func() {
				res := push.Response{
					DeviceToken: "testToken1",
					ID:          "idTest1",
					Err: &push.Error{
						Reason: push.ErrBadDeviceToken,
					},
				}
				go handler.handleAPNSResponse(res)

				fromKafka := &ResponseWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.DeviceToken).To(Equal(res.DeviceToken))
				Expect(fromKafka.ID).To(Equal(res.ID))
				Expect(fromKafka.Metadata).To(BeNil())
				Expect(string(msg.Value)).To(ContainSubstring("bad device token"))
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
				certificatePath,
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
				handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
				Eventually(handler.PushQueue.(*push.Queue).Responses, 5*time.Second).Should(Receive())
				Expect(handler.sentMessages).To(Equal(int64(1)))
			})
		})

		Describe("Handle Responses", func() {
			It("should be called without panicking", func() {
				Expect(func() { go handler.HandleResponses() }).ShouldNot(Panic())
				handler.PushQueue.(*push.Queue).Responses <- push.Response{}
				time.Sleep(50 * time.Millisecond)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})
		})

	})
})
