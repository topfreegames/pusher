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

package extensions

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"

	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/topfreegames/go-gcm"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/mocks"
)

type GCMMessageHandlerTestSuite struct {
	suite.Suite

	config  *config.Config
	vConfig *viper.Viper
	game    string
}

func TestGCMMessageHandlerSuite(t *testing.T) {
	suite.Run(t, new(GCMMessageHandlerTestSuite))
}

func (s *GCMMessageHandlerTestSuite) SetupSuite() {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "../config/test.yaml"
	}
	c, vConfig, err := config.NewConfigAndViper(configFile)
	s.Require().NoError(err)

	s.config = c
	s.vConfig = vConfig
	s.game = "game"
}

func (s *GCMMessageHandlerTestSuite) setupHandler() (
	*GCMMessageHandler,
	*mocks.GCMClientMock,
	*mocks.StatsDClientMock,
	*mocks.KafkaProducerClientMock,
) {
	logger, _ := test.NewNullLogger()
	mockClient := mocks.NewGCMClientMock()
	mockStatsdClient := mocks.NewStatsDClientMock()
	mockRateLimiter := mocks.NewRateLimiterMock()

	statsD, err := NewStatsD(s.vConfig, logger, mockStatsdClient)
	s.Require().NoError(err)

	mockKafkaProducer := mocks.NewKafkaProducerClientMock()
	kc, err := NewKafkaProducer(s.vConfig, logger, mockKafkaProducer)
	s.Require().NoError(err)

	statsClients := []interfaces.StatsReporter{statsD}
	feedbackClients := []interfaces.FeedbackReporter{kc}
	handler, err := NewGCMMessageHandlerWithClient(
		s.game,
		false,
		s.vConfig,
		logger,
		nil,
		statsClients,
		feedbackClients,
		mockClient,
		mockRateLimiter,
	)
	s.NoError(err)
	s.Require().NotNil(handler)
	s.Equal(s.game, handler.game)
	s.NotNil(handler.ViperConfig)
	s.False(handler.IsProduction)
	s.Equal(int64(0), handler.responsesReceived)
	s.Equal(int64(0), handler.sentMessages)
	s.Len(mockClient.MessagesSent, 0)

	return handler, mockClient, mockStatsdClient, mockKafkaProducer
}

func (s *GCMMessageHandlerTestSuite) TestConfigureHandler() {
	s.Run("should fail if invalid credentials", func() {
		handler, err := NewGCMMessageHandler(
			s.game,
			false,
			s.vConfig,
			logrus.New(),
			nil,
			[]interfaces.StatsReporter{},
			[]interfaces.FeedbackReporter{},
			nil,
		)
		s.Error(err)
		s.Nil(handler)
		s.Equal("error connecting gcm xmpp client: auth failure: not-authorized", err.Error())
	})
}

func (s *GCMMessageHandlerTestSuite) TestHandleGCMResponse() {
	s.Run("should succeed if response has no error", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.NoError(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.successesReceived)
	})

	s.Run("if response has error DEVICE_UNREGISTERED", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "DEVICE_UNREGISTERED",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has error BAD_REGISTRATION", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "BAD_REGISTRATION",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has error INVALID_JSON", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "INVALID_JSON",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has error SERVICE_UNAVAILABLE", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "SERVICE_UNAVAILABLE",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has error INTERNAL_SERVER_ERROR", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "INTERNAL_SERVER_ERROR",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has error DEVICE_MESSAGE_RATE_EXCEEDED", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "DEVICE_MESSAGE_RATE_EXCEEDED",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has error TOPICS_MESSAGE_RATE_EXCEEDED", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "TOPICS_MESSAGE_RATE_EXCEEDED",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})

	s.Run("if response has untracked error", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			Error: "BAD_ACK",
		}
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), handler.responsesReceived)
		s.Equal(int64(1), handler.failuresReceived)
	})
}

