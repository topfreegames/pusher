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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	. "github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("GCM Message Handler", func() {
	var feedbackClients []interfaces.FeedbackReporter
	var handler *GCMMessageHandler
	var mockClient *mocks.GCMClientMock
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	config, _ := util.NewViperWithConfigFile(configFile)
	senderID := "sender-id"
	apiKey := "api-key"
	isProduction := false
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	Describe("[Unit]", func() {
		BeforeEach(func() {
			var err error

			mockStatsDClient = mocks.NewStatsDClientMock()
			mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
			mockKafkaProducerClient.StartConsumingMessagesInProduceChannel()
			c, err := NewStatsD(config, logger, mockStatsDClient)
			Expect(err).NotTo(HaveOccurred())

			kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
			Expect(err).NotTo(HaveOccurred())
			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			mockClient = mocks.NewGCMClientMock()
			handler, err = NewGCMMessageHandler(
				senderID,
				apiKey,
				isProduction,
				config,
				logger,
				nil,
				statsClients,
				feedbackClients,
				mockClient,
			)
			Expect(err).NotTo(HaveOccurred())

			hook.Reset()
		})

		Describe("Creating new handler", func() {
			It("should return configured handler", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.apiKey).To(Equal(apiKey))
				Expect(handler.Config).NotTo(BeNil())
				Expect(handler.IsProduction).To(Equal(isProduction))
				Expect(handler.senderID).To(Equal(senderID))
				Expect(handler.responsesReceived).To(Equal(int64(0)))
				Expect(handler.sentMessages).To(Equal(int64(0)))
				Expect(mockClient.MessagesSent).To(HaveLen(0))
			})
		})

		Describe("Configuring Handler", func() {
			It("should fail if invalid credentials", func() {
				handler.apiKey = "badkey"
				handler.senderID = "badsender"
				err := handler.configure(nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("error connecting gcm xmpp client: auth failure: not-authorized"))
			})
		})

		Describe("Handle GCM response", func() {
			It("if response has nil error", func() {
				res := gcm.CCSMessage{}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.successesReceived).To(Equal(int64(1)))
			})

			It("if response has error DEVICE_UNREGISTERED", func() {
				res := gcm.CCSMessage{
					Error: "DEVICE_UNREGISTERED",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has error BAD_REGISTRATION", func() {
				res := gcm.CCSMessage{
					Error: "BAD_REGISTRATION",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has error INVALID_JSON", func() {
				res := gcm.CCSMessage{
					Error: "INVALID_JSON",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has error SERVICE_UNAVAILABLE", func() {
				res := gcm.CCSMessage{
					Error: "SERVICE_UNAVAILABLE",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has error INTERNAL_SERVER_ERROR", func() {
				res := gcm.CCSMessage{
					Error: "INTERNAL_SERVER_ERROR",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has error DEVICE_MESSAGE_RATE_EXCEEDED", func() {
				res := gcm.CCSMessage{
					Error: "DEVICE_MESSAGE_RATE_EXCEEDED",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has error TOPICS_MESSAGE_RATE_EXCEEDED", func() {
				res := gcm.CCSMessage{
					Error: "TOPICS_MESSAGE_RATE_EXCEEDED",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})

			It("if response has untracked error", func() {
				res := gcm.CCSMessage{
					Error: "BAD_ACK",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(handler.failuresReceived).To(Equal(int64(1)))
			})
		})

		Describe("Test push expiration", func() {
			It("should send message if PushExpiry is in the future", func() {
				ttl := uint(0)
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "gcm",
				}
				msg := &KafkaGCMMessage{
					XMPPMessage: gcm.XMPPMessage{
						TimeToLive:               &ttl,
						DeliveryReceiptRequested: false,
						DryRun:                   true,
						To:                       uuid.NewV4().String(),
						Data:                     map[string]interface{}{},
					},
					Metadata:   metadata,
					PushExpiry: makeTimestamp() + int64(1000000),
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: msgBytes,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.sentMessages).To(Equal(int64(1)))
				Expect(handler.ignoredMessages).To(Equal(int64(0)))
				Expect(hook.LastEntry().Message).To(Equal("sent message"))
			})
			It("should not send message if PushExpiry is in the past", func() {
				ttl := uint(0)
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "gcm",
				}
				msg := &KafkaGCMMessage{
					XMPPMessage: gcm.XMPPMessage{
						TimeToLive:               &ttl,
						DeliveryReceiptRequested: false,
						DryRun:                   true,
						To:                       uuid.NewV4().String(),
						Data:                     map[string]interface{}{},
					},
					Metadata:   metadata,
					PushExpiry: makeTimestamp() - int64(100),
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: msgBytes,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.sentMessages).To(Equal(int64(0)))
				Expect(handler.ignoredMessages).To(Equal(int64(1)))
				Expect(hook.LastEntry().Message).To(ContainSubstring("ignoring push"))
			})

		})

		Describe("Send message", func() {
			It("should send xmpp message and not increment sentMessages if an error occurs", func() {
				err := handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: []byte("gogogo"),
				})
				Expect(err).To(HaveOccurred())
				Expect(handler.sentMessages).To(Equal(int64(0)))
				Expect(hook.Entries).To(ContainLogMessage("Error unmarshaling message."))
				Expect(mockClient.MessagesSent).To(HaveLen(0))
				Expect(len(handler.pendingMessages)).To(Equal(0))
			})

			It("should send xmpp message", func() {
				ttl := uint(0)
				msg := &gcm.XMPPMessage{
					TimeToLive:               &ttl,
					DeliveryReceiptRequested: false,
					DryRun:                   true,
					To:                       uuid.NewV4().String(),
					Data:                     map[string]interface{}{},
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: msgBytes,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.sentMessages).To(Equal(int64(1)))
				Expect(hook.LastEntry().Message).To(Equal("sent message"))
				Expect(mockClient.MessagesSent).To(HaveLen(1))
				Expect(len(handler.pendingMessages)).To(Equal(1))
			})

			It("should send xmpp message with metadata", func() {
				ttl := uint(0)
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "gcm",
				}
				msg := &KafkaGCMMessage{
					XMPPMessage: gcm.XMPPMessage{
						TimeToLive:               &ttl,
						DeliveryReceiptRequested: false,
						DryRun:                   true,
						To:                       uuid.NewV4().String(),
						Data:                     map[string]interface{}{},
					},
					Metadata:   metadata,
					PushExpiry: makeTimestamp() + int64(1000000),
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: msgBytes,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.sentMessages).To(Equal(int64(1)))
				Expect(hook.LastEntry().Message).To(Equal("sent message"))
				Expect(mockClient.MessagesSent).To(HaveLen(1))
				Expect(len(handler.pendingMessages)).To(Equal(1))
			})

			It("should wait to send message if maxPendingMessages limit is reached", func() {
				ttl := uint(0)
				msg := &gcm.XMPPMessage{
					TimeToLive:               &ttl,
					DeliveryReceiptRequested: false,
					DryRun:                   true,
					To:                       uuid.NewV4().String(),
					Data:                     map[string]interface{}{},
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				for i := 1; i <= 3; i++ {
					err = handler.sendMessage(interfaces.KafkaMessage{
						Topic: "push-game_gcm",
						Value: msgBytes,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(handler.sentMessages).To(Equal(int64(i)))
					Expect(len(handler.pendingMessages)).To(Equal(i))
				}

				go handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: msgBytes,
				})
				Consistently(handler.sentMessages).Should(Equal(int64(3)))
				Consistently(len(handler.pendingMessages)).Should(Equal(3))

				time.Sleep(100 * time.Millisecond)
				<-handler.pendingMessages
				Eventually(func() int64 { return handler.sentMessages }).Should(Equal(int64(4)))
			})
		})

		Describe("Clean Cache", func() {
			It("should remove from push queue after timeout", func() {
				handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				time.Sleep(500 * time.Millisecond)
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InflightMessagesMetadata).To(BeEmpty())
			})

			It("should not panic if a request got a response", func() {
				handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				Expect(func() { go handler.CleanMetadataCache() }).ShouldNot(Panic())
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "ack",
					Category:    "testCategory",
				}

				handler.handleGCMResponse(res)
				time.Sleep(500 * time.Millisecond)
				Expect(*handler.requestsHeap).To(BeEmpty())
				Expect(handler.InflightMessagesMetadata).To(BeEmpty())
			})

			It("should handle all responses or remove them after timeout", func() {
				var n int = 10
				sendRequests := func() {
					for i := 0; i < n; i++ {
						handler.sendMessage(interfaces.KafkaMessage{
							Topic: "push-game_gcm",
							Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
						})
					}
				}

				handleResponses := func() {
					for i := 0; i < n/2; i++ {
						res := gcm.CCSMessage{
							From:        "testToken1",
							MessageID:   "idTest1",
							MessageType: "ack",
							Category:    "testCategory",
						}

						handler.handleGCMResponse(res)
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
				handler.ignoredMessages = 10
				Expect(func() { go handler.LogStats() }).ShouldNot(Panic())
				Eventually(func() []*logrus.Entry { return hook.Entries }).Should(ContainLogMessage("flushing stats"))
				Eventually(func() int64 { return handler.sentMessages }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.responsesReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.successesReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.failuresReceived }).Should(Equal(int64(0)))
				Eventually(func() int64 { return handler.ignoredMessages }).Should(Equal(int64(0)))
			})
		})

		Describe("Stats Reporter sent message", func() {
			It("should call HandleNotificationSent upon message sent to queue", func() {
				ttl := uint(0)
				msg := &gcm.XMPPMessage{
					TimeToLive:               &ttl,
					DeliveryReceiptRequested: false,
					DryRun:                   true,
					To:                       uuid.NewV4().String(),
					Data:                     map[string]interface{}{},
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())
				kafkaMessage := interfaces.KafkaMessage{
					Game:  "game",
					Topic: "push-game_gcm",
					Value: msgBytes,
				}
				err = handler.sendMessage(kafkaMessage)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(kafkaMessage)
				Expect(err).NotTo(HaveOccurred())
				Expect(mockStatsDClient.Counts["sent"]).To(Equal(int64(2)))
			})

			It("should call HandleNotificationSuccess upon message response received", func() {
				res := gcm.CCSMessage{}
				handler.handleGCMResponse(res)
				handler.handleGCMResponse(res)
				Expect(mockStatsDClient.Counts["ack"]).To(Equal(int64(2)))
			})

			It("should call HandleNotificationFailure upon message response received", func() {
				res := gcm.CCSMessage{
					Error: "DEVICE_UNREGISTERED",
				}
				handler.handleGCMResponse(res)
				handler.handleGCMResponse(res)

				Expect(mockStatsDClient.Counts["failed"]).To(Equal(int64(2)))
			})
		})

		Describe("Feedback Reporter sent message", func() {
			BeforeEach(func() {
				var err error

				mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
				kc, err := NewKafkaProducer(config, logger, mockKafkaProducerClient)
				Expect(err).NotTo(HaveOccurred())
				feedbackClients = []interfaces.FeedbackReporter{kc}

				mockClient = mocks.NewGCMClientMock()

				handler, err = NewGCMMessageHandler(
					senderID,
					apiKey,
					isProduction,
					config,
					logger,
					nil,
					statsClients,
					feedbackClients,
					mockClient,
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
					"platform":  "gcm",
				}
				handler.InflightMessagesMetadata["idTest1"] = metadata
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "ack",
					Category:    "testCategory",
				}
				go handler.handleGCMResponse(res)

				fromKafka := &CCSMessageWithMetadata{}
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
					"platform":  "gcm",
				}
				handler.InflightMessagesMetadata["idTest1"] = metadata
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "ack",
					Category:    "testCategory",
				}
				go handler.handleGCMResponse(res)

				fromKafka := &CCSMessageWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.From).To(Equal(res.From))
				Expect(fromKafka.MessageID).To(Equal(res.MessageID))
				Expect(fromKafka.MessageType).To(Equal(res.MessageType))
				Expect(fromKafka.Category).To(Equal(res.Category))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
			})

			It("should send feedback if success and metadata is not present", func() {
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "ack",
					Category:    "testCategory",
				}
				go handler.handleGCMResponse(res)

				fromKafka := &CCSMessageWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.From).To(Equal(res.From))
				Expect(fromKafka.MessageID).To(Equal(res.MessageID))
				Expect(fromKafka.MessageType).To(Equal(res.MessageType))
				Expect(fromKafka.Category).To(Equal(res.Category))
				Expect(fromKafka.Metadata).To(BeNil())
			})

			It("should send feedback if error and metadata is present and token should be deleted", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "gcm",
				}
				handler.InflightMessagesMetadata["idTest1"] = metadata
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "nack",
					Category:    "testCategory",
					Error:       "BAD_REGISTRATION",
				}
				go handler.handleGCMResponse(res)

				fromKafka := &CCSMessageWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.From).To(Equal(res.From))
				Expect(fromKafka.MessageID).To(Equal(res.MessageID))
				Expect(fromKafka.MessageType).To(Equal(res.MessageType))
				Expect(fromKafka.Category).To(Equal(res.Category))
				Expect(fromKafka.Error).To(Equal(res.Error))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
				Expect(fromKafka.Metadata["deleteToken"]).To(BeTrue())
			})

			It("should send feedback if error and metadata is present and token should not be deleted", func() {
				metadata := map[string]interface{}{
					"some":      "metadata",
					"timestamp": time.Now().Unix(),
					"game":      "game",
					"platform":  "gcm",
				}
				handler.InflightMessagesMetadata["idTest1"] = metadata
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "nack",
					Category:    "testCategory",
					Error:       "INVALID_JSON",
				}
				go handler.handleGCMResponse(res)

				fromKafka := &CCSMessageWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.From).To(Equal(res.From))
				Expect(fromKafka.MessageID).To(Equal(res.MessageID))
				Expect(fromKafka.MessageType).To(Equal(res.MessageType))
				Expect(fromKafka.Category).To(Equal(res.Category))
				Expect(fromKafka.Error).To(Equal(res.Error))
				Expect(fromKafka.Metadata["some"]).To(Equal(metadata["some"]))
				Expect(fromKafka.Metadata["deleteToken"]).To(BeNil())
			})

			It("should send feedback if error and metadata is not present", func() {
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "nack",
					Category:    "testCategory",
					Error:       "BAD_REGISTRATION",
				}
				go handler.handleGCMResponse(res)

				fromKafka := &CCSMessageWithMetadata{}
				msg := <-mockKafkaProducerClient.ProduceChannel()
				json.Unmarshal(msg.Value, fromKafka)
				Expect(fromKafka.From).To(Equal(res.From))
				Expect(fromKafka.MessageID).To(Equal(res.MessageID))
				Expect(fromKafka.MessageType).To(Equal(res.MessageType))
				Expect(fromKafka.Category).To(Equal(res.Category))
				Expect(fromKafka.Error).To(Equal(res.Error))
				Expect(fromKafka.Metadata).To(BeNil())
			})
		})

		Describe("Cleanup", func() {
			It("should close GCMClient without error", func() {
				err := handler.Cleanup()
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.GCMClient.(*mocks.GCMClientMock).Closed).To(BeTrue())
			})
		})
	})

	Describe("[Integration]", func() {
		BeforeEach(func() {
			var err error

			c, err := NewStatsD(config, logger)
			Expect(err).NotTo(HaveOccurred())

			kc, err := NewKafkaProducer(config, logger)
			Expect(err).NotTo(HaveOccurred())
			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			handler, err = NewGCMMessageHandler(
				senderID,
				apiKey,
				isProduction,
				config,
				logger,
				nil,
				statsClients,
				feedbackClients,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			hook.Reset()
		})

		PDescribe("Creating new handler", func() {
			It("should fail when real client", func() {
				var err error
				handler, err = NewGCMMessageHandler(
					senderID,
					apiKey,
					isProduction,
					config,
					logger,
					nil,
					statsClients,
					feedbackClients,
					nil,
				)
				Expect(handler).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error connecting gcm xmpp client: auth failure: not-authorized"))
			})
		})
	})
})
