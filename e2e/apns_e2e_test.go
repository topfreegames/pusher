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
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	mocks "github.com/topfreegames/pusher/mocks/interfaces"
	"github.com/topfreegames/pusher/pusher"
	"github.com/topfreegames/pusher/structs"
	"go.uber.org/mock/gomock"
)

type ApnsE2ETestSuite struct {
	suite.Suite
	producer *kafka.Producer
}

func TestApnsE2eSuite(t *testing.T) {
	suite.Run(t, new(ApnsE2ETestSuite))
}

func (s *ApnsE2ETestSuite) SetupSuite() {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	c, _, err := config.NewConfigAndViper(configFile)
	s.Require().NoError(err)

	producer, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": c.Queue.Brokers,
	})
	s.Require().NoError(err)

	s.producer = producer
}

func (s *ApnsE2ETestSuite) setupApnsPusher() (
	string,
	*pusher.APNSPusher,
	*mocks.MockAPNSPushQueue,
	*mocks.MockStatsDClient,
	chan *structs.ResponseWithMetadata,
) {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	cfg, viper, err := config.NewConfigAndViper(configFile)
	s.Require().NoError(err)

	responsesChannel := make(chan *structs.ResponseWithMetadata)

	ctrl := gomock.NewController(s.T())
	mockApnsClient := mocks.NewMockAPNSPushQueue(ctrl)
	mockApnsClient.EXPECT().ResponseChannel().Return(responsesChannel)

	statsdClientMock := mocks.NewMockStatsDClient(ctrl)
	// Gauge can be called any times from the go stats report
	statsdClientMock.EXPECT().Gauge(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	logger := logrus.New()
	logger.Level = logrus.DebugLevel

	appName := strings.Split(uuid.NewString(), "-")[0]
	cfg.Apns.Apps = appName
	viper.Set("queue.topics", []string{fmt.Sprintf(apnsTopicTemplate, appName)})
	viper.Set("queue.group", appName)

	s.assureTopicsExist(appName)
	time.Sleep(wait)

	apnsPusher, err := pusher.NewAPNSPusher(false, viper, cfg, logger, statsdClientMock, nil, mockApnsClient)
	s.Require().NoError(err)

	return appName, apnsPusher, mockApnsClient, statsdClientMock, responsesChannel
}

func (s *ApnsE2ETestSuite) TestSimpleNotification() {
	app, p, mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	go p.Start(context.Background())
	time.Sleep(wait)

	hostname, _ := os.Hostname()
	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	testDone := make(chan bool)
	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)

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

	statsdClientMock.EXPECT().Count(
			"duplicated_messages", 
			int64(1),              
			[]string{fmt.Sprintf("hostname:%s", hostname), fmt.Sprintf("game:%s", app), fmt.Sprintf("platform:%s", "apns")},         
			float64(1.0),         
		).
		DoAndReturn(func(metric_arg string, value_arg int64, tags_arg []string, rate_arg float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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

	err := s.producer.Produce(&kafka.Message{
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
	app, p, mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	go p.Start(context.Background())
	time.Sleep(wait)

	hostname, _ := os.Hostname()
	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	done := make(chan bool)

	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)

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

	statsdClientMock.EXPECT().Count(
			"duplicated_messages", 
			int64(1),              
			[]string{fmt.Sprintf("hostname:%s", hostname), fmt.Sprintf("game:%s", app), fmt.Sprintf("platform:%s", "apns")},         
			float64(1.0),         
		).
		DoAndReturn(func(metric_arg string, value_arg int64, tags_arg []string, rate_arg float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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

	err := s.producer.Produce(&kafka.Message{
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
	app, p, mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	go p.Start(context.Background())
	time.Sleep(wait)

	hostname, _ := os.Hostname()
	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	done := make(chan bool)

	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		Times(3).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)

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

	statsdClientMock.EXPECT().Count(
			"duplicated_messages", 
			int64(1),              
			[]string{fmt.Sprintf("hostname:%s", hostname), fmt.Sprintf("game:%s", app), fmt.Sprintf("platform:%s", "apns")},         
			float64(1.0),         
		).
		DoAndReturn(func(metric_arg string, value_arg int64, tags_arg []string, rate_arg float64) error {
			return nil
		})
		
	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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

	err := s.producer.Produce(&kafka.Message{
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
	app, p, mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	go p.Start(context.Background())
	time.Sleep(wait)

	hostname, _ := os.Hostname()
	notificationsToSend := 10
	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockApnsClient.EXPECT().
			Push(gomock.Any()).
			DoAndReturn(func(notification *structs.ApnsNotification) error {

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

	statsdClientMock.EXPECT().Count(
			"duplicated_messages", 
			int64(1),              
			[]string{fmt.Sprintf("hostname:%s", hostname), fmt.Sprintf("game:%s", app), fmt.Sprintf("platform:%s", "apns")},         
			float64(1.0),         
		).
		Times(notificationsToSend).
		DoAndReturn(func(metric_arg string, value_arg int64, tags_arg []string, rate_arg float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), fmt.Sprintf("topic:%s", topic)}, float64(1)).
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
		err := s.producer.Produce(&kafka.Message{
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

func (s *ApnsE2ETestSuite) TestConsumeMessagesBeforeExiting() {
	app, p, mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	c := context.Background()
	ctx, cancel := context.WithCancel(c)
	go p.Start(ctx)
	time.Sleep(wait)

	notificationsToSend := 30

	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	done := make(chan bool)

	for i := 0; i < notificationsToSend; i++ {
		mockApnsClient.EXPECT().
			Push(gomock.Any()).
			DoAndReturn(func(notification *structs.ApnsNotification) error {
				// Simulate a delay in the push to make sure it will not have finished during the graceful shutdown
				time.Sleep(50 * time.Millisecond)
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
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), fmt.Sprintf("topic:%s", topic)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		Times(notificationsToSend).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(notificationsToSend).
		Return(nil)

	for i := 0; i < notificationsToSend; i++ {
		err := s.producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &topic,
				Partition: kafka.PartitionAny,
			},
			Value: []byte(`{"deviceToken":"` + fmt.Sprintf("%s%d", token, i) + `", "payload": {"aps": {"alert": "Hello"}}}`),
		},
			nil)
		s.Require().NoError(err)
	}

	// Give it some time to pull messages from Kafka
	time.Sleep(100 * time.Millisecond)
	cancel()

	wg := p.Queue.PendingMessagesWaitGroup()
	go func() {
		wg.Wait()
		done <- true
	}()

	timer := time.NewTimer(timeout)
	select {
	case <-done:
	case <-timer.C:
		s.FailNow("Did not consume all messages before exiting")
		return
	}
	// Wait some time to make sure it won't call the push client again after everything is done
	time.Sleep(wait)
}

func (s *ApnsE2ETestSuite) TestConsumeMessagesBeforeExitingWithRetries() {
	app, p, mockApnsClient, statsdClientMock, responsesChannel := s.setupApnsPusher()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	go p.Start(ctx)
	time.Sleep(wait)

	topic := fmt.Sprintf(apnsTopicTemplate, app)
	token := "token"
	done := make(chan bool)

	mockApnsClient.EXPECT().
		Push(gomock.Any()).
		Times(2).
		DoAndReturn(func(notification *structs.ApnsNotification) error {
			s.Equal(token, notification.DeviceToken)
			// Simulate a delay in the push to make sure it will not have finished during the graceful shutdown
			time.Sleep(50 * time.Millisecond)
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
			// Simulate a delay in the push to make sure it will not have finished during the graceful shutdown
			time.Sleep(50 * time.Millisecond)
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
		Incr("sent", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app), fmt.Sprintf("topic:%s", topic)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			return nil
		})

	statsdClientMock.EXPECT().
		Incr("ack", []string{fmt.Sprintf("platform:%s", "apns"), fmt.Sprintf("game:%s", app)}, float64(1)).
		DoAndReturn(func(string, []string, float64) error {
			//done <- true
			return nil
		})
	statsdClientMock.EXPECT().
		Timing("send_notification_latency", gomock.Any(), gomock.Any(), gomock.Any()).
		Times(3).
		Return(nil)

	err := s.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: []byte(`{"deviceToken":"` + token + `", "payload": {"aps": {"alert": "Hello"}}}`),
	},
		nil)
	s.Require().NoError(err)

	// Give it some time to pull messages from Kafka
	time.Sleep(100 * time.Millisecond)
	cancel()

	wg := p.Queue.PendingMessagesWaitGroup()
	go func() {
		wg.Wait()
		done <- true
	}()

	timer := time.NewTimer(timeout)
	select {
	case <-done:
	case <-timer.C:
		s.FailNow("Timeout waiting for Handler to report notification sent")
	}
	// Wait some time to make sure it won't call the push client again after everything is done
	time.Sleep(wait)
}

func (s *ApnsE2ETestSuite) assureTopicsExist(app string) {

	topic := fmt.Sprintf(apnsTopicTemplate, app)
	err := s.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value: []byte("not a notification"),
	},
		nil)
	s.Require().NoError(err)
}