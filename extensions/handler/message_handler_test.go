package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	pushererrors "github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
	mock_interfaces "github.com/topfreegames/pusher/mocks/firebase"
	"go.uber.org/mock/gomock"
)

const concurrentWorkers = 5

type MessageHandlerTestSuite struct {
	suite.Suite
	vConfig *viper.Viper
	config  *config.Config
	game    string

	mockClient        *mock_interfaces.MockPushClient
	mockStatsdClient  *mocks.StatsDClientMock
	mockKafkaProducer *mocks.KafkaProducerClientMock

	handler *messageHandler
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

	s.mockStatsdClient = mocks.NewStatsDClientMock()
	statsD, err := extensions.NewStatsD(s.vConfig, l, s.mockStatsdClient)
	s.Require().NoError(err)

	s.mockKafkaProducer = mocks.NewKafkaProducerClientMock()
	kc, err := extensions.NewKafkaProducer(s.vConfig, l, s.mockKafkaProducer)
	s.Require().NoError(err)

	statsClients := []interfaces.StatsReporter{statsD}
	feedbackClients := []interfaces.FeedbackReporter{kc}

	handler := &messageHandler{
		app:                        s.game,
		client:                     s.mockClient,
		feedbackReporters:          feedbackClients,
		statsReporters:             statsClients,
		logger:                     l,
		config:                     newDefaultMessageHandlerConfig(),
		sendPushConcurrencyControl: make(chan interface{}, concurrentWorkers),
		responsesChannel: make(chan struct {
			msg   interfaces.Message
			error error
		}, concurrentWorkers),
	}

	for i := 0; i < concurrentWorkers; i++ {
		handler.sendPushConcurrencyControl <- struct{}{}
	}

	s.NoError(err)
	s.Require().NotNil(handler)

	s.handler = handler
}

