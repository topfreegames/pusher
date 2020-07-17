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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
)

var _ = Describe("PG Extension", func() {
	var config *viper.Viper

	BeforeEach(func() {
		config = viper.New()
		config.SetConfigFile("../config/test.yaml")
		Expect(config.ReadInConfig()).NotTo(HaveOccurred())
	})

	Describe("[Unit]", func() {
		var mockDB *mocks.PGMock
		BeforeEach(func() {
			mockDB = mocks.NewPGMock(0, 1)
		})

		Describe("Connect", func() {
			It("Should use config to load connection details", func() {
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())
				Expect(client.Options).NotTo(BeNil())
			})
		})

		Describe("IsConnected", func() {
			It("should verify that db is connected", func() {
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())
				Expect(client.IsConnected()).To(BeTrue())
				Expect(mockDB.Execs).To(HaveLen(2))
			})

			It("should not be connected if error", func() {
				connErr := fmt.Errorf("could not connect")
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())
				mockDB.Error = connErr
				Expect(client.IsConnected()).To(BeFalse())
				Expect(mockDB.Execs).To(HaveLen(2))
			})

			It("should not be connected if zero rows returned", func() {
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				mockDB.RowsReturned = 0
				Expect(err).NotTo(HaveOccurred())
				Expect(client.IsConnected()).To(BeFalse())
				Expect(mockDB.Execs).To(HaveLen(2))
			})
		})

		Describe("Close", func() {
			It("should close if no errors", func() {
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				mockDB.RowsReturned = 0
				Expect(err).NotTo(HaveOccurred())
				err = client.Close()
				Expect(err).NotTo(HaveOccurred())
				Expect(mockDB.Closed).To(BeTrue())
			})

			It("should return error", func() {
				connErr := fmt.Errorf("could not close")
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())

				mockDB.Error = connErr

				err = client.Close()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("could not close"))
			})
		})

		Describe("WaitForConnection", func() {
			It("should wait for connection", func() {
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())

				err = client.WaitForConnection(1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should error waiting for connection", func() {
				pErr := fmt.Errorf("connection failed")
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())
				mockDB.Error = pErr

				err = client.WaitForConnection(10)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timed out waiting for PostgreSQL to connect"))
			})
		})

		Describe("Cleanup", func() {
			It("should close connection", func() {
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())
				mockDB.RowsReturned = 0

				err = client.Cleanup()
				Expect(err).NotTo(HaveOccurred())
				Expect(mockDB.Closed).To(BeTrue())
			})

			It("should return error if error when closing connection", func() {
				pErr := fmt.Errorf("failed to close connection")
				mockDB = mocks.NewPGMock(0, 1)
				client, err := NewPGClient("push.db", config, mockDB)
				Expect(err).NotTo(HaveOccurred())
				mockDB.Error = pErr
				mockDB.RowsReturned = 0

				err = client.Cleanup()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to close connection"))
			})
		})
	})

	Describe("[Integration]", func() {
		PDescribe("Creating new client", func() {
			It("should return connected client", func() {
				client, err := NewPGClient("push.db", config)
				Expect(err).NotTo(HaveOccurred())
				defer client.Close()
				Expect(client).NotTo(BeNil())
				client.WaitForConnection(10)
				Expect(client.IsConnected()).To(BeTrue())
			})
		})
	})
})
