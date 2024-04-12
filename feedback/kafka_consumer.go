/*
 * Copyright (c) 2019 TFG Co <backend@tfgco.com>
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

package feedback

import (
	"context"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	raven "github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// KafkaConsumer for getting pusher feedbacks
type KafkaConsumer struct {
	Topics                         []string
	Brokers                        string
	Consumer                       interfaces.KafkaConsumerClient
	ConsumerGroup                  string
	OffsetResetStrategy            string
	Config                         *viper.Viper
	ChannelSize                    int
	Logger                         *logrus.Logger
	FetchMinBytes                  int
	FetchWaitMaxMs                 int
	messagesReceived               int64
	msgChan                        chan QueueMessage
	SessionTimeout                 int
	pendingMessagesWG              *sync.WaitGroup
	stopChannel                    chan struct{}
	run                            bool
	HandleAllMessagesBeforeExiting bool
	consumerContext                context.Context
	stopFunc                       context.CancelFunc
}

// NewKafkaConsumer for creating a new KafkaConsumer instance
func NewKafkaConsumer(
	config *viper.Viper,
	logger *logrus.Logger,
	stopChannel *chan struct{},
	clientOrNil ...interfaces.KafkaConsumerClient,
) (*KafkaConsumer, error) {
	q := &KafkaConsumer{
		Config:            config,
		Logger:            logger.WithField("source", "feedback.KafkaConsumer").Logger,
		messagesReceived:  0,
		pendingMessagesWG: nil,
		stopChannel:       *stopChannel,
	}

	var client interfaces.KafkaConsumerClient
	if len(clientOrNil) == 1 {
		client = clientOrNil[0]
	}

	err := q.configure(client)
	if err != nil {
		return nil, err
	}

	q.consumerContext, q.stopFunc = context.WithCancel(context.Background())

	return q, nil
}

func (q *KafkaConsumer) loadConfigurationDefaults() {
	q.Config.SetDefault("feedbackListeners.queue.topics", []string{"com.games.test"})
	q.Config.SetDefault("feedbackListeners.queue.brokers", "localhost:9092")
	q.Config.SetDefault("feedbackListeners.queue.channelSize", 100)
	q.Config.SetDefault("feedbackListeners.queue.fetch.min.bytes", 1)
	q.Config.SetDefault("feedbackListeners.queue.fetch.wait.max.ms", 100)
	q.Config.SetDefault("feedbackListeners.queue.group", "test")
	q.Config.SetDefault("feedbackListeners.queue.sessionTimeout", 6000)
	q.Config.SetDefault("feedbackListeners.queue.offsetResetStrategy", "latest")
	q.Config.SetDefault("feedbackListeners.queue.handleAllMessagesBeforeExiting", true)
}

func (q *KafkaConsumer) configure(client interfaces.KafkaConsumerClient) error {
	q.loadConfigurationDefaults()
	q.OffsetResetStrategy = q.Config.GetString("feedbackListeners.queue.offsetResetStrategy")
	q.Brokers = q.Config.GetString("feedbackListeners.queue.brokers")
	q.ConsumerGroup = q.Config.GetString("feedbackListeners.queue.group")
	q.SessionTimeout = q.Config.GetInt("feedbackListeners.queue.sessionTimeout")
	q.FetchMinBytes = q.Config.GetInt("feedbackListeners.queue.fetch.min.bytes")
	q.FetchWaitMaxMs = q.Config.GetInt("feedbackListeners.queue.fetch.wait.max.ms")
	q.Topics = q.Config.GetStringSlice("feedbackListeners.queue.topics")
	q.ChannelSize = q.Config.GetInt("feedbackListeners.queue.channelSize")
	q.HandleAllMessagesBeforeExiting = q.Config.GetBool("feedbackListeners.queue.handleAllMessagesBeforeExiting")

	q.msgChan = make(chan QueueMessage, q.ChannelSize)

	if q.HandleAllMessagesBeforeExiting {
		var wg sync.WaitGroup
		q.pendingMessagesWG = &wg
	}

	err := q.configureConsumer(client)
	if err != nil {
		return err
	}

	return nil
}

func (q *KafkaConsumer) configureConsumer(client interfaces.KafkaConsumerClient) error {
	l := q.Logger.WithFields(logrus.Fields{
		"method":             "configureConsumer",
		"bootstrap.servers":  q.Brokers,
		"group.id":           q.ConsumerGroup,
		"session.timeout.ms": q.SessionTimeout,
		"fetch.min.bytes":    q.FetchMinBytes,
		"fetch.wait.max.ms":  q.FetchWaitMaxMs,
		"enable.auto.commit": true,
		"default.topic.config": kafka.ConfigMap{
			"auto.offset.reset":  q.OffsetResetStrategy,
			"enable.auto.commit": true,
		},
		"topics": q.Topics,
	})
	l.Debug("configuring kafka queue")

	if client == nil {
		c, err := kafka.NewConsumer(&kafka.ConfigMap{
			"bootstrap.servers":  q.Brokers,
			"group.id":           q.ConsumerGroup,
			"fetch.min.bytes":    q.FetchMinBytes,
			"fetch.wait.max.ms":  q.FetchWaitMaxMs,
			"session.timeout.ms": q.SessionTimeout,
			"enable.auto.commit": true,
			"default.topic.config": kafka.ConfigMap{
				"auto.offset.reset":  q.OffsetResetStrategy,
				"enable.auto.commit": true,
			},
		})

		if err != nil {
			l.WithError(err).Error("error configuring kafka queue")
			return err
		}
		q.Consumer = c
	} else {
		q.Consumer = client
	}

	l.Info("kafka queue configured")
	return nil
}

// PendingMessagesWaitGroup returns the waitGroup that is incremented every time
// a feedback is consumed
func (q *KafkaConsumer) PendingMessagesWaitGroup() *sync.WaitGroup {
	return q.pendingMessagesWG
}

// StopConsuming stops consuming messages from the queue
func (q *KafkaConsumer) StopConsuming() {
	q.stopFunc()
}

// MessagesChannel returns the channel that will receive all messages got from kafka
func (q *KafkaConsumer) MessagesChannel() chan QueueMessage {
	return q.msgChan
}

// ConsumeLoop consume messages from the queue and put in messages to send channel
func (q *KafkaConsumer) ConsumeLoop() error {
	l := q.Logger.WithFields(logrus.Fields{
		"method": "ConsumeLoop",
		"topics": q.Topics,
	})

	err := q.Consumer.SubscribeTopics(q.Topics, nil)
	if err != nil {
		l.WithError(err).Error("error subscribing to topics")
		return err
	}

	l.Info("successfully subscribed to topics")

	for {
		select {
		case <-q.consumerContext.Done():
			l.Info("context done, stopping consuming")
			return nil
		default:
			message, err := q.Consumer.ReadMessage(100)
			if message == nil && err.(kafka.Error).IsTimeout() {
				continue
			}
			if err != nil {
				q.handleError(err)
				continue
			}
			q.receiveMessage(message.TopicPartition, message.Value)
		}
	}
}

func (q *KafkaConsumer) receiveMessage(topicPartition kafka.TopicPartition, value []byte) {
	l := q.Logger.WithFields(logrus.Fields{
		"method": "receiveMessage",
	})

	l.Debug("Processing received message...")

	q.messagesReceived++
	if q.messagesReceived%1000 == 0 {
		l.Infof("messages from kafka: %d", q.messagesReceived)
	}
	l.Debugf("message on %s:\n%s\n", topicPartition, string(value))
	if q.pendingMessagesWG != nil {
		q.pendingMessagesWG.Add(1)
	}

	parsedTopic := extensions.GetGameAndPlatformFromTopic(*topicPartition.Topic)
	message := &KafkaMessage{
		Game:     parsedTopic.Game,
		Platform: parsedTopic.Platform,
		Value:    value,
	}

	q.msgChan <- message
	l.Debug("Received message processed.")
}

func (q *KafkaConsumer) handleError(err error) {
	l := q.Logger.WithFields(logrus.Fields{
		"method":  "feedback.handleError",
		"brokers": q.Brokers,
	})
	raven.CaptureError(err, map[string]string{
		"version":   util.Version,
		"extension": "kafka-consumer",
	})
	l.WithError(err).Error("Error in Kafka connection.")
}

// Cleanup closes kafka consumer connection
func (q *KafkaConsumer) Cleanup() error {
	select {
	case <-q.consumerContext.Done():
	default:
		q.StopConsuming()
	}

	if q.Consumer != nil {
		err := q.Consumer.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
