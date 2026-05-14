package firebase

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/topfreegames/pusher/errors"
	mock_interfaces "github.com/topfreegames/pusher/mocks/interfaces"

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
	s.mockDedup = mock_interfaces.NewMockDedup(ctrl)
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
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyRateLimit",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)

		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "gcm").
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

	s.Run("should fail open if message is not unique", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyDedup",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)

		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "gcm").
			Return(false)

		s.mockStatsReporter.EXPECT().
			ReportMetricCount("duplicated_messages", int64(1), s.game, "gcm").
			Return()

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
			HandleNotificationSent(s.game, "gcm", gomock.Any()).
			Return()

		s.handler.HandleMessages(context.Background(), msg)
		timeout := time.NewTimer(10 * time.Millisecond)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for message to be sent")
		}
	})

	s.Run("should succeed", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodySuccess",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}

		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "gcm").
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
			HandleNotificationSent(s.game, "gcm", "push-game_gcm").
			Do(func(game, platform, topic string) {
				done <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.game, "gcm").
			Return()

		go s.handler.HandleResponses()
		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)

		timeout := time.NewTimer(5 * time.Second)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for message to be processed")
		}
		s.waitGroup.Wait()
	})

	s.Run("should propagate ios platform through dedup, rate limiter, and stats reporters", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyIOS",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		msg := interfaces.KafkaMessage{
			Value:    bytes,
			Topic:    "push-game_ios-single",
			Game:     s.game,
			Platform: "ios",
		}

		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "ios").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "ios").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, m interfaces.Message) {
				s.Equal(token, m.To)
				s.Equal("ios", m.Platform)
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "ios", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "ios", "push-game_ios-single").
			Do(func(game, platform, topic string) {
				done <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.game, "ios").
			Return()

		go s.handler.HandleResponses()
		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)

		timeout := time.NewTimer(5 * time.Second)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for ios message to be processed")
		}
		s.waitGroup.Wait()
	})

	s.Run("should report ios platform on duplicate detected", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyIOSDedup",
				},
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		msg := interfaces.KafkaMessage{
			Value:    bytes,
			Topic:    "push-game_ios-single",
			Game:     s.game,
			Platform: "ios",
		}

		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "ios").
			Return(false)

		s.mockStatsReporter.EXPECT().
			ReportMetricCount("duplicated_messages", int64(1), s.game, "ios").
			Return()

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "ios").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, m interfaces.Message) {
				s.Equal(token, m.To)
				done <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "ios", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "ios", "push-game_ios-single").
			Return()

		s.handler.HandleMessages(context.Background(), msg)
		timeout := time.NewTimer(50 * time.Millisecond)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for ios dup-detected message to be sent")
		}
	})

	s.Run("should report ios platform on rate limit reached", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyIOSRateLimit",
				},
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		msg := interfaces.KafkaMessage{
			Value:    bytes,
			Topic:    "push-game_ios-single",
			Game:     s.game,
			Platform: "ios",
		}

		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "ios").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "ios").
			Return(false)

		s.mockStatsReporter.EXPECT().
			NotificationRateLimitReached(s.game, "ios").
			Return()

		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)
		waitWG(s.T(), s.waitGroup)
	})

	s.Run("should send ios feedback on failure", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyIOSFailure",
				},
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		msg := interfaces.KafkaMessage{
			Value:    bytes,
			Topic:    "push-game_ios-single",
			Game:     s.game,
			Platform: "ios",
		}

		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "ios").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "ios").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(errors.NewPushError("DEVICE_UNREGISTERED", "device unregistered"))

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "ios", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "ios", "push-game_ios-single").
			Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationFailure(s.game, "ios", gomock.Any())

		s.mockFeedbackReporter.EXPECT().
			SendFeedback(s.game, "ios", gomock.Any()).
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

		timeout := time.NewTimer(50 * time.Millisecond)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for ios failure feedback")
		}
	})

	s.Run("should default empty platform to gcm for backward compatibility", func() {
		token := uuid.NewString()
		msgValue := kafkaFCMMessage{
			Message: interfaces.Message{
				To: token,
				Data: map[string]interface{}{
					"title": "notification",
					"body":  "bodyEmptyPlatform",
				},
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		// Platform is intentionally left empty.
		msg := interfaces.KafkaMessage{
			Value: bytes,
			Topic: "push-game_gcm-single",
			Game:  s.game,
		}

		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "gcm").
			Return(true)

		s.mockRateLimiter.EXPECT().
			Allow(gomock.Any(), token, s.game, "gcm").
			Return(true)

		done := make(chan struct{})

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, m interfaces.Message) {
				s.Equal("gcm", m.Platform)
			})

		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "gcm", gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "gcm", "push-game_gcm-single").
			Do(func(game, platform, topic string) {
				done <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.game, "gcm").
			Return()

		go s.handler.HandleResponses()
		s.waitGroup.Add(1)
		s.handler.HandleMessages(context.Background(), msg)

		timeout := time.NewTimer(5 * time.Second)
		select {
		case <-done:
		case <-timeout.C:
			s.Fail("timed out waiting for empty-platform message to be processed")
		}
		s.waitGroup.Wait()
	})

	s.Run("should not lock sendPushConcurrencyControl when sending multiple messages", func() {
		newMessage := func() kafkaFCMMessage {
			token := uuid.NewString()
			title := uuid.NewString()
			ttl := uint(1000)
			metadata := map[string]interface{}{
				"some": "metadata",
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

		mockDone := make(chan struct{}, qtyMsgs)
		s.mockStatsReporter.EXPECT().
			ReportSendNotificationLatency(gomock.Any(), s.game, "gcm", gomock.Any()).
			Times(qtyMsgs).
			Do(func(latency time.Duration, game, platform string, labels ...string) {
				mockDone <- struct{}{}
			})

		s.mockStatsReporter.EXPECT().
			HandleNotificationSent(s.game, "gcm", gomock.Any()).
			Times(qtyMsgs).
			Return()

		s.mockStatsReporter.EXPECT().
			HandleNotificationSuccess(s.game, "gcm").
			Times(qtyMsgs).
			Return()

		s.mockStatsReporter.EXPECT().
			ReportFirebaseLatency(gomock.Any(), s.game, gomock.Any()).
			Times(qtyMsgs).
			Return()

		ctx := context.Background()
		for i := 0; i < qtyMsgs; i++ {
			km := newMessage()
			bytes, err := json.Marshal(km)
			s.Require().NoError(err)
			s.waitGroup.Add(1)
			go s.handler.HandleMessages(ctx, interfaces.KafkaMessage{Value: bytes, Game: s.game})
		}

		// Wait for all mock expectations to be met
		timeout := time.NewTimer(5 * time.Second)
		for i := 0; i < qtyMsgs; i++ {
			select {
			case <-mockDone:
			case <-timeout.C:
				s.FailNow("timed out waiting for all messages to be processed")
			}
		}

		// Wait for all goroutines to complete
		s.waitGroup.Wait()
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
					"body":  "bodySendMetricOnFailure",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)
		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "gcm").
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
			HandleNotificationSent(s.game, "gcm", "push-game_gcm").
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
					"body":  "bodyAckMetricOnSuccess",
				},
			},
			Metadata: map[string]interface{}{
				"some": "metadata",
			},
		}
		bytes, err := json.Marshal(msgValue)
		s.Require().NoError(err)

		msg := interfaces.KafkaMessage{Value: bytes, Topic: "push-game_gcm", Game: s.game}
		dedupMsg, err := createDedupContentForTest(msgValue)
		s.Require().NoError(err)

		s.mockDedup.EXPECT().
			IsUnique(gomock.Any(), token, dedupMsg, s.game, "gcm").
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
			HandleNotificationSent(s.game, "gcm", "push-game_gcm").
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
	t.Helper()
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

func createDedupContentForTest(km kafkaFCMMessage) (string, error) {
	contentData := make(map[string]interface{})

	if km.Data != nil {
		contentData["data"] = km.Data
	}

	if km.Message.Notification != nil {
		contentData["notification"] = km.Message.Notification
	}

	contentJSON, err := json.Marshal(contentData)
	if err != nil {
		return "", err
	}
	return string(contentJSON), nil
}
