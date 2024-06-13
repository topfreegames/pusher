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
	"github.com/sideshow/apns2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	mocks "github.com/topfreegames/pusher/mocks/interfaces"
	"github.com/topfreegames/pusher/pusher"
	"github.com/topfreegames/pusher/structs"
	"go.uber.org/mock/gomock"
)

type ApnsE2ETestSuite struct {
	suite.Suite

	config  *config.Config
	vConfig *viper.Viper
}

func TestApnsE2eSuite(t *testing.T) {
	suite.Run(t, new(ApnsE2ETestSuite))
}

func (s *ApnsE2ETestSuite) SetupSuite() {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	c, v, err := config.NewConfigAndViper(configFile)
	s.Require().NoError(err)
	s.config = c
	s.vConfig = v
}

func (s *ApnsE2ETestSuite) setupApnsPusher() (*mocks.MockAPNSPushQueue, *mocks.MockStatsDClient, chan *structs.ResponseWithMetadata) {
	responsesChannel := make(chan *structs.ResponseWithMetadata)

	ctrl := gomock.NewController(s.T())
	mockApnsClient := mocks.NewMockAPNSPushQueue(ctrl)
	mockApnsClient.EXPECT().ResponseChannel().Return(responsesChannel)

	statsdClientMock := mocks.NewMockStatsDClient(ctrl)
	// Gauge can be called any times from the go stats report
	statsdClientMock.EXPECT().Gauge(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	s.assureTopicsExist()
	time.Sleep(wait)

	apnsPusher, err := pusher.NewAPNSPusher(false, s.vConfig, s.config, logger, statsdClientMock, nil, mockApnsClient)
	s.Require().NoError(err)
	ctx := context.Background()
	go apnsPusher.Start(ctx)

	time.Sleep(wait * 3)

	return mockApnsClient, statsdClientMock, responsesChannel
}

func (s *ApnsE2ETestSuite) TestSimpleNotification() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.config.Apns.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(apnsTopicTemplate, appName)})

	mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := "push-" + app + "_apns-single"
	token := "token"
	testDone := make(chan bool)
	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:       notification.ApnsID,
					Sent:         true,
					StatusCode:   200,
					DeviceToken:  token,
					Notification: notification,
				}
			}()
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			testDone <- true
			return nil
		})

	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

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

func (s *ApnsE2ETestSuite) TestNotificationRetry() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.config.Apns.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(apnsTopicTemplate, appName)})

	mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()

	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := "push-" + app + "_apns-single"
	token := "token"
	done := make(chan bool)

	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:       notification.ApnsID,
					Sent:         true,
					StatusCode:   429,
					Reason:       apns2.ReasonTooManyRequests,
					DeviceToken:  token,
					Notification: notification,
				}
			}()
			return nil
		})

	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:       notification.ApnsID,
					Sent:         true,
					StatusCode:   200,
					DeviceToken:  token,
					Notification: notification,
				}
			}()
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
			return nil
		})
	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(2).
		Return(nil)

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
	case <-done:
		// Wait some time to make sure it won't call the push client again after the done signal
		time.Sleep(wait)
	case <-timer.C:
		s.FailNow("Timeout waiting for Handler to report notification sent")
	}
}

func (s *ApnsE2ETestSuite) TestRetryLimit() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.config.Apns.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(apnsTopicTemplate, appName)})

	mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()

	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := "push-" + app + "_apns-single"
	token := "token"
	done := make(chan bool)

	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		Times(3).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:       notification.ApnsID,
					Sent:         true,
					StatusCode:   429,
					Reason:       apns2.ReasonTooManyRequests,
					DeviceToken:  token,
					Notification: notification,
				}
			}()
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("failed", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), "reason:too-many-requests"}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
			return nil
		})
	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(3).
		Return(nil)

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
	case <-done:
		// Wait some time to make sure it won't call the push client again after the done signal
		time.Sleep(wait)
	case <-timer.C:
		s.FailNow("Timeout waiting for Handler to report notification sent")
	}
}

func (s *ApnsE2ETestSuite) TestMultipleNotifications() {
	appName := strings.Split(uuid.NewString(), "-")[0]
	s.config.Apns.Apps = appName
	s.vConfig.Set("queue.topics", []string{fmt.Sprintf(apnsTopicTemplate, appName)})

	mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()

	notificationsToSend := 10
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockApnsClient.EXPECT().
			Push(gomock.Any()).
			DoAndReturn(func(notification *structs.ApnsNotification) error {
				s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

				go func() {
					responsesChannel <- &structs.ResponseWithMetadata{
						ApnsID:       notification.ApnsID,
						Sent:         true,
						StatusCode:   200,
						DeviceToken:  notification.DeviceToken,
						Notification: notification,
					}
				}()
				return nil
			})
	}

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
			return nil
		})

	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(notificationsToSend).
		Return(nil)

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

func (s *ApnsE2ETestSuite) assureTopicsExist() {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	apnsApps := s.config.GetApnsAppsArray()
	for _, a := range apnsApps {
		topic := fmt.Sprintf(apnsTopicTemplate, a)
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
