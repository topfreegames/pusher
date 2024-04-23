package e2e

import (
	"context"
	"fmt"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/sideshow/apns2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	mocks "github.com/topfreegames/pusher/mocks/interfaces"
	"github.com/topfreegames/pusher/pusher"
	"github.com/topfreegames/pusher/structs"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

type ApnsE2ETestSuite struct {
	suite.Suite

	statsdClientMock         *mocks.MockStatsDClient
	config                   *config.Config
	listenerStatsdClientMock *mocks.MockStatsDClient
	mockApnsClient           *mocks.MockAPNSPushQueue
	responsesChannel         chan *structs.ResponseWithMetadata
	stop                     context.CancelFunc
}

func TestApnsE2eSuite(t *testing.T) {
	suite.Run(t, new(ApnsE2ETestSuite))
}

func (s *ApnsE2ETestSuite) SetupTest() {
	c, v, err := config.NewConfigAndViper("../config/e2e.yaml")
	s.Require().NoError(err)
	s.config = c
	s.responsesChannel = make(chan *structs.ResponseWithMetadata)

	ctrl := gomock.NewController(s.T())
	s.mockApnsClient = mocks.NewMockAPNSPushQueue(ctrl)
	s.mockApnsClient.EXPECT().ResponseChannel().Return(s.responsesChannel)

	s.statsdClientMock = mocks.NewMockStatsDClient(ctrl)
	s.listenerStatsdClientMock = mocks.NewMockStatsDClient(ctrl)
	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	apnsPusher, err := pusher.NewAPNSPusher(false, v, logger, s.statsdClientMock, nil, s.mockApnsClient)
	s.Require().NoError(err)

	ctx := context.Background()
	ctx, s.stop = context.WithCancel(ctx)
	go apnsPusher.Start(ctx)

	time.Sleep(5 * time.Second)
}

func (s *ApnsE2ETestSuite) TearDownTest() {
	s.stop()
}

func (s *ApnsE2ETestSuite) TestSimpleNotification() {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := "push-" + app + "_apns-single"
	token := "token"
	done := make(chan bool)
	s.mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *apns2.Notification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				s.responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:      notification.ApnsID,
					Sent:        true,
					StatusCode:  200,
					DeviceToken: token,
				}
			}()
			return nil
		})

	s.statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	s.statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
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

	//Give it some time to process the message
	timeout := time.NewTimer(1 * time.Minute)
	select {
	case <-done:
		// Wait some time to make sure it won't call the push client again after the done signal
		time.Sleep(10 * time.Second)
	case <-timeout.C:
		s.Fail("Timeout waiting for Handler to report notification sent")
	}
}

func (s *ApnsE2ETestSuite) TestNotificationRetry() {
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := "push-" + app + "_apns-single"
	token := "token"
	done := make(chan bool)

	s.mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *apns2.Notification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				s.responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:      notification.ApnsID,
					Sent:        true,
					StatusCode:  429,
					Reason:      apns2.ReasonTooManyRequests,
					DeviceToken: token,
				}
			}()
			return nil
		})

	s.mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *apns2.Notification) error {
			s.Equal(token, notification.DeviceToken)
			s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

			go func() {
				s.responsesChannel <- &structs.ResponseWithMetadata{
					ApnsID:      notification.ApnsID,
					Sent:        true,
					StatusCode:  200,
					DeviceToken: token,
				}
			}()
			return nil
		})

	s.statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	s.statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			done <- true
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

	//Give it some time to process the message
	timeout := time.NewTimer(30 * time.Second)
	select {
	case <-done:
		// Wait some time to make sure it won't call the push client again after the done signal
		time.Sleep(10 * time.Second)
	case <-timeout.C:
		s.Fail("Timeout waiting for Handler to report notification sent")
	}
}

func (s *ApnsE2ETestSuite) TestMultipleNotificaions() {
	notificationsToSend := 1000
	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": s.config.Queue.Brokers,
	})
	s.Require().NoError(err)

	app := s.config.GetApnsAppsArray()[0]
	topic := "push-" + app + "_apns-single"
	token := "token"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		s.mockApnsClient.EXPECT().
			Push(gomock.Any()).
			DoAndReturn(func(notification *apns2.Notification) error {
				s.Equal(s.config.Apns.Certs[app].Topic, notification.Topic)

				go func() {
					s.responsesChannel <- &structs.ResponseWithMetadata{
						ApnsID:      notification.ApnsID,
						Sent:        true,
						StatusCode:  200,
						DeviceToken: notification.DeviceToken,
					}
				}()
				return nil
			})
	}

	s.statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	s.statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
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
	//Give it some time to process the message
	timeout := time.NewTimer(5 * time.Minute)
	for i := 0; i < notificationsToSend; i++ {
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("Timeout waiting for Handler to report notification sent")
		}
	}
	// Wait some time to make sure it won't call the push client again after everything is done
	time.Sleep(30 * time.Second)
}
