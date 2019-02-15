package feedback

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/testing"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("InvalidToken Handler", func() {
	var config *viper.Viper
	var err error

	configFile := "../config/test.yaml"

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

				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())
			})
		})

		Describe("Flush and Buffer", func() {
			var tokens []*InvalidToken
			BeforeEach(func() {
				tokens = []*InvalidToken{
					&InvalidToken{
						Token:    "flushA",
						Game:     "boomforce",
						Platform: "apns",
					},
					&InvalidToken{
						Token:    "flushB",
						Game:     "boomforce",
						Platform: "apns",
					},
				}
			})

			It("Should flush because buffer is full", func() {
				logger, hook := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				config.Set("feedbackListeners.invalidToken.flush.time.ms", 1000)
				config.Set("feedbackListeners.invalidToken.buffer.size", 2)

				logger.Level = logrus.DebugLevel
				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()
				for _, t := range tokens {
					inChan <- t
				}

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage("buffer is full"))
			})

			It("Should flush because reached flush timeout", func() {
				logger, hook := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				config.Set("feedbackListeners.invalidToken.flush.time.ms", 1)
				config.Set("feedbackListeners.invalidToken.buffer.size", 200)

				logger.Level = logrus.DebugLevel
				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()
				for _, t := range tokens {
					inChan <- t
				}

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage("flush ticker"))
			})
		})

		Describe("Deleting from database", func() {
			It("Should create correct queries", func() {
				logger, _ := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				config.Set("feedbackListeners.invalidToken.flush.time.ms", 10000)
				config.Set("feedbackListeners.invalidToken.buffer.size", 6)

				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()

				tokens := []*InvalidToken{
					&InvalidToken{
						Token:    "AAAAAAAAAA",
						Game:     "boomforce",
						Platform: "apns",
					},
					&InvalidToken{
						Token:    "BBBBBBBBBB",
						Game:     "boomforce",
						Platform: "apns",
					},
					&InvalidToken{
						Token:    "CCCCCCCCCC",
						Game:     "sniper",
						Platform: "apns",
					},
					&InvalidToken{
						Token:    "DDDDDDDDDD",
						Game:     "boomforce",
						Platform: "gcm",
					},
					&InvalidToken{
						Token:    "EEEEEEEEEE",
						Game:     "sniper",
						Platform: "gcm",
					},
					&InvalidToken{
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

				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()
				inChan <- &InvalidToken{
					Token:    "AAAAAAAA",
					Game:     "sniper",
					Platform: "apns",
				}
				Consistently(func() []*logrus.Entry { return hook.Entries }).
					ShouldNot(testing.ContainLogMessage("error deleting tokens"))
			})

			It("should not break if a pg error occurred", func() {
				logger, hook := test.NewNullLogger()
				mockClient := mocks.NewPGMock(0, 1)
				inChan := make(chan *InvalidToken, 100)

				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
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
			})
		})
	})
})
