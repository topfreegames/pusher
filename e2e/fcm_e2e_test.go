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
	"github.com/topfreegames/pusher/extensions/firebase"
	"github.com/topfreegames/pusher/interfaces"
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

func (s *FcmE2ETestSuite) setupFcmPusher(appName string) (*mocks.MockPushClient, *mocks.MockStatsDClient) {
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
	s.vConfig.Set("dedup.games", fmt.Sprintf(`{"%s": 100}`, appName))

	ctx := context.Background()
	gcmPusher, err := pusher.NewGCMPusher(ctx, false, s.vConfig, s.config, logger, statsdClientMock)
	s.Require().NoError(err)

	statsReport, err := extensions.NewStatsD(s.vConfig, logger, statsdClientMock)
	s.Require().NoError(err)

	limit := s.vConfig.GetInt("gcm.rateLimit.rpm")
	rateLimiter := extensions.NewRateLimiter(limit, s.vConfig, []interfaces.StatsReporter{statsReport}, logger)

	ttl := s.vConfig.GetDuration("gcm.dedup.ttl")
	dedup := extensions.NewDedup(ttl, s.vConfig, []interfaces.StatsReporter{statsReport}, logger)

	pushClient := mocks.NewMockPushClient(ctrl)
	gcmPusher.MessageHandler = map[string]interfaces.MessageHandler{
		appName: firebase.NewMessageHandler(
			appName,
			pushClient,
			[]interfaces.FeedbackReporter{},
			[]interfaces.StatsReporter{statsReport},
			rateLimiter,
			dedup,
			nil,
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
	token := "token-simple"
	testDone := make(chan bool)
	mockFcmClient.EXPECT().
		SendPush(gomock.Any(), gomock.Any()).
		Return(nil)

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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
		Value: []byte(`{"to":"` + token + `", "data": {"alert": "Hello Simple"}}`),
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
	token := "token-multiple"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockFcmClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(nil)
	}

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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
			Value: []byte(`{"to":"` + fmt.Sprintf("%s%d", token, i) + `", "data": {"alert": "Hello Multiple"}}`),
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

func (s *FcmE2ETestSuite) TestDuplicatedMessages() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.config.GCM.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(gcmTopicTemplate, appName)})

	mockFcmClient, statsdClientMock := s.setupFcmPusher(appName)

	notificationsToSend := 2
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	hostname, _ := os.Hostname()
	topic := fmt.Sprintf(gcmTopicTemplate, appName)
	token := "token-duplicated"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockFcmClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(nil)
	}

	statsdClientMock.EXPECT().Count(
		"duplicated_messages",
		int64(1),
		[]string{fmt.Sprintf("hostname:%s", hostname), fmt.Sprintf("game:%s", appName), fmt.Sprintf("platform:%s", "gcm")},
		float64(1.0),
	).
		Times(1).
		DoAndReturn(func(metric_arg string, value_arg int64, tags_arg []string, rate_arg float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "gcm"), fmt.Sprintf("game:%s", appName), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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
			Value: []byte(`{"to":"` + token + `", "data": {"alert": "Hello Duplicated"}}`),
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
