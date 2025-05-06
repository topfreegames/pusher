package firebase

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/topfreegames/pusher/errors"
	mock_interfaces "github.com/topfreegames/pusher/mocks/interfaces"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"go.uber.org/mock/gomock"
)

const concurrentWorkers = 5

type MessageHandlerTestSuite struct {
	suite.Suite
	vConfig *viper.Viper
	config  *config.Config
	game    string

	mockClient           *mock_interfaces.MockPushClient
	mockStatsReporter    *mock_interfaces.MockStatsReporter
	mockFeedbackReporter *mock_interfaces.MockFeedbackReporter
	mockRateLimiter      *mock_interfaces.MockRateLimiter
	mockDedup            *mock_interfaces.MockDedup
	waitGroup            *sync.WaitGroup

	handler interfaces.MessageHandler
}

func TestMessageHandlerSuite(t *testing.T) {
	suite.Run(t, new(MessageHandlerTestSuite))
}

func (s *MessageHandlerTestSuite) SetupSuite() {
	file := os.Getenv("CONFIG_FILE")
	if file == "" {
		file = "../../config/test.yaml"
	}

	config, vConfig, err := config.NewConfigAndViper(file)
	s.Require().NoError(err)
	s.config = config
	s.vConfig = vConfig
	s.game = "game"
}

func (s *MessageHandlerTestSuite) SetupSubTest() {
	ctrl := gomock.NewController(s.T())
	s.mockClient = mock_interfaces.NewMockPushClient(ctrl)

	l, _ := test.NewNullLogger()

	s.mockStatsReporter = mock_interfaces.NewMockStatsReporter(ctrl)
	s.mockFeedbackReporter = mock_interfaces.NewMockFeedbackReporter(ctrl)
	statsClients := []interfaces.StatsReporter{s.mockStatsReporter}
	feedbackClients := []interfaces.FeedbackReporter{s.mockFeedbackReporter}
	s.mockRateLimiter = mock_interfaces.NewMockRateLimiter(ctrl)
	s.waitGroup = &sync.WaitGroup{}

	cfg := newDefaultMessageHandlerConfig()
	cfg.concurrentResponseHandlers = concurrentWorkers
	handler := NewMessageHandler(
		s.game,
		s.mockClient,
		feedbackClients,
		statsClients,
		s.mockRateLimiter,
		s.mockDedup,
		s.waitGroup,
		l,
		concurrentWorkers,
	)

	s.handler = handler
}

