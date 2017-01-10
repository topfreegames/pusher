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
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/util"
)

// Kafka for getting push requests
type Kafka struct {
	Brokers             string
	Config              *viper.Viper
	ConfigFile          string
	Consumer            *kafka.Consumer
	ConsumerGroup       string
	Logger              *logrus.Logger
	messagesReceived    int64
	msgChan             chan []byte
	OffsetResetStrategy string
	run                 bool
	SessionTimeout      int
	Topic               string
	Topics              []string
	pendingMessagesWG   *sync.WaitGroup
	//TODO document that if this bool is set to true, one should call Done() in
	// q.PendingMessagesWG for each message that was consumed from the queue
	HandleAllMessagesBeforeExiting bool
}

// NewKafka for creating a new Kafka instance
func NewKafka(configFile string, logger *logrus.Logger) *Kafka {
	q := &Kafka{
		ConfigFile:        configFile,
		Logger:            logger,
		messagesReceived:  0,
		msgChan:           make(chan []byte),
		pendingMessagesWG: nil,
	}
	q.configure()
	return q
}

func (q *Kafka) loadConfigurationDefaults() {
	q.Config.SetDefault("queue.topics", []string{"com.games.teste"})
	q.Config.SetDefault("queue.brokers", "localhost:9092")
	q.Config.SetDefault("queue.group", "teste")
	q.Config.SetDefault("queue.sessionTimeout", 6000)
	q.Config.SetDefault("queue.offsetResetStrategy", "latest")
	q.Config.SetDefault("queue.handleAllMessagesBeforeExiting", true)
}

func (q *Kafka) configure() {
	q.Config = util.NewViperWithConfigFile(q.ConfigFile)
	q.loadConfigurationDefaults()
	q.OffsetResetStrategy = q.Config.GetString("queue.offsetResetStrategy")
	q.Brokers = q.Config.GetString("queue.brokers")
	q.ConsumerGroup = q.Config.GetString("queue.group")
	q.SessionTimeout = q.Config.GetInt("queue.sessionTimeout")
	q.Topics = q.Config.GetStringSlice("queue.topics")
	q.HandleAllMessagesBeforeExiting = q.Config.GetBool("queue.handleAllMessagesBeforeExiting")

	if q.HandleAllMessagesBeforeExiting {
		var wg sync.WaitGroup
		q.pendingMessagesWG = &wg
	}

	q.configureConsumer()
}

// PendingMessagesWaitGroup returns the waitGroup that is incremented every time a push is consumed
func (q *Kafka) PendingMessagesWaitGroup() *sync.WaitGroup {
	return q.pendingMessagesWG
}

func (q *Kafka) configureConsumer() {
	//TODO auto commit needs to be false
	l := q.Logger.WithFields(logrus.Fields{
		"method":                          "configureConsumer",
		"bootstrap.servers":               q.Brokers,
		"group.id":                        q.ConsumerGroup,
		"session.timeout.ms":              q.SessionTimeout,
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
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":               q.Brokers,
		"group.id":                        q.ConsumerGroup,
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
		l.WithError(err).Panic("error configuring kafka queue")
	}
	l.Info("kafka queue configured")
	q.Consumer = c
}

// StopConsuming stops consuming messages from the queue
func (q *Kafka) StopConsuming() {
	q.run = false
}

// MessagesChannel returns the channel that will receive all messages got from kafka
func (q *Kafka) MessagesChannel() *chan []byte {
	return &q.msgChan
}

// ConsumeLoop consume messages from the queue and put in messages to send channel
func (q *Kafka) ConsumeLoop() {
	q.run = true
	l := q.Logger.WithFields(logrus.Fields{
		"method": "ConsumeLoop",
		"topics": q.Topics,
	})

	err := q.Consumer.SubscribeTopics(q.Topics, nil)
	if err != nil {
		l.WithError(err).Panic("error subscribing to topics")
	}

	l.Info("successfully subscribed to topics")

	for q.run == true {
		select {
		case ev := <-q.Consumer.Events():
			switch e := ev.(type) {
			case kafka.AssignedPartitions:
				l.Infof("%v\n", e)
				q.Consumer.Assign(e.Partitions)
			case kafka.RevokedPartitions:
				l.Warnf("%v\n", e)
				q.Consumer.Unassign()
			case *kafka.Message:
				q.messagesReceived++
				if q.messagesReceived%1000 == 0 {
					l.Infof("messages from kafka: %d", q.messagesReceived)
				}
				l.Debugf("message on %s:\n%s\n", e.TopicPartition, string(e.Value))
				if q.pendingMessagesWG != nil {
					q.pendingMessagesWG.Add(1)
				}
				q.msgChan <- e.Value
			case kafka.PartitionEOF:
				l.Debugf("reached %v\n", e)
			case kafka.OffsetsCommitted:
				l.Debugf("%v\n", e)
			case kafka.Error:
				l.Errorf("error: %v\n", e)
				//TODO ver isso
				q.run = false
			default:
				l.Warnf("ev not recognized: %v\n", e)
			}
		}
	}
}

//Cleanup closes kafka consumer connection
func (q *Kafka) Cleanup() error {
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
