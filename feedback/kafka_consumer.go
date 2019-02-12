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
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	raven "github.com/getsentry/raven-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// KafkaConsumer for getting push requests
type KafkaConsumer struct {
	Brokers                        string
	Config                         *viper.Viper
	Consumer                       *cluster.Consumer
	ConsumerGroup                  string
	Logger                         *log.Logger
	messagesReceived               int64
	FetchWaitMaxMs                 int
	FetchMinBytes                  int
	msgChan                        chan []byte
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
	logger *log.Logger,
	stopChannel *chan struct{},
	clientOrNil ...*cluster.Consumer,
) (*KafkaConsumer, error) {
	q := &KafkaConsumer{
		Config:            config,
		Logger:            logger,
		messagesReceived:  0,
		msgChan:           make(chan []byte),
		pendingMessagesWG: nil,
		stopChannel:       *stopChannel,
	}
	var client *cluster.Consumer
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
	q.Config.SetDefault("feedback.listener.kafka.topics", []string{"com.game.test"})
	q.Config.SetDefault("feedback.listener.kafka.bootstrapServers", "localhost:9941")
	q.Config.SetDefault("feedback.listener.kafka.group", "test")
	q.Config.SetDefault("feedback.listener.kafka.fetch.wait.max.ms", 100)
	q.Config.SetDefault("feedback.listener.kafka.fetch.min.bytes", 2)
	q.Config.SetDefault("feedback.listener.kafka.sessionTimeout", 6000)
	q.Config.SetDefault("feedback.listener.kafka.offsetResetStrategy", "latest")
	q.Config.SetDefault("feedback.listener.kafka.handleAllMessagesBeforeExiting", true)
}

func (q *KafkaConsumer) configure(client *cluster.Consumer) error {
	q.loadConfigurationDefaults()
	q.OffsetResetStrategy = q.Config.GetString("feedback.listener.kafka.offsetResetStrategy")
	q.Brokers = q.Config.GetString("feedback.listener.kafka.bootstrapServers")
	q.ConsumerGroup = q.Config.GetString("feedback.listener.kafka.group")
	q.FetchMinBytes = q.Config.GetInt("feedback.listener.kafka.fetch.min.bytes")
	q.FetchWaitMaxMs = q.Config.GetInt("feedback.listener.kafka.fetch.wait.max.ms")
	q.SessionTimeout = q.Config.GetInt("feedback.listener.kafka.sessionTimeout")
	q.Topics = q.Config.GetStringSlice("feedback.listener.kafka.topics")
	q.HandleAllMessagesBeforeExiting = q.Config.GetBool("feedback.listener.kafka.handleAllMessagesBeforeExiting")

	if q.HandleAllMessagesBeforeExiting {
		var wg sync.WaitGroup
		q.pendingMessagesWG = &wg
	}

	fmt.Println("KAFKA TOPICS: ", q.Config.GetStringSlice("feedback.listener.kafka.topics"))
	fmt.Println("FETCH MIN: ", q.Config.GetInt("feedback.listener.kafka.fetch.min.bytes"))
	err := q.configureConsumer(client)
	if err != nil {
		return err
	}
	return nil
}

type exampleConsumerGroupHandler struct{}

// func (exampleConsumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
// func (exampleConsumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
// func (h exampleConsumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
// 	for msg := range claim.Messages() {
// 		fmt.Printf("Message topic:%q partition:%d offset:%d\n", msg.Topic, msg.Partition, msg.Offset)
// 		sess.MarkMessage(msg, "")
// 	}
// 	return nil
// }

