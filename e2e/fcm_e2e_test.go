package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/extensions/handler"
	"github.com/topfreegames/pusher/interfaces"
	firebaseMock "github.com/topfreegames/pusher/mocks/firebase"
	mocks "github.com/topfreegames/pusher/mocks/interfaces"
	"github.com/topfreegames/pusher/pusher"
	"go.uber.org/mock/gomock"
)

type FcmE2ETestSuite struct {
	suite.Suite

	config  *config.Config
	vConfig *viper.Viper
}

func TestFcmE2eSuite(t *testing.T) {
	suite.Run(t, new(FcmE2ETestSuite))
}

func (s *FcmE2ETestSuite) SetupSuite() {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	c, v, err := config.NewConfigAndViper(configFile)
	// TODO: explain this
	v.Set("gcm.apps", "")

	s.Require().NoError(err)
	s.config = c
	s.vConfig = v
}

func (s *FcmE2ETestSuite) setupFcmPusher(appName string) (*firebaseMock.MockPushClient, *mocks.MockStatsDClient) {
	ctrl := gomock.NewController(s.T())

	statsdClientMock := mocks.NewMockStatsDClient(ctrl)
	// Gauge can be called any times from the go stats report
	statsdClientMock.EXPECT().Gauge(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	s.assureTopicsExist()
	time.Sleep(wait)

	ctx := context.Background()
	gcmPusher, err := pusher.NewGCMPusher(ctx, false, s.vConfig, s.config, logger, statsdClientMock)
	s.Require().NoError(err)

	statsReport, err := extensions.NewStatsD(s.vConfig, logger, statsdClientMock)
	s.Require().NoError(err)

	pushClient := firebaseMock.NewMockPushClient(ctrl)
	gcmPusher.MessageHandler = map[string]interfaces.MessageHandler{
		appName: handler.NewMessageHandler(
			appName,
			pushClient,
			[]interfaces.FeedbackReporter{},
			[]interfaces.StatsReporter{statsReport},
			logger,
			s.config.GCM.ConcurrentWorkers,
		),
	}
	go gcmPusher.Start(ctx)

	time.Sleep(wait)

	return pushClient, statsdClientMock
}

func (s *FcmE2ETestSuite) TestSimpleNotification() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(topicTemplate, appName)})

	mockFcmClient, statsdClientMock := s.setupFcmPusher(appName)
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	topic := "push-" + appName + "_gcm-single"
	token := "token"
	testDone := make(chan bool)
	mockFcmClient.EXPECT().
		SendPush(gomock.Any(), gomock.Any()).
		Return(nil)

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			testDone <- true
			return nil
		})

	err = producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: []byte(`{"deviceToken":"` + token + `", "payload": {"aps": {"alert": "Hello"}}}`),
	},
		nil)
	s.Require().NoError(err)

	// Give it some time to process the message
	timer := time.NewTimer(timeout)
	select {
	case <-testDone:
		// Wait some time to make sure it won't call the push client again after the testDone signal
		time.Sleep(wait)
	case <-timer.C:
		s.FailNow("Timeout waiting for Handler to report notification sent")
	}
}

func (s *FcmE2ETestSuite) TestMultipleNotifications() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.config.Apns.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(topicTemplate, appName)})

	mockApnsClient, statsdClientMock := s.setupFcmPusher(appName)

	notificationsToSend := 10
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	topic := fmt.Sprintf(topicTemplate, appName)
	token := "token"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockApnsClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(nil)
	}

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", appName)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", appName)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
			return nil
		})

	for i := 0; i < notificationsToSend; i++ {
		err = producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: kafka.PartitionAny,
			},
			Value: []byte(`{"deviceToken":"` + fmt.Sprintf("%s%d", token, i) + `", "payload": {"aps": {"alert": "Hello"}}}`),
		},
			nil)
		s.Require().NoError(err)
	}
	// Give it some time to process the message
	timer := time.NewTimer(timeout)
	for i := 0; i < notificationsToSend; i++ {
		select {
		case <-done:
		case <-timer.C:
			s.FailNow("Timeout waiting for Handler to report notification sent")
		}
	}
	// Wait some time to make sure it won't call the push client again after everything is done
	time.Sleep(wait)
}

func (s *FcmE2ETestSuite) assureTopicsExist() {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	apnsApps := s.config.GetApnsAppsArray()
	for _, a := range apnsApps {
		topic := fmt.Sprintf(topicTemplate, a)
		err = producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: kafka.PartitionAny,
			},
			Value: []byte("not a notification"),
		},
			nil)
		s.Require().NoError(err)
	}
}