func (s *GCMMessageHandlerTestSuite) TestSendMessage() {
	s.Run("should not send message if expire is in the past", func() {
		handler, _, _, _ := s.setupHandler()
		ttl := uint(0)
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}
		msg := &KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       uuid.NewV4().String(),
				Data: map[string]interface{}{
					"title": "notification",
				},
			},
			Metadata:   metadata,
			PushExpiry: MakeTimestamp() - int64(100),
		}
		msgBytes, err := json.Marshal(msg)
		s.Require().NoError(err)

		err = handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		s.Equal(int64(0), handler.sentMessages)
		s.Equal(int64(1), handler.ignoredMessages)
	})

	s.Run("should send message if PushExpiry is in the future", func() {
		handler, _, _, _ := s.setupHandler()
		ttl := uint(0)
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}
		msg := &KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       uuid.NewV4().String(),
				Data: map[string]interface{}{
					"title": "notification",
				},
			},
			Metadata:   metadata,
			PushExpiry: MakeTimestamp() + int64(1000000),
		}
		msgBytes, err := json.Marshal(msg)
		s.Require().NoError(err)

		err = handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		gcmResMutex.Lock()
		s.Equal(int64(1), handler.sentMessages)
		s.Equal(int64(0), handler.ignoredMessages)
		gcmResMutex.Unlock()
	})

	s.Run("should send message and not increment sentMessages if an error occurs", func() {
		handler, mockClient, _, _ := s.setupHandler()
		err := handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte("value"),
		})
		s.Require().Error(err)
		gcmResMutex.Lock()
		s.Equal(int64(0), handler.sentMessages)
		s.Len(handler.pendingMessages, 0)
		gcmResMutex.Unlock()
		s.Len(mockClient.MessagesSent, 0)
	})

	s.Run("should send xmpp message", func() {
		handler, mockClient, _, _ := s.setupHandler()
		ttl := uint(0)
		msg := &interfaces.Message{
			TimeToLive:               &ttl,
			DeliveryReceiptRequested: false,
			DryRun:                   true,
			To:                       uuid.NewV4().String(),
			Data: map[string]interface{}{
				"title": "notification",
			},
		}
		msgBytes, err := json.Marshal(msg)
		s.Require().NoError(err)

		err = handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		gcmResMutex.Lock()
		s.Equal(int64(1), handler.sentMessages)
		s.Len(mockClient.MessagesSent, 1)
		s.Len(handler.pendingMessages, 1)
		gcmResMutex.Unlock()
	})

	s.Run("should send xmpp message with metadata", func() {
		handler, mockClient, _, _ := s.setupHandler()
		ttl := uint(0)
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}
		msg := &KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       uuid.NewV4().String(),
				Data: map[string]interface{}{
					"title": "notification",
				},
			},
			Metadata:   metadata,
			PushExpiry: MakeTimestamp() + int64(1000000),
		}
		msgBytes, err := json.Marshal(msg)
		s.Require().NoError(err)

		err = handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		gcmResMutex.Lock()
		s.Equal(int64(1), handler.sentMessages)
		s.Len(mockClient.MessagesSent, 1)
		s.Len(handler.pendingMessages, 1)
		gcmResMutex.Unlock()
	})

	s.Run("should forward metadata content on GCM request", func() {
		handler, mockClient, _, _ := s.setupHandler()
		ttl := uint(0)
		metadata := map[string]interface{}{
			"some": "metadata",
		}
		msg := &KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       uuid.NewV4().String(),
				Data: map[string]interface{}{
					"title": "notification",
				},
			},
			Metadata:   metadata,
			PushExpiry: MakeTimestamp() + int64(1000000),
		}
		msgBytes, err := json.Marshal(msg)
		s.Require().NoError(err)

		err = handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})

		s.Require().NoError(err)
		gcmResMutex.Lock()
		s.Equal(int64(1), handler.sentMessages)
		s.Len(mockClient.MessagesSent, 1)
		s.Len(handler.pendingMessages, 1)
		gcmResMutex.Unlock()

		sentMessage := mockClient.MessagesSent[0]
		s.NotNil(sentMessage)
		s.Equal("metadata", sentMessage.Data["some"])
	})

	s.Run("should forward nested metadata content on GCM request", func() {
		handler, mockClient, _, _ := s.setupHandler()
		ttl := uint(0)
		metadata := map[string]interface{}{
			"some": "metadata",
		}
		msg := &KafkaGCMMessage{
			Message: interfaces.Message{
				TimeToLive:               &ttl,
				DeliveryReceiptRequested: false,
				DryRun:                   true,
				To:                       uuid.NewV4().String(),
				Data: map[string]interface{}{
					"nested": map[string]interface{}{
						"some": "data",
					},
				},
			},
			Metadata:   metadata,
			PushExpiry: MakeTimestamp() + int64(1000000),
		}
		msgBytes, err := json.Marshal(msg)
		s.Require().NoError(err)

		err = handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})

		s.Require().NoError(err)
		gcmResMutex.Lock()
		s.Equal(int64(1), handler.sentMessages)
		s.Len(mockClient.MessagesSent, 1)
		s.Len(handler.pendingMessages, 1)
		gcmResMutex.Unlock()

		sentMessage := mockClient.MessagesSent[0]
		s.NotNil(sentMessage)
		s.Equal("metadata", sentMessage.Data["some"])
		s.Len(sentMessage.Data["nested"], 1)
		s.Equal("data", sentMessage.Data["nested"].(map[string]interface{})["some"])
	})

	s.Run("should wait to send message if maxPendingMessages is reached", func() {
		handler, _, _, _ := s.setupHandler()
		ttl := uint(0)
		msg := &gcm.XMPPMessage{
			TimeToLive:               &ttl,
			DeliveryReceiptRequested: false,
			DryRun:                   true,
			To:                       uuid.NewV4().String(),
			Data:                     map[string]interface{}{},
		}
		msgBytes, err := json.Marshal(msg)
		s.NoError(err)

		for i := 1; i <= 3; i++ {
			err = handler.sendMessage(interfaces.KafkaMessage{
				Topic: "push-game_gcm",
				Value: msgBytes,
			})
			s.NoError(err)
			s.Equal(int64(i), handler.sentMessages)
			s.Equal(i, len(handler.pendingMessages))
		}

		go func() {
			err := handler.sendMessage(interfaces.KafkaMessage{
				Topic: "push-game_gcm",
				Value: msgBytes,
			})
			s.Require().NoError(err)
		}()

		<-handler.pendingMessages
		s.Eventually(
			func() bool {
				gcmResMutex.Lock()
				defer gcmResMutex.Unlock()
				return handler.sentMessages == 4
			},
			5*time.Second,
			100*time.Millisecond,
		)
	})
}

