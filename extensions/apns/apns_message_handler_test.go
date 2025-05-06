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

package apns

import (
	"context"
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/sideshow/apns2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/errors"
	mock_interfaces "github.com/topfreegames/pusher/mocks/interfaces"
	"go.uber.org/mock/gomock"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/structs"
	"github.com/topfreegames/pusher/util"
)

type ApnsMessageHandlerTestSuite struct {
	suite.Suite

	config       *viper.Viper
	authKeyPath  string
	keyID        string
	teamID       string
	topic        string
	appName      string
	isProduction bool
	logger       *logrus.Logger

	mockApnsPushQueue    *mock_interfaces.MockAPNSPushQueue
	mockStatsReporter    *mock_interfaces.MockStatsReporter
	mockFeedbackReporter *mock_interfaces.MockFeedbackReporter
	mockRateLimiter      *mock_interfaces.MockRateLimiter
	mockDedup            *mock_interfaces.MockDedup
	waitGroup            *sync.WaitGroup
	handler              *APNSMessageHandler
}

func TestApnsMessageHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(ApnsMessageHandlerTestSuite))
}

func (s *ApnsMessageHandlerTestSuite) SetupSuite() {
	cfg, err := util.NewViperWithConfigFile("../../config/test.yaml")
	require.NoError(s.T(), err)

	s.config = cfg
	s.authKeyPath = "../tls/authkey.p8"
	s.keyID = "ABC123DEFG"
	s.teamID = "DEF123GHIJ"
	s.topic = "com.game.test"
	s.appName = "game"
	s.isProduction = false
	s.logger, _ = test.NewNullLogger()
	s.logger.Level = logrus.DebugLevel
}

func (s *ApnsMessageHandlerTestSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())

	mockFeedbackReporter := mock_interfaces.NewMockFeedbackReporter(ctrl)
	mockStatsReporter := mock_interfaces.NewMockStatsReporter(ctrl)
	statsClients := []interfaces.StatsReporter{mockStatsReporter}
	feedbackClients := []interfaces.FeedbackReporter{mockFeedbackReporter}

	mockPushQueue := mock_interfaces.NewMockAPNSPushQueue(ctrl)
	mockRateLimiter := mock_interfaces.NewMockRateLimiter(ctrl)
	mockDedup := mock_interfaces.NewMockDedup(ctrl)
	wg := &sync.WaitGroup{}

	handler, err := NewAPNSMessageHandler(
		s.authKeyPath,
		s.keyID,
		s.teamID,
		s.topic,
		s.appName,
		s.isProduction,
		s.config,
		s.logger,
		wg,
		statsClients,
		feedbackClients,
		mockPushQueue,
		mockRateLimiter,
		mockDedup,
	)
	require.NoError(s.T(), err)

	s.handler = handler
	s.mockApnsPushQueue = mockPushQueue
	s.mockStatsReporter = mockStatsReporter
	s.mockFeedbackReporter = mockFeedbackReporter
	s.waitGroup = wg
	s.mockRateLimiter = mockRateLimiter
	s.mockDedup = mockDedup
}

func (s *ApnsMessageHandlerTestSuite) SetupSubTest() {
	s.SetupTest()
}

