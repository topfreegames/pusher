package feedback

import (
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
	var logger *logrus.Logger
	var hook *test.Hook
	var inChan chan *InvalidToken
	var config *viper.Viper
	var err error

	var mockClient *mocks.PGMock

	configFile := "../config/test.yaml"

	BeforeEach(func() {
		logger, hook = test.NewNullLogger()

		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())

		mockClient = mocks.NewPGMock(0, 1)
		inChan = make(chan *InvalidToken, 100)
	})

	Describe("[Unit]", func() {
		Describe("Creating new InvalidTokenHandler", func() {
			It("Should return a new handler", func() {
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
						Token:    "AAAAAAAAAA",
						Game:     "boomforce",
						Platform: "apns",
					},
					&InvalidToken{
						Token:    "BBBBBBBBBB",
						Game:     "boomforce",
						Platform: "apns",
					},
				}
				for _, t := range tokens {
					inChan <- t
				}

			})

			It("Should flush because buffer is full", func() {
				config.Set("invalidToken.flush.time.ms", 1000)
				config.Set("invalidToken.buffer.size", 2)

				logger.Level = logrus.DebugLevel
				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage("buffer is full"))
			})

			It("Should flush because reached flush timeout", func() {
				config.Set("invalidToken.flush.time.ms", 1)
				config.Set("invalidToken.buffer.size", 200)

				logger.Level = logrus.DebugLevel
				handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())

				handler.Start()

				Eventually(func() []*logrus.Entry { return hook.Entries }).
					Should(testing.ContainLogMessage("flush ticker"))
			})
		})

		Describe("Deleting from database", func() {
			var handler *InvalidTokenHandler
			var err error

			BeforeEach(func() {
				handler, err = NewInvalidTokenHandler(logger, config, &inChan, mockClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(handler).NotTo(BeNil())
			})

			It("Should create correct queries", func() {
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
					Tokens []string
				}{
					{
						Query:  "DELETE FROM boomforce_apns WHERE token IN (?0, ?1);",
						Tokens: []string{"AAAAAAAAAA", "BBBBBBBBBB"},
					},
					{
						Query:  "DELETE FROM sniper_apns WHERE token IN (?0);",
						Tokens: []string{"CCCCCCCCCC"},
					},
					{
						Query:  "DELETE FROM boomforce_gcm WHERE token IN (?0);",
						Tokens: []string{"DDDDDDDDDD"},
					},
					{
						Query:  "DELETE FROM sniper_gcm WHERE token IN (?0, ?1);",
						Tokens: []string{"EEEEEEEEEE", "FFFFFFFFFF"},
					},
				}

				for _, res := range expResults {
					Eventually(func() interface{} {
						for _, exec := range mockClient.Execs[1:] {
							if exec[0].(string) == res.Query {
								return exec[1].([]interface{})[0]
							}
						}
						return nil
					}).Should(Equal(res.Tokens))
				}
			})

			// It("should not break if token does not exist in db", func() {
			// 	mockClient.Error = fmt.Errorf("pg: no rows in result set")
			// 	handler, err = NewInvalidTokenHandler(logger, config, &inChan, mockClient)
			// 	Expect(err).NotTo(HaveOccurred())
			// 	Expect(handler).NotTo(BeNil())

			// })

			// It("should not break if a pg error occured", func() {

			// })
		})
	})
})