func (s *GCMMessageHandlerTestSuite) TestCleanCache() {
	s.Run("should remove from push queue after timeout", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
		})
		s.Require().NoError(err)

		go handler.CleanMetadataCache()

		time.Sleep(2 * time.Duration(s.vConfig.GetInt("feedback.cache.requestTimeout")) * time.Millisecond)

		s.True(handler.requestsHeap.Empty())
		handler.inflightMessagesMetadataLock.Lock()
		s.Empty(handler.InflightMessagesMetadata)
		handler.inflightMessagesMetadataLock.Unlock()
	})

	s.Run("should succeed if request gets a response", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		err := handler.sendMessage(interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
		})
		s.Require().NoError(err)

		go handler.CleanMetadataCache()

		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		err = handler.handleGCMResponse(res)
		s.Require().NoError(err)

		time.Sleep(2 * time.Duration(s.vConfig.GetInt("feedback.cache.requestTimeout")) * time.Millisecond)

		s.True(handler.requestsHeap.Empty())
		handler.inflightMessagesMetadataLock.Lock()
		s.Empty(handler.InflightMessagesMetadata)
		handler.inflightMessagesMetadataLock.Unlock()
	})

	s.Run("should handle all responses or remove them after timeout", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		n := 10
		sendRequests := func() {
			for i := 0; i < n; i++ {
				err := handler.sendMessage(interfaces.KafkaMessage{
					Topic: "push-game_gcm",
					Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
				})
				s.Require().NoError(err)
			}
		}

		handleResponses := func() {
			for i := 0; i < n/2; i++ {
				res := gcm.CCSMessage{
					From:        "testToken1",
					MessageID:   "idTest1",
					MessageType: "ack",
					Category:    "testCategory",
				}

				err := handler.handleGCMResponse(res)
				s.Require().NoError(err)
			}
		}

		go handler.CleanMetadataCache()
		go sendRequests()
		time.Sleep(2 * time.Duration(s.vConfig.GetInt("feedback.cache.requestTimeout")) * time.Millisecond)

		go handleResponses()
		time.Sleep(2 * time.Duration(s.vConfig.GetInt("feedback.cache.requestTimeout")) * time.Millisecond)

		s.True(handler.requestsHeap.Empty())
		handler.inflightMessagesMetadataLock.Lock()
		s.Empty(handler.InflightMessagesMetadata)
		handler.inflightMessagesMetadataLock.Unlock()
	})
}

func (s *GCMMessageHandlerTestSuite) TestLogStats() {
	s.Run("should log stats and reset them", func() {
		handler, _, _, _ := s.setupHandler()
		handler.sentMessages = 100
		handler.responsesReceived = 90
		handler.successesReceived = 60
		handler.failuresReceived = 30
		handler.ignoredMessages = 10

		go handler.LogStats()

		s.Eventually(func() bool {
			gcmResMutex.Lock()
			defer gcmResMutex.Unlock()
			return handler.sentMessages == int64(0) &&
				handler.responsesReceived == int64(0) &&
				handler.successesReceived == int64(0) &&
				handler.failuresReceived == int64(0) &&
				handler.ignoredMessages == int64(0)
		}, time.Second, time.Millisecond*100)
	})
}

