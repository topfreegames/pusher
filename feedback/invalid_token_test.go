/*
 * Copyright (c) 2019 TFG Co <backend@tfgco.com>
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

package feedback

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("InvalidToken Handler", func() {
	var config *viper.Viper
	var mockStatsDClient *mocks.StatsDClientMock
	var statsReporters []interfaces.StatsReporter
	var err error

	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}

	BeforeEach(func() {
		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("[Unit]", func() {
		Describe("Creating new InvalidTokenHandler", func() {
			It("Should return a new handler", func() {
				logger, _ := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				mockStatsDClient = mocks.NewStatsDClientMock()
				c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
				Expect(err).NotTo(HaveOccurred())

				statsReporters = []interfaces.StatsReporter{c}

				handler, err := NewInvalidTokenHandler(logger, config, statsReporters, inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())
			})
		})

		Describe("Flush and Buffer", func() {
			var tokens []*InvalidToken
			BeforeEach(func() {
				tokens = []*InvalidToken{
					{
						Token:    "flushA",
						Game:     "boomforce",
						Platform: "apns",
					},
					{
						Token:    "flushB",
						Game:     "boomforce",
						Platform: "apns",
					},
				}
			})

			It("Should flush because buffer is full", func() {
				logger, hook := test.NewNullLogger()
				logger.Level = logrus.DebugLevel

				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				mockStatsDClient = mocks.NewStatsDClientMock()
				c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
				Expect(err).NotTo(HaveOccurred())

				statsReporters = []interfaces.StatsReporter{c}

				config.Set("feedbackListeners.invalidToken.flush.time.ms", 1000)
				config.Set("feedbackListeners.invalidToken.buffer.size", 2)

				handler, err := NewInvalidTokenHandler(logger, config, statsReporters, inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				mockClient.RowsAffected = 2
				mockClient.RowsReturned = 0
				handler.Start()
				for _, t := range tokens {
					inChan <- t
				}

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage("buffer is full"))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteSuccess]
				}).Should(BeEquivalentTo(2))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteError]
				}).Should(BeEquivalentTo(0))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteNonexistent]
				}).Should(BeEquivalentTo(0))
			})

			It("Should flush because reached flush timeout", func() {
				logger, hook := test.NewNullLogger()
				logger.Level = logrus.DebugLevel

				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				config.Set("feedbackListeners.invalidToken.flush.time.ms", 1)
				config.Set("feedbackListeners.invalidToken.buffer.size", 200)

				mockStatsDClient = mocks.NewStatsDClientMock()
				c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
				Expect(err).NotTo(HaveOccurred())

				statsReporters = []interfaces.StatsReporter{c}

				handler, err := NewInvalidTokenHandler(logger, config, statsReporters, inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				mockClient.RowsAffected = 2
				mockClient.RowsReturned = 0
				handler.Start()
				for _, t := range tokens {
					inChan <- t
				}

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage("flush ticker"))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteSuccess]
				}).Should(BeEquivalentTo(2))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteError]
				}).Should(BeEquivalentTo(0))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteNonexistent]
				}).Should(BeEquivalentTo(0))
			})
		})

		Describe("Deleting from database", func() {
			It("Should create correct queries", func() {
				logger, _ := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				config.Set("feedbackListeners.invalidToken.flush.time.ms", 10000)
				config.Set("feedbackListeners.invalidToken.buffer.size", 6)

				mockStatsDClient = mocks.NewStatsDClientMock()
				c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
				Expect(err).NotTo(HaveOccurred())

				statsReporters = []interfaces.StatsReporter{c}

				handler, err := NewInvalidTokenHandler(logger, config, statsReporters, inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()

				tokens := []*InvalidToken{
					{
						Token:    "AAAAAAAAAA",
						Game:     "boomforce",
						Platform: "apns",
					},
					{
						Token:    "BBBBBBBBBB",
						Game:     "boomforce",
						Platform: "apns",
					},
					{
						Token:    "CCCCCCCCCC",
						Game:     "sniper",
						Platform: "apns",
					},
					{
						Token:    "DDDDDDDDDD",
						Game:     "boomforce",
						Platform: "gcm",
					},
					{
						Token:    "EEEEEEEEEE",
						Game:     "sniper",
						Platform: "gcm",
					},
					{
						Token:    "FFFFFFFFFF",
						Game:     "sniper",
						Platform: "gcm",
					},
				}

				for _, t := range tokens {
					inChan <- t
				}

				time.Sleep(5 * time.Millisecond)
				expResults := []struct {
					Query  string
					Tokens []interface{}
				}{
					{
						Query:  "DELETE FROM boomforce_apns WHERE token IN (?0, ?1);",
						Tokens: []interface{}{"AAAAAAAAAA", "BBBBBBBBBB"},
					},
					{
						Query:  "DELETE FROM sniper_apns WHERE token IN (?0);",
						Tokens: []interface{}{"CCCCCCCCCC"},
					},
					{
						Query:  "DELETE FROM boomforce_gcm WHERE token IN (?0);",
						Tokens: []interface{}{"DDDDDDDDDD"},
					},
					{
						Query:  "DELETE FROM sniper_gcm WHERE token IN (?0, ?1);",
						Tokens: []interface{}{"EEEEEEEEEE", "FFFFFFFFFF"},
					},
				}

				for _, res := range expResults {
					Eventually(func() interface{} {
						for _, exec := range mockClient.Execs[1:] {
							if exec[0].(string) == res.Query {
								return exec[1]
							}
						}
						return nil
					}).Should(Equal(res.Tokens))
				}
			})

			It("should not break if token does not exist in db", func() {
				logger, hook := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				mockStatsDClient = mocks.NewStatsDClientMock()
				c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
				Expect(err).NotTo(HaveOccurred())

				statsReporters = []interfaces.StatsReporter{c}

				config.Set("feedbackListeners.invalidToken.buffer.size", 1)

				handler, err := NewInvalidTokenHandler(logger, config, statsReporters, inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				for len(mockClient.Execs) < 1 {
					// waiting connection to db
					time.Sleep(10 * time.Millisecond)
				}
				mockClient.Error = fmt.Errorf("pg: no rows in result set")
				mockClient.RowsAffected = 0
				mockClient.RowsReturned = 0

				handler.Start()
				inChan <- &InvalidToken{
					Token:    "AAAAAAAA",
					Game:     "sniper",
					Platform: "apns",
				}
				Consistently(func() []*logrus.Entry { return hook.Entries }).
					ShouldNot(testing.ContainLogMessage("error deleting tokens"))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteSuccess]
				}).Should(BeEquivalentTo(0))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteError]
				}).Should(BeEquivalentTo(0))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteNonexistent]
				}).Should(BeEquivalentTo(1))
			})

			It("should not break if a pg error occurred", func() {
				logger, hook := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				mockStatsDClient = mocks.NewStatsDClientMock()
				c, err := extensions.NewStatsD(config, logger, mockStatsDClient)
				Expect(err).NotTo(HaveOccurred())

				statsReporters = []interfaces.StatsReporter{c}

				config.Set("feedbackListeners.invalidToken.buffer.size", 1)

				handler, err := NewInvalidTokenHandler(logger, config, statsReporters, inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())
				handler.bufferSize = 1

				for len(mockClient.Execs) < 1 {
					// waiting connection to db
					time.Sleep(10 * time.Millisecond)
				}
				mockClient.Error = fmt.Errorf("pg: error")

				handler.Start()
				inChan <- &InvalidToken{
					Token:    "AAAAAAAAAA",
					Game:     "sniper",
					Platform: "apns",
				}

				for len(mockClient.Execs) < 2 {
					time.Sleep(10 * time.Millisecond)
				}

				Eventually(func() []*logrus.Entry {
					return hook.Entries
				}).Should(testing.ContainLogMessage("error deleting tokens"))

				mockClient.Error = nil
				mockClient.RowsAffected = 1
				mockClient.RowsReturned = 0

				inChan <- &InvalidToken{
					Token:    "BBBBBBBBBB",
					Game:     "sniper",
					Platform: "apns",
				}

				expQuery := "DELETE FROM sniper_apns WHERE token IN (?0);"
				expTokens := []interface{}{"BBBBBBBBBB"}

				Eventually(func() interface{} {
					if len(mockClient.Execs) >= 3 {
						return mockClient.Execs[2][0]
					}
					return nil
				}).Should(BeEquivalentTo(expQuery))

				Eventually(func() interface{} {
					if len(mockClient.Execs) >= 3 {
						return mockClient.Execs[2][1]
					}
					return nil
				}).Should(BeEquivalentTo(expTokens))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteSuccess]
				}).Should(BeEquivalentTo(1))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteError]
				}).Should(BeEquivalentTo(1))

				Eventually(func() int64 {
					return mockStatsDClient.Counts[MetricsTokensDeleteNonexistent]
				}).Should(BeEquivalentTo(0))
			})
		})
	})
})