func (s *ApnsMessageHandlerTestSuite) TestHandleMessage() {
	s.Run("should fail if invalid kafka message format", func() {
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(`not json`),
		}

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)

		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should fail if notification expired", func() {
		expiration := time.Now().Add(-1 * time.Hour).Unix()
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(fmt.Sprintf(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "expiration": 0 }, "push_expiry": %d }`, expiration)),
		}

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), gomock.Any(), gomock.Any(), s.appName, "apns").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), gomock.Any(), s.appName, "apns").
			Return(true)

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)

		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should fail if rate limit reached", func() {
		expiration := time.Now().Add(1 * time.Hour).UnixNano()
		token := uuid.NewV4().String()
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(fmt.Sprintf(`{"DeviceToken": "%s", "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "expiration": 0 }, "push_expiry": %d }`,
				token,
				expiration,
			)),
		}

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, gomock.Any(), s.appName, "apns").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.appName, "apns").
			Return(false)

		s.mockStatsReporter.EXPECT().
			NotificationRateLimitReached(s.appName, "apns").
			Return()

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)
		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should fail open if finds duplicate message", func() {
		expiration := time.Now().Add(1 * time.Hour).UnixNano()
		token := uuid.NewV4().String()
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(fmt.Sprintf(`{"DeviceToken": "%s", "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "expiration": 0 }, "push_expiry": %d }`,
				token,
				expiration,
			)),
		}
		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, gomock.Any(), s.appName, "apns").
			Return(false)
			
		s.mockStatsReporter.EXPECT().
			ReportMetricCount("duplicated_messages", int64(1), s.appName, "apns").
			Return()

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.appName, "apns").
			Return(true)

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)
		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should succeed", func() {
		expiration := time.Now().Add(1 * time.Hour).UnixNano()
		token := uuid.NewV4().String()
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(fmt.Sprintf(`{"DeviceToken": "%s", "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "expiration": 0 }, "push_expiry": %d }`,
				token,
				expiration),
			),
		}

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.appName, "apns").
			Return(true)

		s.mockApnsPushQueue.EXPECT().
			Push(gomock.Any()).
			Do(func(n *structs.ApnsNotification) {
				assert.Equal(s.T(), s.topic, n.Topic)
				assert.Equal(s.T(), token, n.DeviceToken)
				assert.Equal(s.T(), 1, n.SendAttempts)
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.appName, "apns", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.appName, "apns").
			Return()

		s.handler.HandleMessages(context.Background(), msg)
	})

	s.Run("should succeed and send metadata on notification", func() {
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "some": "data" }}`),
		}

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), gomock.Any(), s.appName, "apns").
			Return(true)

		s.mockApnsPushQueue.EXPECT().
			Push(gomock.Any()).
			Do(func(n *structs.ApnsNotification) {
				bytes, ok := n.Notification.Payload.([]byte)
				assert.True(s.T(), ok)

				var payload map[string]interface{}
				err := json.Unmarshal(bytes, &payload)
				assert.NoError(s.T(), err)

				metadata, ok := payload["M"].(map[string]interface{})
				assert.True(s.T(), ok)
				assert.Equal(s.T(), "data", metadata["some"])
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.appName, "apns", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.appName, "apns").
			Return()

		s.handler.HandleMessages(context.Background(), msg)
	})

	s.Run("should succeed and merge metadata on notification", func() {
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" }, "M": { "previous": "value" }}, "Metadata": { "some": "data" } }`),
		}

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), gomock.Any(), s.appName, "apns").
			Return(true)

		s.mockApnsPushQueue.EXPECT().
			Push(gomock.Any()).
			Do(func(n *structs.ApnsNotification) {
				bytes, ok := n.Notification.Payload.([]byte)
				assert.True(s.T(), ok)

				var payload map[string]interface{}
				err := json.Unmarshal(bytes, &payload)
				assert.NoError(s.T(), err)

				metadata, ok := payload["M"].(map[string]interface{})
				assert.True(s.T(), ok)
				assert.Equal(s.T(), "value", metadata["previous"])
				assert.Equal(s.T(), "data", metadata["some"])
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.appName, "apns", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.appName, "apns").
			Return()

		s.handler.HandleMessages(context.Background(), msg)
	})

	s.Run("should succeed and handle nested metadata on notification", func() {
		msg := interfaces.KafkaMessage{
			Topic: "push-game_apns",
			Value: []byte(`{ "Payload": { "aps" : { "alert" : "Hello HTTP/2" }}, "Metadata": { "nested": { "some": "data" }}}`),
		}

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), gomock.Any(), gomock.Any(), s.appName, "apns").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), gomock.Any(), s.appName, "apns").
			Return(true)

		s.mockApnsPushQueue.EXPECT().
			Push(gomock.Any()).
			Do(func(n *structs.ApnsNotification) {
				bytes, ok := n.Notification.Payload.([]byte)
				assert.True(s.T(), ok)

				var payload map[string]interface{}
				err := json.Unmarshal(bytes, &payload)
				assert.NoError(s.T(), err)

				metadata, ok := payload["M"].(map[string]interface{})
				assert.True(s.T(), ok)
				nested, ok := metadata["nested"].(map[string]interface{})
				assert.True(s.T(), ok)
				assert.Equal(s.T(), "data", nested["some"])
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.appName, "apns", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.appName, "apns").
			Return()

		s.handler.HandleMessages(context.Background(), msg)
	})
}