func (s *GCMMessageHandlerTestSuite) TestStatsReporter() {
	s.Run("should call HandleNotificationSent upon message sent to queue", func() {
		handler, _, mockStatsdClient, mockKafkaProducer := s.setupHandler()
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		ttl := uint(0)
		msg := &gcm.XMPPMessage{
			TimeToLive:               &ttl,
			DeliveryReceiptRequested: false,
			DryRun:                   true,
			To:                       uuid.NewV4().String(),
			Data:                     map[string]interface{}{},
		}
		msgBytes, err := json.Marshal(msg)
		s.NoError(err)

		kafkaMessage := interfaces.KafkaMessage{
			Game:  "game",
			Topic: "push-game_gcm",
			Value: msgBytes,
		}
		err = handler.sendMessage(kafkaMessage)
		s.NoError(err)

		err = handler.sendMessage(kafkaMessage)
		s.NoError(err)
		s.Equal(int64(2), mockStatsdClient.Counts["sent"])
	})

	s.Run("should call HandleNotificationSuccess upon response received", func() {
		handler, _, mockStatsdClient, mockKafkaProducer := s.setupHandler()
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		res := gcm.CCSMessage{}
		err := handler.handleGCMResponse(res)
		s.Require().NoError(err)
		err = handler.handleGCMResponse(res)
		s.Require().NoError(err)

		s.Equal(int64(2), mockStatsdClient.Counts["ack"])
	})

	s.Run("should call HandleNotificationFailure upon error response received", func() {
		handler, _, mockStatsdClient, mockKafkaProducer := s.setupHandler()
		mockKafkaProducer.StartConsumingMessagesInProduceChannel()
		res := gcm.CCSMessage{
			Error: "DEVICE_UNREGISTERED",
		}
		err := handler.handleGCMResponse(res)
		s.Error(err)

		s.Equal(int64(1), mockStatsdClient.Counts["failed"])
	})

}

func (s *GCMMessageHandlerTestSuite) TestFeedbackReporter() {
	s.Run("should include a timestamp in feedback root and the hostname in metadata", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		timestampNow := time.Now().Unix()
		hostname, err := os.Hostname()
		s.Require().NoError(err)

		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": timestampNow,
			"hostname":  hostname,
			"game":      "game",
			"platform":  "gcm",
		}
		handler.inflightMessagesMetadataLock.Lock()
		handler.InflightMessagesMetadata["idTest1"] = metadata
		handler.inflightMessagesMetadataLock.Unlock()
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		go func() {
			err := handler.handleGCMResponse(res)
			s.Require().NoError(err)
		}()

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-mockKafkaProducer.ProduceChannel()
		err = json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(timestampNow, fromKafka.Timestamp)
		s.Equal(hostname, fromKafka.Metadata["hostname"])
	})

	s.Run("should send feedback if success and metadata is present", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}

		handler.inflightMessagesMetadataLock.Lock()
		handler.InflightMessagesMetadata["idTest1"] = metadata
		handler.inflightMessagesMetadataLock.Unlock()

		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		go func() {
			err := handler.handleGCMResponse(res)
			s.Require().NoError(err)
		}()

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Equal(metadata["some"], fromKafka.Metadata["some"])
	})

	s.Run("should send feedback if success and metadata is not present", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		go func() {
			err := handler.handleGCMResponse(res)
			s.Require().NoError(err)
		}()

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Nil(fromKafka.Metadata)
	})

	s.Run("should send feedback if error and metadata is present and token should be deleted", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}

		handler.inflightMessagesMetadataLock.Lock()
		handler.InflightMessagesMetadata["idTest1"] = metadata
		handler.inflightMessagesMetadataLock.Unlock()

		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "nack",
			Category:    "testCategory",
			Error:       "BAD_REGISTRATION",
		}
		go func() {
			err := handler.handleGCMResponse(res)
			s.Error(err)
		}()

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Equal(res.Error, fromKafka.Error)
		s.Equal(metadata["some"], fromKafka.Metadata["some"])
		s.True(fromKafka.Metadata["deleteToken"].(bool))
	})

	s.Run("should send feedback if error and metadata is present and token should not be deleted", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}

		handler.inflightMessagesMetadataLock.Lock()
		handler.InflightMessagesMetadata["idTest1"] = metadata
		handler.inflightMessagesMetadataLock.Unlock()

		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "nack",
			Category:    "testCategory",
			Error:       "INVALID_JSON",
		}
		go func() {
			err := handler.handleGCMResponse(res)
			s.Error(err)
		}()

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Equal(res.Error, fromKafka.Error)
		s.Equal(metadata["some"], fromKafka.Metadata["some"])
		s.Nil(fromKafka.Metadata["deleteToken"])
	})
	s.Run("should send feedback if error and metadata is not present", func() {
		handler, _, _, mockKafkaProducer := s.setupHandler()
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "nack",
			Category:    "testCategory",
			Error:       "BAD_REGISTRATION",
		}
		go func() {
			err := handler.handleGCMResponse(res)
			s.Error(err)
		}()

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Equal(res.Error, fromKafka.Error)
		s.Nil(fromKafka.Metadata)
	})
}
