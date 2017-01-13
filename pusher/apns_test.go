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

package pusher

import (
	"time"

	"github.com/Sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/topfreegames/pusher/extensions"
)

var _ = Describe("APNS Pusher", func() {
	configFile := "../config/test.yaml"
	certificatePath := "../tls/self_signed_cert.pem"
	isProduction := false
	appName := "testapp"
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		hook.Reset()
	})

	//Describe("[Unit]", func() {
	//Describe("Creating new apns pusher", func() {
	//It("should return configured pusher", func() {
	//pusher, err := NewAPNSPusher(configFile, certificatePath, appName, isProduction, logger)
	//Expect(err).NotTo(HaveOccurred())
	//Expect(pusher).NotTo(BeNil())
	//Expect(pusher.AppName).To(Equal(appName))
	//Expect(pusher.ConfigFile).To(Equal(configFile))
	//Expect(pusher.CertificatePath).To(Equal(certificatePath))
	//Expect(pusher.IsProduction).To(Equal(isProduction))
	//Expect(pusher.run).To(BeFalse())
	//Expect(pusher.Queue).NotTo(BeNil())
	//Expect(pusher.Config).NotTo(BeNil())
	//Expect(pusher.MessageHandler).NotTo(BeNil())

	//Expect(pusher.StatsReporters).To(HaveLen(1))
	//Expect(pusher.MessageHandler.(*extensions.APNSMessageHandler).StatsReporters).To(HaveLen(1))
	//})
	//})
	//})

	Describe("[Integration]", func() {
		Describe("Creating new apns pusher", func() {
			It("should return configured pusher", func() {
				pusher, err := NewAPNSPusher(configFile, certificatePath, appName, isProduction, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(pusher).NotTo(BeNil())
				Expect(pusher.AppName).To(Equal(appName))
				Expect(pusher.ConfigFile).To(Equal(configFile))
				Expect(pusher.CertificatePath).To(Equal(certificatePath))
				Expect(pusher.IsProduction).To(Equal(isProduction))
				Expect(pusher.run).To(BeFalse())
				Expect(pusher.Queue).NotTo(BeNil())
				Expect(pusher.Config).NotTo(BeNil())
				Expect(pusher.MessageHandler).NotTo(BeNil())

				Expect(pusher.StatsReporters).To(HaveLen(1))
				Expect(pusher.MessageHandler.(*extensions.APNSMessageHandler).StatsReporters).To(HaveLen(1))
			})
		})

		Describe("Start apns pusher", func() {
			It("should launch go routines and run forever", func() {
				pusher, err := NewAPNSPusher(configFile, certificatePath, appName, isProduction, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(pusher).NotTo(BeNil())
				defer func() { pusher.run = false }()
				go pusher.Start()
				time.Sleep(50 * time.Millisecond)
			})
		})
	})
})
