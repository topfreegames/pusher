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

	"github.com/sirupsen/logrus/hooks/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("TokenPG Extension", func() {
	var config *viper.Viper
	var mockClient *mocks.PGMock
	var tokenPG *TokenPG
	logger, hook := test.NewNullLogger()
	token := uuid.NewV4().String()

	BeforeEach(func() {
		var err error
		config, err = util.NewViperWithConfigFile("../config/test.yaml")
		Expect(err).NotTo(HaveOccurred())
		mockClient = mocks.NewPGMock(0, 1)
		tokenPG, err = NewTokenPG(config, logger, mockClient)
		Expect(err).NotTo(HaveOccurred())
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Creating new client", func() {
			It("should return connected client", func() {
				t, err := NewTokenPG(config, logger, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(t).NotTo(BeNil())
				Expect(t.Client).NotTo(BeNil())
				defer t.Cleanup()

				Expect(t.Config).NotTo(BeNil())
				Expect(t.Logger).NotTo(BeNil())
				Expect(t.tableName).To(Equal("test_apns"))
			})

			It("should return an error if failed to connect to postgres", func() {
				mockClient.RowsReturned = 0
				t, err := NewTokenPG(config, logger, mockClient)
				Expect(t).NotTo(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Timed out waiting for PostgreSQL to connect."))
			})
		})

		Describe("Handle invalid token", func() {
			It("should delete apns token", func() {
				err := tokenPG.HandleToken(token)
				Expect(err).NotTo(HaveOccurred())

				query := "DELETE FROM test_apns WHERE token = ?0;"
				Expect(mockClient.Execs).To(HaveLen(2))
				Expect(mockClient.Execs[1][0]).To(BeEquivalentTo(query))
				Expect(mockClient.Execs[1][1]).To(BeEquivalentTo([]interface{}{token}))
			})

			It("should not break if token does not exist in db", func() {
				mockClient.Error = fmt.Errorf("pg: no rows in result set")
				err := tokenPG.HandleToken(token)
				Expect(err).NotTo(HaveOccurred())

				query := "DELETE FROM test_apns WHERE token = ?0;"
				Expect(mockClient.Execs).To(HaveLen(2))
				Expect(mockClient.Execs[1][0]).To(BeEquivalentTo(query))
				Expect(mockClient.Execs[1][1]).To(BeEquivalentTo([]interface{}{token}))
			})

			It("should return an error if pg error occurred", func() {
				mockClient.Error = fmt.Errorf("pg: error")
				err := tokenPG.HandleToken(token)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("pg: error"))
			})
		})
	})
})
