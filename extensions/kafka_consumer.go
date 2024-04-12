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
	"fmt"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// KafkaConsumer for getting push requests
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
	msgChan                        chan interfaces.KafkaMessage
	SessionTimeout                 int
	pendingMessagesWG              *sync.WaitGroup
	stopChannel                    chan struct{}
	run                            bool
	HandleAllMessagesBeforeExiting bool
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
		Logger:            logger.WithField("source", "extensions.KafkaConsumer").Logger,
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
	return q, nil
}

func (q *KafkaConsumer) loadConfigurationDefaults() {
	q.Config.SetDefault("queue.topics", []string{"com.games.test"})
	q.Config.SetDefault("queue.brokers", "localhost:9092")
	q.Config.SetDefault("queue.channelSize", 100)
	q.Config.SetDefault("queue.fetch.min.bytes", 1)
	q.Config.SetDefault("queue.fetch.wait.max.ms", 100)
	q.Config.SetDefault("queue.group", "test")
	q.Config.SetDefault("queue.sessionTimeout", 6000)
	q.Config.SetDefault("queue.offsetResetStrategy", "latest")
	q.Config.SetDefault("queue.handleAllMessagesBeforeExiting", true)
}

func (q *KafkaConsumer) configure(client interfaces.KafkaConsumerClient) error {
	q.loadConfigurationDefaults()
	q.OffsetResetStrategy = q.Config.GetString("queue.offsetResetStrategy")
	q.Brokers = q.Config.GetString("queue.brokers")
	q.ConsumerGroup = q.Config.GetString("queue.group")
	q.SessionTimeout = q.Config.GetInt("queue.sessionTimeout")
	q.FetchMinBytes = q.Config.GetInt("queue.fetch.min.bytes")
	q.FetchWaitMaxMs = q.Config.GetInt("queue.fetch.wait.max.ms")
	q.Topics = q.Config.GetStringSlice("queue.topics")
	q.ChannelSize = q.Config.GetInt("queue.channelSize")
	q.HandleAllMessagesBeforeExiting = q.Config.GetBool("queue.handleAllMessagesBeforeExiting")

	q.msgChan = make(chan interfaces.KafkaMessage, q.ChannelSize)

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
			"auto.offset.reset": q.OffsetResetStrategy,
		},
		"topics": q.Topics,
	})
	l.Debug("configuring kafka queue extension")

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

// PendingMessagesWaitGroup returns the waitGroup that is incremented every time a push is consumed
func (q *KafkaConsumer) PendingMessagesWaitGroup() *sync.WaitGroup {
	return q.pendingMessagesWG
}

// StopConsuming stops consuming messages from the queue
func (q *KafkaConsumer) StopConsuming() {
	q.run = false
}

func (q *KafkaConsumer) Pause(topic string) error {
	topicPartitions, err := q.Consumer.Assignment()
	if err != nil {
		return err
	}
	var topicPartitionsToPause []kafka.TopicPartition
	for _, tp := range topicPartitions {
		if *tp.Topic == topic {
			topicPartitionsToPause = append(topicPartitionsToPause, tp)
		}
	}
	if len(topicPartitionsToPause) == 0 {
		return fmt.Errorf("not subscribed to provided topic: %s", topic)
	}
	return q.Consumer.Pause(topicPartitionsToPause)
}

func (q *KafkaConsumer) Resume(topic string) error {
	topicPartitions, err := q.Consumer.Assignment()
	if err != nil {
		return err
	}
	var topicPartitionsToResume []kafka.TopicPartition
	for _, tp := range topicPartitions {
		if *tp.Topic == topic {
			topicPartitionsToResume = append(topicPartitionsToResume, tp)
		}
	}
	if len(topicPartitionsToResume) == 0 {
		return fmt.Errorf("not subscribed to provided topic: %s", topic)
	}
	return q.Consumer.Resume(topicPartitionsToResume)
}

// MessagesChannel returns the channel that will receive all messages got from kafka
func (q *KafkaConsumer) MessagesChannel() *chan interfaces.KafkaMessage {
	return &q.msgChan
}

// ConsumeLoop consume messages from the queue and put in messages to send channel
func (q *KafkaConsumer) ConsumeLoop() error {
	q.run = true
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

	//nolint[:gosimple]
	for q.run {
		message, err := q.Consumer.ReadMessage(100)
		if message == nil && err.(kafka.Error).IsTimeout() {
			continue
		}
		if err != nil {
			q.handleError(err)
			continue
		}
		q.receiveMessage(message.TopicPartition, message.Value)
		_, err = q.Consumer.CommitMessage(message)
		if err != nil {
			q.handleError(err)
			return fmt.Errorf("error committing message: %s", err.Error())
		}
	}

	return nil
}

func (q *KafkaConsumer) receiveMessage(topicPartition kafka.TopicPartition, value []byte) {
	l := q.Logger.WithFields(logrus.Fields{
		"method":       "receiveMessage",
		"topic":        *topicPartition.Topic,
		"partitionKey": topicPartition.Partition,
		"jsonValue":    string(value),
	})

	l.Debug("Processing received message...")

	if q.pendingMessagesWG != nil {
		q.pendingMessagesWG.Add(1)
	}

	message := interfaces.KafkaMessage{
		Game:  getGameAndPlatformFromTopic(*topicPartition.Topic).Game,
		Topic: *topicPartition.Topic,
		Value: value,
	}

	q.msgChan <- message

	l.Debug("added message to channel")
}

func (q *KafkaConsumer) handleError(err error) {
	l := q.Logger.WithFields(logrus.Fields{
		"method":  "extensions.handleError",
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
	if q.run {
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
