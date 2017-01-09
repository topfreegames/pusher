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
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rounds/go-gcm"
	uuid "github.com/satori/go.uuid"
)

var _ = Describe("GCM Message Handler", func() {
	configFile := "../config/test.yaml"
	senderID := "sender-id"
	apiKey := "api-key"
	appName := "testapp"
	isProduction := false
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		hook.Reset()
	})

	Describe("Creating new handler", func() {
		It("should return configured handler", func() {
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			Expect(handler).NotTo(BeNil())
			Expect(handler.apiKey).To(Equal(apiKey))
			Expect(handler.appName).To(Equal(appName))
			Expect(handler.Config).NotTo(BeNil())
			Expect(handler.ConfigFile).To(Equal(configFile))
			Expect(handler.IsProduction).To(Equal(isProduction))
			Expect(handler.senderID).To(Equal(senderID))
			Expect(handler.responsesReceived).To(Equal(int64(0)))
			Expect(handler.sentMessages).To(Equal(int64(0)))
		})
	})

	Describe("Handle token error", func() {
		It("should delete token", func() {
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			token := uuid.NewV4().String()
			insertQuery := fmt.Sprintf("INSERT INTO %s_gcm (user_id, token, region, locale, tz) VALUES ('%s', '%s', 'BR', 'pt', '-0300');", appName, token, token)
			_, err := handler.PushDB.DB.ExecOne(insertQuery)
			Expect(err).NotTo(HaveOccurred())

			handler.handleTokenError(token)

			query := fmt.Sprintf("SELECT * FROM %s_gcm WHERE token = '%s' LIMIT 1;", appName, token)
			_, err = handler.PushDB.DB.ExecOne(query)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("pg: no rows in result set"))
		})

		It("should not break if token does not exist in db", func() {
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			token := uuid.NewV4().String()
			Expect(func() { handler.handleTokenError(token) }).ShouldNot(Panic())
		})
	})

	Describe("Handle GCM response", func() {
		It("if reponse has nil error", func() {
			res := gcm.CCSMessage{}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(len(hook.Entries)).To(Equal(0))
		})

		It("if reponse has error DEVICE_UNREGISTERED", func() {
			res := gcm.CCSMessage{
				Error: "DEVICE_UNREGISTERED",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.InfoLevel))
			Expect(hook.LastEntry().Message).To(Equal("deleting token"))
			Expect(hook.Entries[len(hook.Entries)-2].Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
		})

		It("if reponse has error BAD_REGISTRATION", func() {
			res := gcm.CCSMessage{
				Error: "BAD_REGISTRATION",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.InfoLevel))
			Expect(hook.LastEntry().Message).To(Equal("deleting token"))
			Expect(hook.Entries[len(hook.Entries)-2].Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
		})

		It("if reponse has error INVALID_JSON", func() {
			res := gcm.CCSMessage{
				Error: "INVALID_JSON",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("JsonError"))
		})

		It("if reponse has error SERVICE_UNAVAILABLE", func() {
			res := gcm.CCSMessage{
				Error: "SERVICE_UNAVAILABLE",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("GoogleError"))
		})

		It("if reponse has error INTERNAL_SERVER_ERROR", func() {
			res := gcm.CCSMessage{
				Error: "INTERNAL_SERVER_ERROR",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("GoogleError"))
		})

		It("if reponse has error DEVICE_MESSAGE_RATE_EXCEEDED", func() {
			res := gcm.CCSMessage{
				Error: "DEVICE_MESSAGE_RATE_EXCEEDED",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("RateExceededError"))
		})

		It("if reponse has error TOPICS_MESSAGE_RATE_EXCEEDED", func() {
			res := gcm.CCSMessage{
				Error: "TOPICS_MESSAGE_RATE_EXCEEDED",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("RateExceededError"))
		})

		It("if reponse has untracked error", func() {
			res := gcm.CCSMessage{
				Error: "BAD_ACK",
			}
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.handleGCMResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("DefaultError"))
		})
	})

	Describe("Send message", func() {
		// TODO: test successfull case
		It("should send xmpp message and not increment sentMessages if an error occurs", func() {
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			handler.sendMessage([]byte(`gogogo`))
			Expect(handler.sentMessages).To(Equal(int64(0)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Message).To(ContainSubstring("error sending message"))
		})
	})

	Describe("Handle Responses", func() {
		It("should be called without panicking", func() {
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			Expect(func() { handler.HandleResponses() }).ShouldNot(Panic())
		})
	})

	Describe("Handle Messages", func() {
		It("should start without panicking and set run to true", func() {
			handler := NewGCMMessageHandler(configFile, senderID, apiKey, appName, isProduction, logger, nil)
			queue := NewKafka(configFile, logger)
			Expect(func() { go handler.HandleMessages(queue.MessagesChannel()) }).ShouldNot(Panic())
			time.Sleep(time.Millisecond)
			Expect(handler.run).To(BeTrue())
		})
	})
})
