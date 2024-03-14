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
	"github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"os"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	"github.com/topfreegames/pusher/util"
)

type KafkaExtensionTestSuite struct {
	suite.Suite
	logger              *logrus.Logger
	hook                *test.Hook
	kafkaConsumerClient *mocks.KafkaConsumerClientMock
	consumer            *KafkaConsumer
	config              *viper.Viper
}

func (suite *KafkaExtensionTestSuite) SetupTest() {
	suite.logger, suite.hook = test.NewNullLogger()
	suite.logger.Level = logrus.DebugLevel
	suite.kafkaConsumerClient = mocks.NewKafkaConsumerClientMock()
	suite.config = viper.New()
	suite.config.Set("queue.topics", []string{"com.games.test"})
	suite.config.Set("queue.brokers", "localhost:9941")
	suite.config.Set("queue.group", "testGroup")
	suite.config.Set("queue.sessionTimeout", 6000)
	suite.config.Set("queue.offsetResetStrategy", "latest")
	suite.config.Set("queue.handleAllMessagesBeforeExiting", true)

	stopChannel := make(chan struct{})
	var err error
	suite.consumer, err = NewKafkaConsumer(
		suite.config, suite.logger,
		&stopChannel, suite.kafkaConsumerClient,
	)
	suite.NoError(err)
}

func (suite *KafkaExtensionTestSuite) TestCreatingNewClient() {
	suite.NotEmpty(suite.consumer.Brokers)
	suite.Len(suite.consumer.Topics, 1)
	suite.NotNil(suite.consumer.ConsumerGroup)
	closedChanMatcher := gomega.BeClosed()
	isClosed, err := closedChanMatcher.Match(suite.consumer.msgChan)
	suite.NoError(err)
	suite.False(isClosed)
	suite.Equal(suite.kafkaConsumerClient, suite.consumer.Consumer)
}

func (suite *KafkaExtensionTestSuite) TestStopConsuming() {
	suite.consumer.run = true
	suite.consumer.StopConsuming()
	suite.False(suite.consumer.run)
}

func (suite *KafkaExtensionTestSuite) TestConsumeLoop_SubscribeFails() {
	suite.kafkaConsumerClient.Error = fmt.Errorf("could not subscribe")
	err := suite.consumer.ConsumeLoop()
	suite.Error(err)
	suite.Contains(err.Error(), "could not subscribe")
}

func (suite *KafkaExtensionTestSuite) TestConsumeLoop_SubscribeToTopic() {
	go suite.consumer.ConsumeLoop()
	defer suite.consumer.StopConsuming()
	suite.Eventually(func() bool {
		_, ok := suite.kafkaConsumerClient.SubscribedTopics["com.games.test"]
		return ok
	}, 5*time.Second, 1*time.Second)
}

func (suite *KafkaExtensionTestSuite) TestConsumeLoop_ReceiveMessage() {
	topic := "push-games_apns-single"
	go suite.consumer.ConsumeLoop()
	defer suite.consumer.StopConsuming()
	time.Sleep(5 * time.Millisecond)

	part := kafka.TopicPartition{
		Topic:     &topic,
		Partition: 1,
	}
	val := []byte("test")
	event := &kafka.Message{TopicPartition: part, Value: val}
	suite.consumer.messagesReceived = 999
	suite.kafkaConsumerClient.MessagesChan <- event

	timeout := time.Millisecond * 30
	timer := time.NewTimer(timeout)
	select {
	case <-timer.C:
		suite.Fail("timeout with no message received")
	case msg := <-suite.consumer.msgChan:
		suite.Equal(interfaces.KafkaMessage{
			Game:  "games",
			Topic: topic,
			Value: val,
		}, msg)
	}
	suite.Equal(int64(1000), suite.consumer.messagesReceived)
}

func (suite *KafkaExtensionTestSuite) TestConfigurationDefaults() {
	cnf := viper.New()
	stopChannel := make(chan struct{})
	cons, err := NewKafkaConsumer(cnf, suite.logger, &stopChannel, suite.kafkaConsumerClient)
	suite.NoError(err)
	cons.loadConfigurationDefaults()

	suite.Equal([]string{"com.games.test"}, cnf.GetStringSlice("queue.topics"))
	suite.Equal("localhost:9092", cnf.GetString("queue.brokers"))
	suite.Equal("test", cnf.GetString("queue.group"))
	suite.Equal(6000, cnf.GetInt("queue.sessionTimeout"))
	suite.Equal("latest", cnf.GetString("queue.offsetResetStrategy"))
	suite.True(cnf.GetBool("queue.handleAllMessagesBeforeExiting"))
}

func (suite *KafkaExtensionTestSuite) TestPendingMessagesWaitingGroup() {
	pmwg := suite.consumer.PendingMessagesWaitGroup()
	suite.NotNil(pmwg)
}

func (suite *KafkaExtensionTestSuite) TestCleanup_StopRunning() {
	suite.consumer.run = true
	err := suite.consumer.Cleanup()
	suite.NoError(err)
	suite.False(suite.consumer.run)
}

func (suite *KafkaExtensionTestSuite) TestCleanup_CloseConnection() {
	err := suite.consumer.Cleanup()
	suite.NoError(err)
	suite.True(suite.kafkaConsumerClient.Closed)
}

func (suite *KafkaExtensionTestSuite) TestCleanup_CloseConnectionError() {
	suite.kafkaConsumerClient.Error = fmt.Errorf("could not close connection")
	err := suite.consumer.Cleanup()
	suite.Error(err)
	suite.Equal(suite.kafkaConsumerClient.Error.Error(), err.Error())
}

func (suite *KafkaExtensionTestSuite) TestCreatingNewClient_Integration() {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	config, err := util.NewViperWithConfigFile(configFile)
	suite.NoError(err)

	stopChannel := make(chan struct{})
	client, err := NewKafkaConsumer(config, suite.logger, &stopChannel)
	suite.NoError(err)

	suite.NotEmpty(client.Brokers)
	suite.Len(client.Topics, 1)
	suite.NotNil(client.ConsumerGroup)
	closedChanMatcher := gomega.BeClosed()
	isClosed, err := closedChanMatcher.Match(client.msgChan)
	suite.NoError(err)
	suite.False(isClosed)
}

func TestKafkaExtensionSuite(t *testing.T) {
	suite.Run(t, new(KafkaExtensionTestSuite))
}
