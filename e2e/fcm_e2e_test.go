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

	s.assureTopicsExist(appName)
	time.Sleep(wait)

	// Required to instantiate at least one client to pusher.NewGCMPusher run without errors. If there is no Apps on s.config.GCM.Apps,
	// an array of apps is created with len=1 and the method tries to create a GCMClient
	s.config.GCM.Apps = appName
	s.vConfig.Set(fmt.Sprintf("gcm.firebaseCredentials.%s", appName), "{ \"project_id\": \"test-app\", \"type\": \"service_account\" }")

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
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(gcmTopicTemplate, appName)})

	mockFcmClient, statsdClientMock := s.setupFcmPusher(appName)
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	topic := fmt.Sprintf(gcmTopicTemplate, appName)
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

	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	statsdClientMock.EXPECT().
		Timing("firebase_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	err = producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: []byte(`{"deviceToken":"` + token + `", "payload": {"gcm": {"alert": "Hello"}}}`),
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
	s.config.GCM.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(gcmTopicTemplate, appName)})

	mockFcmClient, statsdClientMock := s.setupFcmPusher(appName)

	notificationsToSend := 10
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	topic := fmt.Sprintf(gcmTopicTemplate, appName)
	token := "token"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockFcmClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(nil)
	}

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
			return nil
		})

	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(notificationsToSend).
		Return(nil)

	statsdClientMock.EXPECT().
		Timing("firebase_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(notificationsToSend).
		Return(nil)

	for i := 0; i < notificationsToSend; i++ {
		err = producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: kafka.PartitionAny,
			},
			Value: []byte(`{"deviceToken":"` + fmt.Sprintf("%s%d", token, i) + `", "payload": {"gcm": {"alert": "Hello"}}}`),
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

func (s *FcmE2ETestSuite) assureTopicsExist(appName string) {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	topic := fmt.Sprintf(gcmTopicTemplate, appName)
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
