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
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"github.com/topfreegames/pusher/config"
	"os"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/sirupsen/logrus"
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
	logger  *logrus.Logger
	hooks   *test.Hook

	mockClient        *mocks.GCMClientMock
	mockStatsdClient  *mocks.StatsDClientMock
	mockKafkaProducer *mocks.KafkaProducerClientMock

	handler *GCMMessageHandler
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

func (s *GCMMessageHandlerTestSuite) SetupSubTest() {
	s.logger, s.hooks = test.NewNullLogger()

	s.mockClient = mocks.NewGCMClientMock()

	s.mockStatsdClient = mocks.NewStatsDClientMock()
	statsD, err := NewStatsD(s.vConfig, s.logger, s.mockStatsdClient)
	s.Require().NoError(err)

	s.mockKafkaProducer = mocks.NewKafkaProducerClientMock()
	kc, err := NewKafkaProducer(s.vConfig, s.logger, s.mockKafkaProducer)
	s.Require().NoError(err)

	statsClients := []interfaces.StatsReporter{statsD}
	feedbackClients := []interfaces.FeedbackReporter{kc}

	handler, err := NewGCMMessageHandlerWithClient(
		s.game,
		false,
		s.vConfig,
		s.logger,
		nil,
		statsClients,
		feedbackClients,
		s.mockClient,
	)
	s.NoError(err)
	s.Require().NotNil(handler)
	s.Equal(s.game, handler.game)
	s.NotNil(handler.ViperConfig)
	s.False(handler.IsProduction)
	s.Equal(int64(0), handler.responsesReceived)
	s.Equal(int64(0), handler.sentMessages)
	s.Len(s.mockClient.MessagesSent, 0)

	s.handler = handler
}

func (s *GCMMessageHandlerTestSuite) TestConfigureHandler() {
	s.Run("should fail if invalid credentials", func() {
		handler, err := NewGCMMessageHandler(
			s.game,
			false,
			s.vConfig,
			s.logger,
			nil,
			[]interfaces.StatsReporter{},
			[]interfaces.FeedbackReporter{},
		)
		s.Error(err)
		s.Nil(handler)
		s.Equal("error connecting gcm xmpp client: auth failure: not-authorized", err.Error())
	})
}

func (s *GCMMessageHandlerTestSuite) TestHandleGCMResponse() {
	s.Run("should succeed if response has no error", func() {
		res := gcm.CCSMessage{}
		err := s.handler.handleGCMResponse(res)
		s.NoError(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.successesReceived)
	})

	s.Run("if response has error DEVICE_UNREGISTERED", func() {
		res := gcm.CCSMessage{
			Error: "DEVICE_UNREGISTERED",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has error BAD_REGISTRATION", func() {
		res := gcm.CCSMessage{
			Error: "BAD_REGISTRATION",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has error INVALID_JSON", func() {
		res := gcm.CCSMessage{
			Error: "INVALID_JSON",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has error SERVICE_UNAVAILABLE", func() {
		res := gcm.CCSMessage{
			Error: "SERVICE_UNAVAILABLE",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has error INTERNAL_SERVER_ERROR", func() {
		res := gcm.CCSMessage{
			Error: "INTERNAL_SERVER_ERROR",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has error DEVICE_MESSAGE_RATE_EXCEEDED", func() {
		res := gcm.CCSMessage{
			Error: "DEVICE_MESSAGE_RATE_EXCEEDED",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has error TOPICS_MESSAGE_RATE_EXCEEDED", func() {
		res := gcm.CCSMessage{
			Error: "TOPICS_MESSAGE_RATE_EXCEEDED",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})

	s.Run("if response has untracked error", func() {
		res := gcm.CCSMessage{
			Error: "BAD_ACK",
		}
		err := s.handler.handleGCMResponse(res)
		s.Error(err)
		s.Equal(int64(1), s.handler.responsesReceived)
		s.Equal(int64(1), s.handler.failuresReceived)
	})
}

func (s *GCMMessageHandlerTestSuite) TestSendMessage() {
	s.Run("should not send message if expire is in the past", func() {
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

		err = s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		s.Equal(int64(0), s.handler.sentMessages)
		s.Equal(int64(1), s.handler.ignoredMessages)
		s.Contains(s.hooks.LastEntry().Message, "ignoring push")
	})

	s.Run("should send message if PushExpiry is in the future", func() {
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

		err = s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		s.Equal(int64(1), s.handler.sentMessages)
		s.Equal(int64(0), s.handler.ignoredMessages)
	})

	s.Run("should send message and not increment sentMessages if an error occurs", func() {
		err := s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte("gogogo"),
		})
		s.Require().Error(err)
		s.Equal(int64(0), s.handler.sentMessages)
		s.Len(s.hooks.Entries, 1)
		s.Contains(s.hooks.LastEntry().Message, "Error unmarshalling message.")
		s.Len(s.mockClient.MessagesSent, 0)
		s.Len(s.handler.pendingMessages, 0)
	})

	s.Run("should send xmpp message", func() {
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

		err = s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		s.Equal(int64(1), s.handler.sentMessages)
		s.Len(s.mockClient.MessagesSent, 1)
		s.Len(s.handler.pendingMessages, 1)
	})

	s.Run("should send xmpp message with metadata", func() {
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

		err = s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})
		s.Require().NoError(err)
		s.Equal(int64(1), s.handler.sentMessages)
		s.Len(s.mockClient.MessagesSent, 1)
		s.Len(s.handler.pendingMessages, 1)
	})

	s.Run("should forward metadata content on GCM request", func() {
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

		err = s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})

		s.Require().NoError(err)
		s.Equal(int64(1), s.handler.sentMessages)
		s.Len(s.mockClient.MessagesSent, 1)
		s.Len(s.handler.pendingMessages, 1)

		sentMessage := s.mockClient.MessagesSent[0]
		s.NotNil(sentMessage)
		s.Equal("metadata", sentMessage.Data["some"])
	})

	s.Run("should forward nested metadata content on GCM request", func() {
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

		err = s.handler.sendMessage(context.Background(), interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})

		s.Require().NoError(err)
		s.Equal(int64(1), s.handler.sentMessages)
		s.Len(s.mockClient.MessagesSent, 1)
		s.Len(s.handler.pendingMessages, 1)

		sentMessage := s.mockClient.MessagesSent[0]
		s.NotNil(sentMessage)
		s.Equal("metadata", sentMessage.Data["some"])
		s.Len(sentMessage.Data["nested"], 1)
		s.Equal("data", sentMessage.Data["nested"].(map[string]interface{})["some"])
	})

	s.Run("should wait to send message if maxPendingMessages is reached", func() {
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
		ctx := context.Background()

		for i := 1; i <= 3; i++ {
			err = s.handler.sendMessage(ctx, interfaces.KafkaMessage{
				Topic: "push-game_gcm",
				Value: msgBytes,
			})
			s.NoError(err)
			s.Equal(int64(i), s.handler.sentMessages)
			s.Equal(i, len(s.handler.pendingMessages))
		}

		go s.handler.sendMessage(ctx, interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: msgBytes,
		})

		<-s.handler.pendingMessages
		s.Eventually(
			func() bool { return s.handler.sentMessages == 4 },
			5*time.Second,
			100*time.Millisecond,
		)
	})
}

