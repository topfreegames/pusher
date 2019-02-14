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

package feedback

import (
	"fmt"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	raven "github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// KafkaConsumer for getting pusher feedbacks
type KafkaConsumer struct {
	Brokers                        string
	Config                         *viper.Viper
	Consumer                       interfaces.KafkaConsumerClient
	ConsumerGroup                  string
	ChannelSize                    int
	Logger                         *logrus.Logger
	FetchMinBytes                  int
	FetchWaitMaxMs                 int
	messagesReceived               int64
	msgChan                        chan *FeedbackMessage
	OffsetResetStrategy            string
	run                            bool
	SessionTimeout                 int
	Topics                         []string
	pendingMessagesWG              *sync.WaitGroup
	HandleAllMessagesBeforeExiting bool
	stopChannel                    chan struct{}
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
		Logger:            logger,
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

	q.msgChan = make(chan *FeedbackMessage, q.ChannelSize)

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
		"method":                          "configureConsumer",
		"bootstrap.servers":               q.Brokers,
		"group.id":                        q.ConsumerGroup,
		"session.timeout.ms":              q.SessionTimeout,
		"fetch.min.bytes":                 q.FetchMinBytes,
		"fetch.wait.max.ms":               q.FetchWaitMaxMs,
		"go.events.channel.enable":        true,
		"go.application.rebalance.enable": true,
		"enable.auto.commit":              true,
		"default.topic.config": kafka.ConfigMap{
			"auto.offset.reset":  q.OffsetResetStrategy,
			"auto.commit.enable": true,
		},
		"topics": q.Topics,
	})
	l.Debug("configuring kafka queue extension")

	if client == nil {
		c, err := kafka.NewConsumer(&kafka.ConfigMap{
			"bootstrap.servers":               q.Brokers,
			"group.id":                        q.ConsumerGroup,
			"fetch.min.bytes":                 q.FetchMinBytes,
			"fetch.wait.max.ms":               q.FetchWaitMaxMs,
			"session.timeout.ms":              q.SessionTimeout,
			"go.events.channel.enable":        true,
			"go.application.rebalance.enable": true,
			"enable.auto.commit":              true,
			"default.topic.config": kafka.ConfigMap{
				"auto.offset.reset":  q.OffsetResetStrategy,
				"auto.commit.enable": true,
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

// MessagesChannel returns the channel that will receive all messages got from kafka
func (q *KafkaConsumer) MessagesChannel() *chan *FeedbackMessage {
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

	for q.run == true {
		select {
		case ev := <-q.Consumer.Events():
			switch e := ev.(type) {
			case kafka.AssignedPartitions:
				err = q.assignPartitions(e.Partitions)
				if err != nil {
					l.WithError(err).Error("error assigning partitions")
				}
			case kafka.RevokedPartitions:
				err = q.unassignPartitions()
				if err != nil {
					l.WithError(err).Error("error revoking partitions")
				}
			case *kafka.Message:
				q.receiveMessage(e.TopicPartition, e.Value)
			case kafka.PartitionEOF:
				q.handlePartitionEOF(ev)
			case kafka.OffsetsCommitted:
				q.handleOffsetsCommitted(ev)
			case kafka.Error:
				q.handleError(ev)
				q.StopConsuming()
				close(q.stopChannel)
				return e
			default:
				q.handleUnrecognized(e)
			}
		}
	}

	return nil
}

func (q *KafkaConsumer) assignPartitions(partitions []kafka.TopicPartition) error {
	l := q.Logger.WithFields(logrus.Fields{
		"method":     "assignPartitions",
		"partitions": fmt.Sprintf("%v", partitions),
	})

	l.Debug("Assigning partitions...")
	err := q.Consumer.Assign(partitions)
	if err != nil {
		l.WithError(err).Error("Failed to assign partitions.")
		return err
	}
	l.Info("Partitions assigned.")
	return nil
}

func (q *KafkaConsumer) unassignPartitions() error {
	l := q.Logger.WithFields(logrus.Fields{
		"method": "unassignPartitions",
	})

	l.Debug("Unassigning partitions...")
	err := q.Consumer.Unassign()
	if err != nil {
		l.WithError(err).Error("Failed to unassign partitions.")
		return err
	}
	l.Info("Partitions unassigned.")
	return nil
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
	message := &FeedbackMessage{
		Game:     parsedTopic.Game,
		Platform: parsedTopic.Platform,
		Value:    value,
	}

	q.msgChan <- message

	l.Debug("Received message processed.")
}

func (q *KafkaConsumer) handlePartitionEOF(ev kafka.Event) {
	l := q.Logger.WithFields(logrus.Fields{
		"method":    "handlePartitionEOF",
		"partition": fmt.Sprintf("%v", ev),
	})

	l.Debug("Reached partition EOF.")
}

func (q *KafkaConsumer) handleOffsetsCommitted(ev kafka.Event) {
	l := q.Logger.WithFields(logrus.Fields{
		"method":    "handleOffsetsCommitted",
		"partition": fmt.Sprintf("%v", ev),
	})

	l.Debug("Offsets committed successfully.")
}

func (q *KafkaConsumer) handleError(ev kafka.Event) {
	l := q.Logger.WithFields(logrus.Fields{
		"method": "handleError",
	})
	err := ev.(error)
	raven.CaptureError(err, map[string]string{
		"version":   util.Version,
		"extension": "kafka-consumer",
	})
	l.WithError(err).Error("Error in Kafka connection.")
}

func (q *KafkaConsumer) handleUnrecognized(ev kafka.Event) {
	l := q.Logger.WithFields(logrus.Fields{
		"method": "handleUnrecognized",
		"event":  fmt.Sprintf("%v", ev),
	})
	l.Warn("Kafka event not recognized.")
}

//Cleanup closes kafka consumer connection
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
