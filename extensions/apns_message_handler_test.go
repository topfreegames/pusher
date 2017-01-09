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

	"github.com/RobotsAndPencils/buford/push"
	"github.com/Sirupsen/logrus"
	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
)

var _ = Describe("APNS Message Handler", func() {
	configFile := "../config/test.yaml"
	certificatePath := "../tls/self_signed_cert.pem"
	isProduction := false
	appName := "testapp"
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		hook.Reset()
	})

	Describe("Creating new handler", func() {
		It("should return configured handler", func() {
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			Expect(handler).NotTo(BeNil())
			Expect(handler.appName).To(Equal(appName))
			Expect(handler.CertificatePath).To(Equal(certificatePath))
			Expect(handler.ConfigFile).To(Equal(configFile))
			Expect(handler.IsProduction).To(Equal(isProduction))
			Expect(handler.responsesReceived).To(Equal(int64(0)))
			Expect(handler.sentMessages).To(Equal(int64(0)))
		})
	})

	Describe("Handle token error", func() {
		It("should delete token", func() {
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			token := uuid.NewV4().String()
			insertQuery := fmt.Sprintf("INSERT INTO %s_apns (user_id, token, region, locale, tz) VALUES ('%s', '%s', 'BR', 'pt', '-0300');", appName, token, token)
			_, err := handler.PushDB.DB.ExecOne(insertQuery)
			Expect(err).NotTo(HaveOccurred())

			handler.handleTokenError(token)

			query := fmt.Sprintf("SELECT * FROM %s_apns WHERE token = '%s' LIMIT 1;", appName, token)
			_, err = handler.PushDB.DB.ExecOne(query)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("pg: no rows in result set"))
		})

		It("should not break if token does not exist in db", func() {
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			token := uuid.NewV4().String()
			Expect(func() { handler.handleTokenError(token) }).ShouldNot(Panic())
		})
	})

	Describe("Handle APNS response", func() {
		It("if reponse has nil error", func() {
			res := push.Response{
				DeviceToken: uuid.NewV4().String(),
				ID:          uuid.NewV4().String(),
			}
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(len(hook.Entries)).To(Equal(0))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.InfoLevel))
			Expect(hook.LastEntry().Message).To(Equal("deleting token"))
			Expect(hook.Entries[len(hook.Entries)-2].Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.InfoLevel))
			Expect(hook.LastEntry().Message).To(Equal("deleting token"))
			Expect(hook.Entries[len(hook.Entries)-2].Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.Entries[len(hook.Entries)-2].Data["category"]).To(Equal("TokenError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("CertificateError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("TopicError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("AppleError"))
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
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.handleAPNSResponse(res)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
			Expect(hook.LastEntry().Level).To(Equal(logrus.ErrorLevel))
			Expect(hook.LastEntry().Data["category"]).To(Equal("DefaultError"))
		})
	})

	Describe("Send message", func() {
		It("should add message to push queue and increment sentMessages", func() {
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			handler.sendMessage([]byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`))
			Eventually(handler.PushQueue.Responses, 5*time.Second).Should(Receive())
			Expect(handler.sentMessages).To(Equal(int64(1)))
		})
	})

	Describe("Handle Responses", func() {
		It("should be called without panicking", func() {
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			Expect(func() { go handler.HandleResponses(nil) }).ShouldNot(Panic())
			handler.PushQueue.Responses <- push.Response{}
			time.Sleep(50 * time.Millisecond)
			Expect(handler.responsesReceived).To(Equal(int64(1)))
		})
	})

	Describe("Handle Messages", func() {
		It("should start without panicking and set run to true", func() {
			handler := NewAPNSMessageHandler(configFile, certificatePath, appName, isProduction, logger)
			queue := NewKafka(configFile, logger)
			Expect(func() { go handler.HandleMessages(queue.MessagesChannel()) }).ShouldNot(Panic())
			time.Sleep(50 * time.Millisecond)
			Expect(handler.run).To(BeTrue())
		})
	})
})
