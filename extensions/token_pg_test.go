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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	testing "github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("TokenPG Extension", func() {
	var config *viper.Viper
	var mockClient *mocks.PGMock
	var tokenPG *TokenPG
	var mockStatsDClient *mocks.StatsDClientMock
	var statsClients []interfaces.StatsReporter
	logger, hook := test.NewNullLogger()
	token := uuid.NewV4().String()

	BeforeEach(func() {
		var err error
		config, err = util.NewViperWithConfigFile("../config/test.yaml")
		Expect(err).NotTo(HaveOccurred())

		mockStatsDClient = mocks.NewStatsDClientMock()
		c, err := NewStatsD(config, logger, mockStatsDClient)
		Expect(err).NotTo(HaveOccurred())

		statsClients = []interfaces.StatsReporter{c}

		mockClient = mocks.NewPGMock(0, 1)
		tokenPG, err = NewTokenPG(config, logger, statsClients, mockClient)
		Expect(err).NotTo(HaveOccurred())
		hook.Reset()
	})

	Describe("[Unit]", func() {
		Describe("Creating new client", func() {
			It("should return connected client", func() {
				t, err := NewTokenPG(config, logger, statsClients, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(t).NotTo(BeNil())
				Expect(t.Client).NotTo(BeNil())
				defer t.Cleanup()

				Expect(t.Config).NotTo(BeNil())
				Expect(t.Logger).NotTo(BeNil())
			})

			It("should return an error if failed to connect to postgres", func() {
				mockClient.RowsReturned = 0
				t, err := NewTokenPG(config, logger, statsClients, mockClient)
				Expect(t).NotTo(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Timed out waiting for PostgreSQL to connect"))
			})
		})

		Describe("Handle invalid token", func() {
			deletionError := "error deleting token"
			It("should delete apns token", func() {
				tokenPG.HandleToken(token, "test", "apns")
				Consistently(func() []*logrus.Entry { return hook.Entries }).
					ShouldNot(testing.ContainLogMessage(deletionError))

				query := "DELETE FROM test_apns WHERE token = ?0;"
				Eventually(func() [][]interface{} { return mockClient.Execs }).
					Should(HaveLen(2))
				Eventually(func() interface{} { return mockClient.Execs[1][0] }).
					Should(BeEquivalentTo(query))
				Eventually(func() interface{} { return mockClient.Execs[1][1] }).
					Should(BeEquivalentTo([]interface{}{token}))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeleted] }).
					Should(Equal(1))
			})

			It("should not break if token does not exist in db", func() {
				mockClient.Error = fmt.Errorf("pg: no rows in result set")
				tokenPG.HandleToken(token, "test", "apns")
				Consistently(func() []*logrus.Entry { return hook.Entries }).
					ShouldNot(testing.ContainLogMessage(deletionError))

				query := "DELETE FROM test_apns WHERE token = ?0;"
				Eventually(func() [][]interface{} { return mockClient.Execs }).
					Should(HaveLen(2))
				Eventually(func() interface{} { return mockClient.Execs[1][0] }).
					Should(BeEquivalentTo(query))
				Eventually(func() interface{} { return mockClient.Execs[1][1] }).
					Should(BeEquivalentTo([]interface{}{token}))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeleted] }).
					Should(Equal(1))
			})

			It("should return an error if pg error occurred", func() {
				mockClient.Error = fmt.Errorf("pg: error")
				tokenPG.HandleToken(token, "test", "apns")

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage(deletionError))

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(1))
				Eventually(func() int { return mockStatsDClient.Count[MetricsTokensDeletionError] }).
					Should(Equal(1))
			})
		})

		Describe("Stopping a tokenPG client", func() {
			It("without waiting", func() {
				tokenPG.Stop()

				Expect(hook.Entries).
					To(testing.ContainLogMessage("waiting the worker to finish"))
				Expect(hook.Entries).
					To(testing.ContainLogMessage("tokenPG closed"))
			})

			It("lefting job undone", func() {
				size := 1000
				tokens := make([]string, size)
				for i := 0; i < len(tokens); i++ {
					tokens[i] = uuid.NewV4().String()
					tokenPG.HandleToken(tokens[i], "test", "apns")
				}

				tokenPG.Stop()
				Expect(hook.Entries).
					To(testing.ContainLogMessage("waiting the worker to finish"))
				Expect(hook.Entries).
					To(testing.ContainLogMessage("tokenPG closed"))
				Expect(hook.Entries).
					To(testing.ContainLogMessage(fmt.Sprintf("%d tokens haven't been processed", len(tokenPG.tokensToDelete))))

				err := tokenPG.HandleToken(token, "test", "apns")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("can't handle more tokens. The TokenPG has been stopped"))
				Consistently(tokenPG.HasJob).Should(BeTrue())

				Expect(mockStatsDClient.Count[MetricsTokensHandleAfterClosing]).To(Equal(1))
			})

			It("waiting finish jobs to stop", func() {
				size := 1000
				tokens := make([]string, size)
				for i := 0; i < len(tokens); i++ {
					tokens[i] = uuid.NewV4().String()
					tokenPG.HandleToken(tokens[i], "test", "apns")
				}

				for tokenPG.HasJob() == true {
					time.Sleep(10 * time.Millisecond)
				}
				tokenPG.Stop()

				Expect(hook.Entries).
					To(testing.ContainLogMessage("waiting the worker to finish"))
				Expect(hook.Entries).
					To(testing.ContainLogMessage("tokenPG closed"))

				Expect(mockClient.Execs).To(HaveLen(size + 1))
				query := "DELETE FROM test_apns WHERE token = ?0;"
				for i := 0; i < len(tokens); i++ {
					Expect(mockClient.Execs[i+1][0]).To(BeIdenticalTo(query))
					Expect(mockClient.Execs[i+1][1]).To(BeEquivalentTo([]interface{}{tokens[i]}))
				}

				Expect(mockStatsDClient.Count[MetricsTokensToDelete]).To(Equal(size))
				Expect(mockStatsDClient.Count[MetricsTokensDeleted]).To(Equal(size))
			})
		})
	})
})
