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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rounds/go-gcm"
	uuid "github.com/satori/go.uuid"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	. "github.com/topfreegames/pusher/testing"
)

var _ = Describe("GCM Message Handler", func() {
	var mockClient *mocks.GCMClientMock
	var mockDb *mocks.PGMock
	var mockKafkaConsumerClient *mocks.KafkaConsumerClientMock
	var mockKafkaProducerClient *mocks.KafkaProducerClientMock
	var handler *GCMMessageHandler
	var mockStatsDClient *mocks.StatsDClientMock
	var feedbackClients []interfaces.FeedbackReporter
	var statsClients []interfaces.StatsReporter

	configFile := "../config/test.yaml"
	senderID := "sender-id"
	apiKey := "api-key"
	appName := "testapp"
	isProduction := false
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	Describe("[Unit]", func() {
		BeforeEach(func() {
			var err error

			mockStatsDClient = mocks.NewStatsDClientMock()
			mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
			mockKafkaProducerClient.StartConsumingMessagesInProduceChannel()
			mockKafkaConsumerClient = mocks.NewKafkaConsumerClientMock()
			c, err := NewStatsD(configFile, logger, appName, mockStatsDClient)
			Expect(err).NotTo(HaveOccurred())

			kc, err := NewKafkaProducer(configFile, logger, mockKafkaProducerClient)
			Expect(err).NotTo(HaveOccurred())
			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			mockDb = mocks.NewPGMock(0, 1)

			mockClient = mocks.NewGCMClientMock()
			handler, err = NewGCMMessageHandler(
				configFile,
				senderID,
				apiKey,
				appName,
				isProduction,
				logger,
				nil,
				statsClients,
				feedbackClients,
				mockClient,
				mockDb,
			)
			Expect(err).NotTo(HaveOccurred())

			hook.Reset()
		})

		Describe("Creating new handler", func() {
			It("should return configured handler", func() {
				Expect(handler).NotTo(BeNil())
				Expect(handler.apiKey).To(Equal(apiKey))
				Expect(handler.appName).To(Equal(appName))
				Expect(handler.Config).NotTo(BeNil())
				Expect(handler.ConfigFile).To(Equal(configFile))
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
				err := handler.configure(nil, handler.PushDB.DB)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("error connecting gcm xmpp client: auth failure: not-authorized"))
			})

			It("should fail if cannot configure push DB", func() {
				mockDb.RowsReturned = 0
				err := handler.configure(mockClient, mockDb)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Timed out waiting for PostgreSQL to connect."))
			})
		})

		Describe("Handle GCM response", func() {
			It("if response has nil error", func() {
				res := gcm.CCSMessage{}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})

			It("if response has error DEVICE_UNREGISTERED", func() {
				res := gcm.CCSMessage{
					Error: "DEVICE_UNREGISTERED",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(hook.Entries).To(ContainLogMessage("deleting token"))
			})

			It("if response has error BAD_REGISTRATION", func() {
				res := gcm.CCSMessage{
					Error: "BAD_REGISTRATION",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
				Expect(hook.Entries).To(ContainLogMessage("deleting token"))
			})

			It("if response has error INVALID_JSON", func() {
				res := gcm.CCSMessage{
					Error: "INVALID_JSON",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})

			It("if response has error SERVICE_UNAVAILABLE", func() {
				res := gcm.CCSMessage{
					Error: "SERVICE_UNAVAILABLE",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})

			It("if response has error INTERNAL_SERVER_ERROR", func() {
				res := gcm.CCSMessage{
					Error: "INTERNAL_SERVER_ERROR",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})

			It("if response has error DEVICE_MESSAGE_RATE_EXCEEDED", func() {
				res := gcm.CCSMessage{
					Error: "DEVICE_MESSAGE_RATE_EXCEEDED",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})

			It("if response has error TOPICS_MESSAGE_RATE_EXCEEDED", func() {
				res := gcm.CCSMessage{
					Error: "TOPICS_MESSAGE_RATE_EXCEEDED",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})

			It("if response has untracked error", func() {
				res := gcm.CCSMessage{
					Error: "BAD_ACK",
				}
				handler.handleGCMResponse(res)
				Expect(handler.responsesReceived).To(Equal(int64(1)))
			})
		})

		Describe("Send message", func() {
			It("should send xmpp message and not increment sentMessages if an error occurs", func() {
				err := handler.sendMessage([]byte("gogogo"))
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
					DelayWhileIdle:           false,
					DeliveryReceiptRequested: false,
					DryRun: true,
					To:     uuid.NewV4().String(),
					Data:   map[string]interface{}{},
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(msgBytes)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler.sentMessages).To(Equal(int64(1)))
				Expect(hook.LastEntry().Message).To(Equal("sent message"))
				Expect(mockClient.MessagesSent).To(HaveLen(1))
				Expect(len(handler.pendingMessages)).To(Equal(1))
			})

			It("should send xmpp message with metadata", func() {
				ttl := uint(0)
				metadata := map[string]interface{}{
					"some": "metadata",
				}
				msg := &KafkaGCMMessage{
					gcm.XMPPMessage{
						TimeToLive:               &ttl,
						DelayWhileIdle:           false,
						DeliveryReceiptRequested: false,
						DryRun: true,
						To:     uuid.NewV4().String(),
						Data:   map[string]interface{}{},
					},
					metadata,
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(msgBytes)
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
					DelayWhileIdle:           false,
					DeliveryReceiptRequested: false,
					DryRun: true,
					To:     uuid.NewV4().String(),
					Data:   map[string]interface{}{},
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				for i := 1; i <= 3; i++ {
					err = handler.sendMessage(msgBytes)
					Expect(err).NotTo(HaveOccurred())
					Expect(handler.sentMessages).To(Equal(int64(i)))
					Expect(len(handler.pendingMessages)).To(Equal(i))
				}

				go handler.sendMessage(msgBytes)
				Consistently(handler.sentMessages).Should(Equal(int64(3)))
				Consistently(len(handler.pendingMessages)).Should(Equal(3))

				time.Sleep(100 * time.Millisecond)
				<-handler.pendingMessages
				Eventually(func() int64 { return handler.sentMessages }).Should(Equal(int64(4)))
			})
		})

		Describe("Handle Messages", func() {
			It("should start without panicking and set run to true", func() {
				queue, err := NewKafkaConsumer(handler.Config, logger, mockKafkaConsumerClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(func() { go handler.HandleMessages(queue.MessagesChannel()) }).ShouldNot(Panic())
				time.Sleep(time.Millisecond)
				Expect(handler.run).To(BeTrue())
			})
		})

		Describe("Stats Reporter sent message", func() {
			It("should call HandleNotificationSent upon message sent to queue", func() {
				ttl := uint(0)
				msg := &gcm.XMPPMessage{
					TimeToLive:               &ttl,
					DelayWhileIdle:           false,
					DeliveryReceiptRequested: false,
					DryRun: true,
					To:     uuid.NewV4().String(),
					Data:   map[string]interface{}{},
				}
				msgBytes, err := json.Marshal(msg)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(msgBytes)
				Expect(err).NotTo(HaveOccurred())

				err = handler.sendMessage(msgBytes)
				Expect(err).NotTo(HaveOccurred())

				Expect(mockStatsDClient.Count["sent"]).To(Equal(2))
			})

			It("should call HandleNotificationSuccess upon message response received", func() {
				res := gcm.CCSMessage{}
				handler.handleGCMResponse(res)
				handler.handleGCMResponse(res)

				Expect(mockStatsDClient.Count["ack"]).To(Equal(2))
			})

			It("should call HandleNotificationFailure upon message response received", func() {
				res := gcm.CCSMessage{
					Error: "DEVICE_UNREGISTERED",
				}
				handler.handleGCMResponse(res)
				handler.handleGCMResponse(res)

				Expect(mockStatsDClient.Count["failed"]).To(Equal(2))
				Expect(mockStatsDClient.Count["device_unregistered"]).To(Equal(2))
			})
		})

		Describe("Feedback Reporter sent message", func() {
			BeforeEach(func() {
				var err error

				mockKafkaProducerClient = mocks.NewKafkaProducerClientMock()
				kc, err := NewKafkaProducer(configFile, logger, mockKafkaProducerClient)
				Expect(err).NotTo(HaveOccurred())
				feedbackClients = []interfaces.FeedbackReporter{kc}

				mockClient = mocks.NewGCMClientMock()
				handler, err = NewGCMMessageHandler(
					configFile,
					senderID,
					apiKey,
					appName,
					isProduction,
					logger,
					nil,
					statsClients,
					feedbackClients,
					mockClient,
					mockDb,
				)
				Expect(err).NotTo(HaveOccurred())

			})

			It("should send feedback if success and metadata is present", func() {
				metadata := map[string]interface{}{
					"some": "metadata",
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

			It("should send feedback if error and metadata is present", func() {
				metadata := map[string]interface{}{
					"some": "metadata",
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
				Expect(handler.PushDB.DB.(*mocks.PGMock).Closed).To(BeTrue())
			})
		})
	})

	Describe("[Integration]", func() {
		BeforeEach(func() {
			var err error

			c, err := NewStatsD(configFile, logger, appName)
			Expect(err).NotTo(HaveOccurred())

			kc, err := NewKafkaProducer(configFile, logger)
			Expect(err).NotTo(HaveOccurred())
			statsClients = []interfaces.StatsReporter{c}
			feedbackClients = []interfaces.FeedbackReporter{kc}

			handler, err = NewGCMMessageHandler(
				configFile,
				senderID,
				apiKey,
				appName,
				isProduction,
				logger,
				nil,
				statsClients,
				feedbackClients,
				nil,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			hook.Reset()
		})

		Describe("Creating new handler", func() {
			It("should fail when real client", func() {
				var err error
				handler, err = NewGCMMessageHandler(
					configFile,
					senderID,
					apiKey,
					appName,
					isProduction,
					logger,
					nil,
					statsClients,
					feedbackClients,
					nil,
					nil,
				)
				Expect(handler).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error connecting gcm xmpp client: auth failure: not-authorized"))
			})
		})
	})
})