func (s *ApnsMessageHandlerTestSuite) TestResponseHandle() {
	s.Run("should send ack metric if response has no error", func() {
		apnsID := uuid.NewV4().String()
		res := &structs.ResponseWithMetadata{
			StatusCode: 200,
			ApnsID:     apnsID,
			Notification: &structs.ApnsNotification{
				Notification: apns2.Notification{
					ApnsID: apnsID,
				},
			},
		}

		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.appName, "apns")

		s.waitGroup.Add(1)

		err := s.handler.handleAPNSResponse(res)
		s.NoError(err)
		waitWG(s.T(), s.waitGroup)
	})

	for _, r := range errorReasons {
		if r == apns2.ReasonTooManyRequests {
			continue
		}
		s.Run(fmt.Sprintf("should send feedback and failure metric if response is %s", r), func() {
			apnsID := uuid.NewV4().String()
			res := &structs.ResponseWithMetadata{
				StatusCode: 400,
				ApnsID:     apnsID,
				Reason:     r,
				Notification: &structs.ApnsNotification{
					Notification: apns2.Notification{
						ApnsID: apnsID,
					},
				},
			}

			s.mockStatsReporter.EXPECT().
				HandleNotificationFailure(s.appName, "apns", errors.NewPushError(s.handler.mapErrorReason(res.Reason), res.Reason)).
				Return()

			s.mockFeedbackReporter.EXPECT().
				SendFeedback(s.appName, "apns", gomock.Any()).
				Do(func(game, platform string, feedback []byte) {
					var actualRes structs.ResponseWithMetadata
					err := json.Unmarshal(feedback, &actualRes)
					s.NoError(err)
					s.Equal(apnsID, res.ApnsID)

					if r == apns2.ReasonBadDeviceToken {
						deleteToken, ok := actualRes.Metadata["deleteToken"].(bool)
						s.True(ok)
						s.True(deleteToken)
					}
				})

			s.waitGroup.Add(1)

			err := s.handler.handleAPNSResponse(res)
			s.NoError(err)
			waitWG(s.T(), s.waitGroup)

		})
	}

	s.Run("should retry if TooManyRequests", func() {
		apnsID := uuid.NewV4().String()
		res := &structs.ResponseWithMetadata{
			StatusCode: 429,
			ApnsID:     apnsID,
			Reason:     apns2.ReasonTooManyRequests,
			Notification: &structs.ApnsNotification{
				Notification: apns2.Notification{
					ApnsID: apnsID,
				},
			},
		}

		s.mockApnsPushQueue.EXPECT().
			Push(res.Notification).
			Return()

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.appName, "apns", gomock.Any()).
			Return()

		err := s.handler.handleAPNSResponse(res)
		s.NoError(err)
	})
}

func waitWG(t *testing.T, wg *sync.WaitGroup) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	timeout := time.After(10 * time.Millisecond)
	select {
	case <-done:
	case <-timeout:
		t.Fatal("timed out waiting for waitgroup")
	}
}

var errorReasons = []string{
	apns2.ReasonPayloadEmpty,
	apns2.ReasonPayloadTooLarge,
	apns2.ReasonMissingDeviceToken,
	apns2.ReasonBadDeviceToken,
	apns2.ReasonTooManyRequests,
	apns2.ReasonBadMessageID,
	apns2.ReasonBadExpirationDate,
	apns2.ReasonBadPriority,
	apns2.ReasonBadTopic,
	apns2.ReasonBadCertificate,
	apns2.ReasonBadCertificateEnvironment,
	apns2.ReasonForbidden,
	apns2.ReasonMissingTopic,
	apns2.ReasonTopicDisallowed,
	apns2.ReasonUnregistered,
	apns2.ReasonDeviceTokenNotForTopic,
	apns2.ReasonDuplicateHeaders,
	apns2.ReasonBadPath,
	apns2.ReasonMethodNotAllowed,
	apns2.ReasonIdleTimeout,
	apns2.ReasonShutdown,
	apns2.ReasonInternalServerError,
	apns2.ReasonServiceUnavailable,
	apns2.ReasonExpiredProviderToken,
	apns2.ReasonInvalidProviderToken,
	apns2.ReasonMissingProviderToken,
}