func (s *MessageHandlerTestSuite) TestHandleMessage() {
	s.Run("should fail if invalid kafka message format", func() {
		msg := interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte(`not json`),
		}

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)

		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should fail if notification expired", func() {
		message := interfaces.Message{}
		km := &kafkaFCMMessage{
			Message:    message,
			PushExpiry: extensions.MakeTimestamp() - time.Hour.Milliseconds(),
		}
		bytes, err := json.Marshal(km)
		s.Require().NoError(err)

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), interfaces.KafkaMessage{Value: bytes})

		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should fail if rate limit reached", func() {
		expiration := time.Now().Add(1 * time.Hour).UnixNano()
		token := uuid.NewString()
		msg := interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Game:  s.game,
			Value: []byte(fmt.Sprintf(`{"To": "%s", "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "expiration": 0 }, "push_expiry": %d }`,
				token,
				expiration,
			)),
		}

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, string(msg.Value), s.game, "gcm").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "gcm").
			Return(false)

		s.mockStatsReporter.EXPECT().
			NotificationRateLimitReached(s.game, "gcm").
			Return()

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)
		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should fail if message is not unique", func() {
		expiration := time.Now().Add(1 * time.Hour).UnixNano()
		token := uuid.NewString()
		msg := interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Game:  s.game,
			Value: []byte(fmt.Sprintf(`{"To": "%s", "Payload": { "aps" : { "alert" : "Hello HTTP/2" } }, "Metadata": { "expiration": 0 }, "push_expiry": %d }`,
				token,
				expiration,
			)),
		}
		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, string(msg.Value), s.game, "gcm").
			Return(false)

		s.mockStatsReporter.EXPECT().
			ReportMetricCount("duplicated_messages", int64(1), s.game, "apns").
			Return()

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "gcm").
			Return(true)
		
		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)
		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should succeed", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "body",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, string(msg.Value), s.game, "gcm").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "gcm").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, msg interfaces.Message) {
				s.Equal(token, msg.To)
				done <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "gcm", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "gcm").
			Return()

		s.handler.HandleMessages(context.Background(), msg)
		timeout := time.NewTimer(10 * time.Millisecond)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for message to be sent")
		}
	})

	s.Run("should not lock sendPushConcurrencyControl when sending multiple messages", func() {
		newMessage := func() kafkaFCMMessage {
			ttl := uint(0)
			token := uuid.NewString()
			title := fmt.Sprintf("title - %s", uuid.NewString())
			metadata := map[string]interface{}{
				"some":     "metadata",
				"game":     "game",
				"platform": "gcm",
			}
			km := kafkaFCMMessage{
				Message: interfaces.Message{
					TimeToLive:               &ttl,
					DeliveryReceiptRequested: false,
					DryRun:                   true,
					To:                       token,
					Data: map[string]interface{}{
						"title": title,
					},
				},
				Metadata:   metadata,
				PushExpiry: extensions.MakeTimestamp() + int64(1000000),
			}
			return km
		}
		go s.handler.HandleResponses()
		qtyMsgs := 100

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), gomock.Any(), gomock.Any(), s.game, "gcm").
			Return(true).
			Times(qtyMsgs)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), gomock.Any(), s.game, "gcm").
			Return(true).
			Times(qtyMsgs)

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(qtyMsgs)

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "gcm", gomock.Any()).
			Times(qtyMsgs).
			Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "gcm").
			Times(qtyMsgs).
			Return()

		done := make(chan struct{})
		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.game, "gcm").
			Times(qtyMsgs).
			Do(func(game, platform string) {
				done <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return().
			Times(qtyMsgs)

		ctx := context.Background()
		for i := 0; i < qtyMsgs; i++ {
			km := newMessage()
			bytes, err := json.Marshal(km)
			s.Require().NoError(err)
			s.waitGroup.Add(1)
			go s.handler.HandleMessages(ctx, interfaces.KafkaMessage{Value: bytes, Game: s.game})
		}

		timeout := time.NewTimer(50 * time.Millisecond)
		for i := 0; i < qtyMsgs; i++ {
			select {
			case <-done:
			case <-timeout.C:
				s.FailNow("timed out waiting for message to be sent")
			}
		}
	})
}

func (s *MessageHandlerTestSuite) TestHandleResponse() {
	s.Run("should send metric and feedback on failure", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "body",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, string(msg.Value), s.game, "gcm").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "gcm").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(errors.NewPushError("DEVICE_UNREGISTERED", "device unregistered"))

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "gcm", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "gcm").
			Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationFailure(s.game, "gcm", gomock.Any())

		s.mockFeedbackReporter.EXPECT().
			SendFeedback(s.game, "gcm", gomock.Any()).
			DoAndReturn(func(game, platform string, feedback []byte) {
				obj := &FeedbackResponse{}
				err := json.Unmarshal(feedback, obj)
				s.NoError(err)
				s.Equal(token, obj.From)
				done <- struct{}{}
			})

		go s.handler.HandleResponses()

		s.waitGroup.Add(1)

		s.handler.HandleMessages(context.Background(), msg)

		timeout := time.NewTimer(10 * time.Millisecond)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for message to be sent")
		}
	})

	s.Run("should send ack metric on success", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "body",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, string(msg.Value), s.game, "gcm").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "gcm").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, msg interfaces.Message) {
				s.Equal(token, msg.To)
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "gcm", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "gcm").
			Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.game, "gcm").
			Do(func(game, platform string) {
				done <- struct{}{}
			})

		go s.handler.HandleResponses()

		s.waitGroup.Add(1)

		s.handler.HandleMessages(context.Background(), msg)

		timeout := time.NewTimer(10 * time.Millisecond)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for message to be sent")
		}
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