func (s *MessageHandlerTestSuite) TestSendMessage() {
	ctx := context.Background()
	s.Run("should do nothing for bad message format", func() {
		message := interfaces.KafkaMessage{
			Value: []byte("bad message"),
		}
		s.handler.HandleMessages(ctx, message)

		s.handler.statsMutex.Lock()
		s.Equal(int64(0), s.handler.stats.sent)
		s.Equal(int64(0), s.handler.stats.failures)
		s.Equal(int64(0), s.handler.stats.ignored)
		s.handler.statsMutex.Unlock()
	})

	s.Run("should ignore message if it has expired", func() {
		message := interfaces.Message{}
		km := &extensions.KafkaGCMMessage{
			Message:    message,
			PushExpiry: extensions.MakeTimestamp() - time.Hour.Milliseconds(),
		}
		bytes, err := json.Marshal(km)
		s.Require().NoError(err)

		s.handler.HandleMessages(ctx, interfaces.KafkaMessage{Value: bytes})
		s.handler.statsMutex.Lock()
		s.Equal(int64(1), s.handler.stats.ignored)
		s.handler.statsMutex.Unlock()
	})

	s.Run("should report failure if cannot send message", func() {
		ttl := uint(0)
		token := "token"
		metadata := map[string]interface{}{
			"some":     "metadata",
			"game":     "game",
			"platform": "gcm",
		}
		km := &extensions.KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       token,
				Data: map[string]interface{}{
					"title": "notification",
				},
			},
			Metadata:   metadata,
			PushExpiry: extensions.MakeTimestamp() + int64(1000000),
		}

		expected := interfaces.Message{
			TimeToLive:               &ttl,
			DeliveryReceiptRequested: false,
			DryRun:                   true,
			To:                       token,
			Data: map[string]interface{}{
				"title":    "notification",
				"some":     "metadata",
				"game":     "game",
				"platform": "gcm",
			},
		}

		bytes, err := json.Marshal(km)
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), expected).
			Return(pushererrors.NewPushError("INVALID_TOKEN", "invalid token"))

		s.handler.HandleMessages(ctx, interfaces.KafkaMessage{Value: bytes})
		s.handler.HandleResponses()

		select {
		case m := <-s.mockKafkaProducer.ProduceChannel():
			val := &FeedbackResponse{}
			err = json.Unmarshal(m.Value, val)
			s.NoError(err)
			s.Equal("INVALID_TOKEN", val.Error)
		case <-time.After(time.Second * 1):
			s.Fail("did not send feedback to kafka")
		}

		s.handler.statsMutex.Lock()
		s.Equal(int64(1), s.handler.stats.failures)
		s.handler.statsMutex.Unlock()

		s.Equal(int64(1), s.mockStatsdClient.Counts["failed"])
	})

	s.Run("should report sent and success if message was sent", func() {
		ttl := uint(0)
		token := "token"
		metadata := map[string]interface{}{
			"some":     "metadata",
			"game":     "game",
			"platform": "gcm",
		}
		km := &extensions.KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       token,
				Data: map[string]interface{}{
					"title": "notification",
				},
			},
			Metadata:   metadata,
			PushExpiry: extensions.MakeTimestamp() + int64(1000000),
		}

		expected := interfaces.Message{
			TimeToLive:               &ttl,
			DeliveryReceiptRequested: false,
			DryRun:                   true,
			To:                       token,
			Data: map[string]interface{}{
				"title":    "notification",
				"some":     "metadata",
				"game":     "game",
				"platform": "gcm",
			},
		}

		bytes, err := json.Marshal(km)
		s.Require().NoError(err)

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), expected).
			Return(nil)

		s.handler.HandleMessages(ctx, interfaces.KafkaMessage{Value: bytes})
		s.handler.HandleResponses()

		select {
		case m := <-s.mockKafkaProducer.ProduceChannel():
			val := &FeedbackResponse{}
			err = json.Unmarshal(m.Value, val)
			s.NoError(err)
			s.Empty(val.Error)
		case <-time.After(time.Second * 1):
			s.Fail("did not send feedback to kafka")
		}

		s.handler.statsMutex.Lock()
		s.Equal(int64(1), s.handler.stats.sent)
		s.handler.statsMutex.Unlock()
		s.Equal(int64(1), s.mockStatsdClient.Counts["sent"])
		s.Equal(int64(1), s.mockStatsdClient.Counts["ack"])
	})

	s.Run("should not lock sendPushConcurrencyControl when sending multiple messages", func() {
		newMessage := func() extensions.KafkaGCMMessage {
			ttl := uint(0)
			token := uuid.NewString()
			title := fmt.Sprintf("title - %s", uuid.NewString())
			metadata := map[string]interface{}{
				"some":     "metadata",
				"game":     "game",
				"platform": "gcm",
			}

			km := extensions.KafkaGCMMessage{
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

		qtyMsgs := 20

		s.mockClient.EXPECT().
			SendPush(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(qtyMsgs)

		for i := 0; i < qtyMsgs; i++ {
			km := newMessage()
			bytes, err := json.Marshal(km)
			s.Require().NoError(err)

			go s.handler.HandleMessages(ctx, interfaces.KafkaMessage{Value: bytes})
		}

		for i := 0; i < qtyMsgs; i++ {
			select {
			case m := <-s.mockKafkaProducer.ProduceChannel():
				val := &FeedbackResponse{}
				err := json.Unmarshal(m.Value, val)
				s.NoError(err)
				s.Empty(val.Error)
			case <-time.After(time.Second * 1):
				s.Fail("did not send feedback to kafka")
			}
		}

		time.Sleep(2 * time.Second)

		s.handler.statsMutex.Lock()
		s.Equal(int64(20), s.handler.stats.sent)
		s.handler.statsMutex.Unlock()
		s.Equal(int64(20), s.mockStatsdClient.Counts["sent"])
		s.Equal(int64(20), s.mockStatsdClient.Counts["ack"])
	})
}
