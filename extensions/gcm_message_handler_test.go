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
	"time"

	pg "gopkg.in/pg.v5"

	"github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rounds/go-gcm"
	uuid "github.com/satori/go.uuid"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
)

var _ = Describe("GCM Message Handler", func() {
	var mockClient *mocks.GCMClientMock
	var handler *GCMMessageHandler
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter

	configFile := "../config/test.yaml"
	senderID := "sender-id"
	apiKey := "api-key"
	appName := "testapp"
	isProduction := false
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		var err error

		mockStatsDClient = mocks.NewStatsDClientMock()
		c, err := NewStatsD(configFile, logger, appName, mockStatsDClient)
		Expect(err).NotTo(HaveOccurred())

		statsClients = []interfaces.StatsReporter{c}

		mockClient = mocks.NewGCMClientMock()
		handler, err = NewGCMMessageHandler(
			configFile, senderID, apiKey, appName,
			isProduction, logger,
			nil, statsClients, mockClient,
		)
		Expect(err).NotTo(HaveOccurred())

		hook.Reset()
	})

	Describe("Creating new handler", func() {
		It("should fail when real client", func() {
			handler, err := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil, statsClients, nil)
			Expect(handler).To(BeNil())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error connecting gcm xmpp client: auth failure: not-authorized"))
		})

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

	Describe("Handle token error", func() {
		It("should delete token", func() {
			token := uuid.NewV4().String()
			insertQuery := fmt.Sprintf(
				"INSERT INTO %s_gcm (user_id, token, region, locale, tz) VALUES (?0, ?0, 'BR', 'pt', '-0300');",
				appName,
			)
			_, err := handler.PushDB.DB.ExecOne(insertQuery, token)
			Expect(err).NotTo(HaveOccurred())

			err = handler.handleTokenError(token)
			Expect(err).NotTo(HaveOccurred())

			count := 100
			query := fmt.Sprintf("SELECT count(*) FROM %s_gcm WHERE token = ?0 LIMIT 1;", appName)
			_, err = handler.PushDB.DB.Query(pg.Scan(&count), query, token)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(0))
		})

		It("should not break if token does not exist in db", func() {
			token := uuid.NewV4().String()
			err := handler.handleTokenError(token)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Handle GCM response", func() {
		It("if response has nil error", func() {
			res := gcm.CCSMessage{}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(len(hook.Entries)).To(Equal(0))
		})

		It("if response has error DEVICE_UNREGISTERED", func() {
			res := gcm.CCSMessage{
				Error: "DEVICE_UNREGISTERED",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.InfoLevel))
			Expect(hook.LastEntry().Message).To(Equal("Deleting token..."))
			Expect(hook.Entries[len(hook.Entries)-2].Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
		})

		It("if response has error BAD_REGISTRATION", func() {
			res := gcm.CCSMessage{
				Error: "BAD_REGISTRATION",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.InfoLevel))
			Expect(hook.LastEntry().Message).To(Equal("Deleting token..."))
			Expect(hook.Entries[len(hook.Entries)-2].Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
		})

		It("if response has error INVALID_JSON", func() {
			res := gcm.CCSMessage{
				Error: "INVALID_JSON",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("JsonError"))
		})

		It("if response has error SERVICE_UNAVAILABLE", func() {
			res := gcm.CCSMessage{
				Error: "SERVICE_UNAVAILABLE",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("GoogleError"))
		})

		It("if response has error INTERNAL_SERVER_ERROR", func() {
			res := gcm.CCSMessage{
				Error: "INTERNAL_SERVER_ERROR",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("GoogleError"))
		})

		It("if response has error DEVICE_MESSAGE_RATE_EXCEEDED", func() {
			res := gcm.CCSMessage{
				Error: "DEVICE_MESSAGE_RATE_EXCEEDED",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("RateExceededError"))
		})

		It("if response has error TOPICS_MESSAGE_RATE_EXCEEDED", func() {
			res := gcm.CCSMessage{
				Error: "TOPICS_MESSAGE_RATE_EXCEEDED",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("RateExceededError"))
		})

		It("if response has untracked error", func() {
			res := gcm.CCSMessage{
				Error: "BAD_ACK",
			}
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("DefaultError"))
		})
	})

	Describe("Send message", func() {
		It("should send xmpp message and not increment sentMessages if an error occurs", func() {
			err := handler.sendMessage([]byte("gogogo"))
			Expect(err).To(HaveOccurred())
			Expect(handler.sentMessages).To(Equal(int64(0)))
			Expect(hook.LastEntry()).NotTo(BeNil())
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Message).To(ContainSubstring("Error sending message."))
			Expect(mockClient.MessagesSent).To(HaveLen(0))
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
			Expect(hook.LastEntry()).To(BeNil())
			Expect(mockClient.MessagesSent).To(HaveLen(1))
		})
	})

	Describe("Handle Messages", func() {
		It("should start without panicking and set run to true", func() {
			queue := NewKafka(configFile, logger)
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
})