func (s *GCMMessageHandlerTestSuite) TestCleanCache() {
	s.Run("should remove from push queue after timeout", func() {
		ctx := context.Background()
		err := s.handler.sendMessage(ctx, interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
		})
		s.Require().NoError(err)

		go s.handler.CleanMetadataCache()

		time.Sleep(500 * time.Millisecond)

		s.Empty(s.handler.requestsHeap)
		s.Empty(s.handler.InflightMessagesMetadata)
	})

	s.Run("should succeed if request gets a response", func() {
		ctx := context.Background()
		err := s.handler.sendMessage(ctx, interfaces.KafkaMessage{
			Topic: "push-game_gcm",
			Value: []byte(`{ "aps" : { "alert" : "Hello HTTP/2" } }`),
		})
		s.Require().NoError(err)

		go s.handler.CleanMetadataCache()

		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		err = s.handler.handleGCMResponse(res)
		s.Require().NoError(err)

		time.Sleep(500 * time.Millisecond)

		s.Empty(s.handler.requestsHeap)
		s.Empty(s.handler.InflightMessagesMetadata)
	})

	s.Run("should handle all responses or remove them after timeout", func() {
		ctx := context.Background()
		n := 10
		sendRequests := func() {
			for i := 0; i < n; i++ {
				err := s.handler.sendMessage(ctx, interfaces.KafkaMessage{
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

				err := s.handler.handleGCMResponse(res)
				s.Require().NoError(err)
			}
		}

		go s.handler.CleanMetadataCache()
		go sendRequests()
		time.Sleep(500 * time.Millisecond)

		go handleResponses()
		time.Sleep(500 * time.Millisecond)

		s.Empty(s.handler.requestsHeap)
		s.Empty(s.handler.InflightMessagesMetadata)
	})
}

func (s *GCMMessageHandlerTestSuite) TestLogStats() {
	s.Run("should log stats and reset them", func() {
		s.handler.sentMessages = 100
		s.handler.responsesReceived = 90
		s.handler.successesReceived = 60
		s.handler.failuresReceived = 30
		s.handler.ignoredMessages = 10

		go s.handler.LogStats()
		s.Eventually(func() bool {
			for _, e := range s.hooks.Entries {
				if e.Message == "flushing stats" {
					return true
				}
			}
			return false
		},
			time.Second,
			time.Millisecond*100,
		)
		s.Eventually(func() bool { return s.handler.sentMessages == int64(0) }, time.Second, time.Millisecond*100)
		s.Eventually(func() bool { return s.handler.responsesReceived == int64(0) }, time.Second, time.Millisecond*100)
		s.Eventually(func() bool { return s.handler.successesReceived == int64(0) }, time.Second, time.Millisecond*100)
		s.Eventually(func() bool { return s.handler.failuresReceived == int64(0) }, time.Second, time.Millisecond*100)
		s.Eventually(func() bool { return s.handler.ignoredMessages == int64(0) }, time.Second, time.Millisecond*100)
	})
}

func (s *GCMMessageHandlerTestSuite) TestStatsReporter() {
	s.Run("should call HandleNotificationSent upon message sent to queue", func() {
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
		ctx := context.Background()
		kafkaMessage := interfaces.KafkaMessage{
			Game:  "game",
			Topic: "push-game_gcm",
			Value: msgBytes,
		}
		err = s.handler.sendMessage(ctx, kafkaMessage)
		s.NoError(err)

		err = s.handler.sendMessage(ctx, kafkaMessage)
		s.NoError(err)
		s.Equal(int64(2), s.mockStatsdClient.Counts["sent"])
	})

	s.Run("should call HandleNotificationSuccess upon response received", func() {
		res := gcm.CCSMessage{}
		err := s.handler.handleGCMResponse(res)
		s.Require().NoError(err)
		err = s.handler.handleGCMResponse(res)
		s.Require().NoError(err)

		s.Equal(int64(2), s.mockStatsdClient.Counts["ack"])
	})

	s.Run("should call HandleNotificationFailure upon error response received", func() {
		res := gcm.CCSMessage{
			Error: "DEVICE_UNREGISTERED",
		}
		s.handler.handleGCMResponse(res)
		s.handler.handleGCMResponse(res)

		s.Equal(int64(2), s.mockStatsdClient.Counts["failed"])
	})

}

func (s *GCMMessageHandlerTestSuite) TestFeedbackReporter() {
	s.Run("should include a timestamp in feedback root and the hostname in metadata", func() {
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
		s.handler.InflightMessagesMetadata["idTest1"] = metadata
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		go s.handler.handleGCMResponse(res)

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-s.mockKafkaProducer.ProduceChannel()
		err = json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(timestampNow, fromKafka.Timestamp)
		s.Equal(hostname, fromKafka.Metadata["hostname"])
	})

	s.Run("should send feedback if success and metadata is present", func() {
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}
		s.handler.InflightMessagesMetadata["idTest1"] = metadata
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		go s.handler.handleGCMResponse(res)

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-s.mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Equal(metadata["some"], fromKafka.Metadata["some"])
	})

	s.Run("should send feedback if success and metadata is not present", func() {
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "ack",
			Category:    "testCategory",
		}
		go s.handler.handleGCMResponse(res)

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-s.mockKafkaProducer.ProduceChannel()
		err := json.Unmarshal(msg.Value, fromKafka)
		s.Require().NoError(err)
		s.Equal(res.From, fromKafka.From)
		s.Equal(res.MessageID, fromKafka.MessageID)
		s.Equal(res.MessageType, fromKafka.MessageType)
		s.Equal(res.Category, fromKafka.Category)
		s.Nil(fromKafka.Metadata)
	})

	s.Run("should send feedback if error and metadata is present and token should be deleted", func() {
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}
		s.handler.InflightMessagesMetadata["idTest1"] = metadata
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "nack",
			Category:    "testCategory",
			Error:       "BAD_REGISTRATION",
		}
		go s.handler.handleGCMResponse(res)

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-s.mockKafkaProducer.ProduceChannel()
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
		metadata := map[string]interface{}{
			"some":      "metadata",
			"timestamp": time.Now().Unix(),
			"game":      "game",
			"platform":  "gcm",
		}
		s.handler.InflightMessagesMetadata["idTest1"] = metadata
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "nack",
			Category:    "testCategory",
			Error:       "INVALID_JSON",
		}
		go s.handler.handleGCMResponse(res)

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-s.mockKafkaProducer.ProduceChannel()
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
		res := gcm.CCSMessage{
			From:        "testToken1",
			MessageID:   "idTest1",
			MessageType: "nack",
			Category:    "testCategory",
			Error:       "BAD_REGISTRATION",
		}
		go s.handler.handleGCMResponse(res)

		fromKafka := &CCSMessageWithMetadata{}
		msg := <-s.mockKafkaProducer.ProduceChannel()
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

func (s *GCMMessageHandlerTestSuite) TestCleanup() {
	s.Run("should close GCM client without errors", func() {
		err := s.handler.Cleanup()
		s.NoError(err)
		s.True(s.mockClient.Closed)
		fmt.Println("AQUIAQUIAQUIAQUIAQUIAQUIAQUIAQUIAQUIAQUIAQUI")
	})
}
