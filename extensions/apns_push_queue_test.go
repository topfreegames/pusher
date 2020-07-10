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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("APNS Message Handler", func() {
	var queue *APNSPushQueue

	configFile := "../config/test.yaml"
	config, _ := util.NewViperWithConfigFile(configFile)
	authKeyPath := "../tls/authkey.p8"
	keyID := "ABC123DEFG"
	teamID := "DEF123GHIJ"
	isProduction := false
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel

	Describe("[Unit]", func() {
		BeforeEach(func() {
			queue = NewAPNSPushQueue(
				authKeyPath,
				keyID,
				teamID,
				isProduction,
				logger,
				config,
			)
			hook.Reset()
		})

		Describe("Creating new Queue", func() {
			It("should return configured queue", func() {
				Expect(queue).NotTo(BeNil())
				Expect(queue.Config).NotTo(BeNil())
				Expect(queue.IsProduction).To(Equal(isProduction))
			})
		})

		Describe("Configuring Queue", func() {
			It("should fail if invalid pem file", func() {
				queue.authKeyPath = "./invalid-certficate.pem"
				err := queue.Configure()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("open ./invalid-certficate.pem: no such file or directory"))
			})
		})

		Describe("Configuring Certificate", func() {
			It("should configure from pem file", func() {
				err := queue.configureCertificate()
				Expect(err).NotTo(HaveOccurred())
				Expect(queue.token).NotTo(BeNil())
			})

			It("should fail if invalid pem file", func() {
				queue.authKeyPath = "./invalid-certficate.pem"
				err := queue.configureCertificate()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