func (q *KafkaConsumer) configureConsumer(client *cluster.Consumer) error {
	l := q.Logger.WithFields(log.Fields{
		"method":                          "configureConsumer",
		"bootstrap.servers":               q.Brokers,
		"group.id":                        q.ConsumerGroup,
		"session.timeout.ms":              q.SessionTimeout,
		"fetch.wait.max.ms":               q.FetchWaitMaxMs,
		"fetch.min.bytes":                 q.FetchMinBytes,
		"go.events.channel.enable":        true,
		"go.application.rebalance.enable": true,
		"enable.auto.commit":              true,
		"default.topic.config": map[string]interface{}{
			"auto.offset.reset":  q.OffsetResetStrategy,
			"auto.commit.enable": true,
		},
		"topics": q.Topics,
	})
	l.Debug("configuring kafka queue extension")

	cfg := cluster.NewConfig()
	// cfg.Brokers = q.Config.GetString(kafka.bootstrapServers)
	cfg.Consumer.Fetch.Min = int32(q.Config.GetInt("feedback.listener.kafka.fetch.min.bytes"))
	cfg.Consumer.MaxWaitTime = (time.Duration(q.Config.GetInt("feedback.listener.kafka.fetch.wait.max.ms")) * time.Millisecond)
	cfg.Consumer.Group.Session.Timeout = time.Duration(q.Config.GetInt("feedback.listener.kafka.sessionTimeout")) * time.Millisecond

	cfg.Consumer.Return.Errors = true
	cfg.Group.Return.Notifications = true

	offsetStrategy := q.Config.GetString("feedback.listener.kafka.offsetResetStrategy")
	cfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	if offsetStrategy == "latest" {
		cfg.Consumer.Offsets.Initial = sarama.OffsetNewest
	} else if offsetStrategy == "earliest" {
		cfg.Consumer.Offsets.Initial = sarama.OffsetOldest
	}

	cfg.Group.Topics.Whitelist = regexp.MustCompile(`^.*-feedbacks$`)
	cfg.Metadata.RefreshFrequency = 10 * time.Millisecond

	cfg.Version = sarama.V0_11_0_1

	// q.OffsetResetStrategy = q.Config.GetString("kafka.offsetResetStrategy")
	q.Brokers = q.Config.GetString("feedback.listener.kafka.bootstrapServers")
	q.ConsumerGroup = q.Config.GetString("feedback.listener.kafka.group")
	// q.FetchMinBytes = q.Config.GetInt("kafka.fetch.min.bytes")
	// q.FetchWaitMaxMs = q.Config.GetInt("kafka.fetch.wait.max.ms")
	// q.SessionTimeout = q.Config.GetInt("kafka.sessionTimeout")
	q.Topics = q.Config.GetStringSlice("feedback.listener.kafka.topics")
	// q.HandleAllMessagesBeforeExiting = q.Config.GetBool("kafka.handleAllMessagesBeforeExiting")

	if client == nil {
		c, err := cluster.NewConsumer([]string{q.Brokers}, q.ConsumerGroup, nil, cfg)
		if err != nil {
			l.WithError(err).Error("error configuring kafka consumer group")
		}

		// c, err := kafka.NewConsumer(&kafka.ConfigMap{
		// 	"bootstrap.servers":               q.Brokers,
		// 	"group.id":                        q.ConsumerGroup,
		// 	"session.timeout.ms":              q.SessionTimeout,
		// 	"go.events.channel.enable":        true,
		// 	"fetch.wait.max.ms":               q.FetchWaitMaxMs,
		// 	"fetch.min.bytes":                 q.FetchMinBytes,
		// 	"go.application.rebalance.enable": true,
		// 	"enable.auto.commit":              true,
		// 	"default.topic.config": kafka.ConfigMap{
		// 		"auto.offset.reset":  q.OffsetResetStrategy,
		// 		"auto.commit.enable": true,
		// 	},
		// })
		// if err != nil {
		// 	l.Error("error configuring kafka queue", zap.Error(err))
		// 	return err
		// }
		q.Consumer = c
	} else {
		q.Consumer = client
	}
	l.Info("kafka queue configured")

	// consume errors
	go func() {
		for err := range q.Consumer.Errors() {
			log.Printf("Error: %s\n", err.Error())
		}
	}()

	// consume notifications
	go func() {
		for ntf := range q.Consumer.Notifications() {
			log.Printf("Rebalanced: %+v\n", ntf)
		}
	}()

	// ctx := context.Background()
	// for {
	// 	topics := []string{"games"}
	// 	handler := exampleConsumerGroupHandler{}

	// 	err := q.Consumer.Consume(ctx, topics, handler)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// }

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
func (q *KafkaConsumer) MessagesChannel() *chan []byte {
	return &q.msgChan
}

// ConsumeLoop consume messages from the queue and put in messages to send channel
func (q *KafkaConsumer) ConsumeLoop() error {
	q.run = true
	l := q.Logger.WithFields(log.Fields{
		"method": "ConsumeLoop",
		"topics": q.Topics,
	})

	// err := q.Consumer.SubscribeTopics(q.Topics, nil)
	// if err != nil {
	// 	l.Error("error subscribing to topics", zap.Error(err))
	// 	return err
	// }

	l.Info("successfully subscribed to topics")

	// for q.run == true {
	// 	select {
	// 	case ev := <-q.Consumer.Events():
	// 		switch e := ev.(type) {
	// 		case kafka.AssignedPartitions:
	// 			err = q.assignPartitions(e.Partitions)
	// 			if err != nil {
	// 				l.Error("error assigning partitions", zap.Error(err))
	// 			}
	// 		case kafka.RevokedPartitions:
	// 			err = q.unassignPartitions()
	// 			if err != nil {
	// 				l.Error("error revoking partitions", zap.Error(err))
	// 			}
	// 		case *kafka.Message:
	// 			q.receiveMessage(e.TopicPartition, e.Value)
	// 		case kafka.PartitionEOF:
	// 			q.handlePartitionEOF(ev)
	// 		case kafka.OffsetsCommitted:
	// 			q.handleOffsetsCommitted(ev)
	// 		case kafka.Error:
	// 			q.handleError(ev)
	// 			q.StopConsuming()
	// 			close(q.stopChannel)
	// 			return e
	// 		default:
	// 			q.handleUnrecognized(e)
	// 		}
	// 	}
	// }

	for {
		select {
		case msg, ok := <-q.Consumer.Messages():
			if ok {
				fmt.Fprintf(os.Stdout, "%s/%d/%d\t%s\t%s\n", msg.Topic, msg.Partition, msg.Offset, msg.Key, msg.Value)
				q.Consumer.MarkOffset(msg, "") // mark message as processed
			}
			// case <-signals:
			// 	return
		}
	}

	return nil
}

func (q *KafkaConsumer) assignPartitions(partitions []kafka.TopicPartition) error {
	l := q.Logger.WithFields(log.Fields{
		"method":     "assignPartitions",
		"partitions": fmt.Sprintf("%v", partitions),
	})

	l.Debug("Assigning partitions...")
	// err := q.Consumer.Assign(partitions)
	// if err != nil {
	// 	l.Error("Failed to assign partitions.", zap.Error(err))
	// 	return err
	// }
	l.Info("Partitions assigned.")
	return nil
}

func (q *KafkaConsumer) unassignPartitions() error {
	l := q.Logger.WithField(
		"method", "unassignPartitions",
	)

	l.Debug("Unassigning partitions...")
	// err := q.Consumer.Unassign()
	// if err != nil {
	// 	l.Error("Failed to unassign partitions.", zap.Error(err))
	// 	return err
	// }
	l.Info("Partitions unassigned.")
	return nil
}

func (q *KafkaConsumer) receiveMessage(topicPartition kafka.TopicPartition, value []byte) {
	l := q.Logger.WithField(
		"method", "receiveMessage",
	)

	l.Debug("Processing received message...")

	q.messagesReceived++
	if q.messagesReceived%1000 == 0 {
		l.Info("received from kafka", zap.Int64("numMessages", q.messagesReceived))
	}
	l.Debug("new kafka message", zap.Int("partition", int(topicPartition.Partition)), zap.String("message", string(value)))
	if q.pendingMessagesWG != nil {
		q.pendingMessagesWG.Add(1)
	}
	q.msgChan <- value

	l.Debug("Received message processed.")
}

func (q *KafkaConsumer) handlePartitionEOF(ev kafka.Event) {
	l := q.Logger.WithFields(log.Fields{
		"method":    "handlePartitionEOF",
		"partition": fmt.Sprintf("%v", ev),
	})

	l.Debug("Reached partition EOF.")
}

func (q *KafkaConsumer) handleOffsetsCommitted(ev kafka.Event) {
	l := q.Logger.WithFields(log.Fields{
		"method":    "handleOffsetsCommitted",
		"partition": fmt.Sprintf("%v", ev),
	})

	l.Debug("Offsets committed successfully.")
}

func (q *KafkaConsumer) handleError(ev kafka.Event) {
	l := q.Logger.WithField(
		"method", "handleError",
	)
	err := ev.(error)
	raven.CaptureError(err, nil)
	l.Error("Error in Kafka connection.", zap.Error(err))
}

func (q *KafkaConsumer) handleUnrecognized(ev kafka.Event) {
	l := q.Logger.WithFields(log.Fields{
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
