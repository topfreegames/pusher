package feedback

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

var _ = Describe("[Unit]", func() {
	var logger *logrus.Logger
	// var hook *test.Hook
	var inChan chan *InvalidToken
	var config *viper.Viper
	var err error

	var mockClient *mocks.PGMock

	configFile := "../config/test.yaml"

	BeforeEach(func() {
		logger, _ = test.NewNullLogger()

		config, err = util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())

		mockClient = mocks.NewPGMock(0, 1)
		inChan = make(chan *InvalidToken, 100)
	})

	Describe("Creating new InvalidTokenHandler", func() {
		It("Should return a new handler", func() {
			handler, err := NewInvalidTokenHandler(logger, config, &inChan, mockClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(handler).NotTo(BeNil())
		})
	})

})
