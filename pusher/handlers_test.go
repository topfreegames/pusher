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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("Handlers", func() {
	var config *viper.Viper
	var mockClient *mocks.PGMock
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		var err error
		config, err = util.NewViperWithConfigFile("../config/test.yaml")
		Expect(err).NotTo(HaveOccurred())
		mockClient = mocks.NewPGMock(0, 1)
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Configuring invalid token handlers", func() {
			It("should return invalid token handler list", func() {
				handlers, err := configureInvalidTokenHandlers(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handlers).To(HaveLen(1))
				Expect(handlers[0]).NotTo(BeNil())
			})

			It("should return an error if token handler is not available", func() {
				config.Set("invalidToken.handlers", []string{"notAvailable"})
				handlers, err := configureInvalidTokenHandlers(config, logger, mockClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Failed to initialize notAvailable. Invalid Token Handler not available."))
				Expect(handlers).To(BeNil())
			})

			It("should return an error if failed to create invalid token handler", func() {
				mockClient.RowsReturned = 0
				handlers, err := configureInvalidTokenHandlers(config, logger, mockClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Failed to initialize pg. Timed out waiting for PostgreSQL to connect."))
				Expect(handlers).To(BeNil())
			})
		})
	})
})
