package extensions

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/util"
	"os"
	"testing"
	"time"
)

type KafkaExtensionIntegrationTestSuite struct {
	suite.Suite
	logger *logrus.Logger
}

func TestKafkaExtensionIntegrationSuite(t *testing.T) {
	suite.Run(t, new(KafkaExtensionIntegrationTestSuite))
}

func (suite *KafkaExtensionIntegrationTestSuite) SetupSuite() {
	suite.logger = logrus.New()
}

func (suite *KafkaExtensionIntegrationTestSuite) TestConsumeLoopShouldReceiveMessage() {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	config, err := util.NewViperWithConfigFile(configFile)
	suite.NoError(err)

	stopChannel := make(chan struct{})
	client, err := NewKafkaConsumer(config, suite.logger, &stopChannel)
	suite.NoError(err)
	suite.NotNil(client)
	defer client.Cleanup()

	go client.ConsumeLoop()

	select {
	case <-client.Ready():
		suite.logger.Info("client is ready")
	case <-time.After(15 * time.Second):
		suite.Fail("timeout waiting for client to be ready")
	}

	time.Sleep(200 * time.Millisecond)

	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": client.Brokers})
	suite.NoError(err)
	topic := "push-mygame_apns-single"
	err = p.Produce(
		&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: kafka.PartitionAny,
			},
			Value: []byte("Hello Go!")},
		nil,
	)
	suite.NoError(err)
	suite.logger.Info("message produced")

	select {
	case msg := <-client.msgChan:
		suite.Equal([]byte("Hello Go!"), msg.Value)
	case <-time.After(2 * time.Minute):
		suite.Fail("timeout waiting for message to be received")
	}
}
