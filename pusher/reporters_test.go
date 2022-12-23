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
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Reporters", func() {
	var config *viper.Viper
	var mockClient *mocks.StatsDClientMock
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		var err error
		configFile := os.Getenv("CONFIG_FILE")
		if configFile == "" {
			configFile = "../config/test.yaml"
		}
		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())
		mockClient = mocks.NewStatsDClientMock()
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Configuring stats reporters", func() {
			It("should return stats reporter list", func() {
				handlers, err := configureStatsReporters(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handlers).To(HaveLen(1))
				Expect(handlers[0]).NotTo(BeNil())
			})

			It("should return an error if stats reporter is not available", func() {
				config.Set("stats.reporters", []string{"notAvailable"})
				handlers, err := configureStatsReporters(config, logger, mockClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to initialize notAvailable. Stats Reporter not available"))
				Expect(handlers).To(BeNil())
			})
		})

		Describe("Configuring feedback reporters", func() {
			It("should return feedback reporter list", func() {
				handlers, err := configureFeedbackReporters(config, logger)
				Expect(err).NotTo(HaveOccurred())
				Expect(handlers).To(HaveLen(1))
				Expect(handlers[0]).NotTo(BeNil())
			})

			It("should return an error if feedback reporter is not available", func() {
				config.Set("feedback.reporters", []string{"notAvailable"})
				handlers, err := configureFeedbackReporters(config, logger)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("failed to initialize notAvailable. Feedback Reporter not available"))
				Expect(handlers).To(BeNil())
			})
		})
	})
})
