/*
 * Copyright (c) 2017 TFG Co
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
	log "github.com/Sirupsen/logrus"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// KafkaProducer for producing push feedbacks to a kafka queue
type KafkaProducer struct {
	Brokers    string
	ConfigFile string
	Config     *viper.Viper
	Producer   interfaces.KafkaProducerClient
	Logger     *log.Logger
	Topic      string
}

// NewKafkaProducer for creating a new KafkaProducer instance
func NewKafkaProducer(configFile string, logger *log.Logger, clientOrNil ...interfaces.KafkaProducerClient) (*KafkaProducer, error) {
	q := &KafkaProducer{
		ConfigFile: configFile,
		Logger:     logger,
	}
	var producer interfaces.KafkaProducerClient
	if len(clientOrNil) == 1 {
		producer = clientOrNil[0]
	}
	err := q.configure(producer)
	return q, err
}

func (q *KafkaProducer) loadConfigurationDefaults() {
	q.Config.SetDefault("feedbackQueue.topic", "com.games.teste.feedbacks")
	q.Config.SetDefault("feedbackQueue.brokers", "localhost:9941")
}

func (q *KafkaProducer) configure(producer interfaces.KafkaProducerClient) error {
	q.Config = util.NewViperWithConfigFile(q.ConfigFile)
	q.loadConfigurationDefaults()
	q.Brokers = q.Config.GetString("feedbackQueue.brokers")
	q.Topic = q.Config.GetString("feedbackQueue.topics")
	c := &kafka.ConfigMap{
		"bootstrap.servers": q.Brokers,
	}
	l := q.Logger.WithFields(log.Fields{
		"brokers":    q.Brokers,
		"configFile": q.ConfigFile,
		"topic":      q.Topic,
	})
	l.Debug("configuring kafka producer")

	if producer == nil {
		p, err := kafka.NewProducer(c)
		q.Producer = p
		if err != nil {
			l.WithError(err).Error("error configuring kafka producer client")
			return err
		}
	} else {
		q.Producer = producer
	}
	go q.listenForKafkaResponses()
	l.Info("kafka producer initialized")
	return nil
}

func (q *KafkaProducer) listenForKafkaResponses() {
	l := q.Logger.WithFields(log.Fields{
		"method": "listenForKafkaResponses",
	})
	for e := range q.Producer.Events() {
		switch ev := e.(type) {
		case *kafka.Message:
			m := ev
			if m.TopicPartition.Error != nil {
				l.Errorf("error sending feedback to kafka: %v", m.TopicPartition.Error)
			} else {
				l.Debugf("delivered feedback to topic %s [%d] at offset %v", *m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
			}
			break
		default:
			l.Warnf("ignored kafka response event: %s", ev)
		}
	}
}

// SendFeedback sends the feedback to the kafka Queue
func (q *KafkaProducer) SendFeedback(feedback []byte) {
	m := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &q.Topic,
			Partition: kafka.PartitionAny,
		},
		Value: feedback,
	}
	q.Producer.ProduceChannel() <- m
}
