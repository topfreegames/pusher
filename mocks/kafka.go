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

package mocks

import (
	"fmt"
	"strconv"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// KafkaProducerClientMock  should be used for tests that need to send messages to Kafka
type KafkaProducerClientMock struct {
	MessagesChan chan kafka.Event
	ProduceChan  chan *kafka.Message
	SentMessages int
}

// MockEvent implements kafka.Event
type MockEvent struct {
	Message *kafka.Message
}

// String returns string
func (m *MockEvent) String() string {
	return string(m.Message.Value)
}

// NewKafkaProducerClientMock creates a new instance
func NewKafkaProducerClientMock() *KafkaProducerClientMock {
	k := &KafkaProducerClientMock{
		MessagesChan: make(chan kafka.Event),
		ProduceChan:  make(chan *kafka.Message),
		SentMessages: 0,
	}
	return k
}

// StartConsumingMessagesInProduceChannel starts to consume messages in produce channel and incrementing sentMessages
func (k *KafkaProducerClientMock) StartConsumingMessagesInProduceChannel() {
	go func() {
		for msg := range k.ProduceChan {
			k.SentMessages++
			k.MessagesChan <- msg
		}
	}()
}

// Events returns the mock events channel
func (k *KafkaProducerClientMock) Events() chan kafka.Event {
	return k.MessagesChan
}

// ProduceChannel returns the mock produce channel
func (k *KafkaProducerClientMock) ProduceChannel() chan *kafka.Message {
	return k.ProduceChan
}

// KafkaConsumerClientMock  should be used for tests that need to send messages to Kafka
type KafkaConsumerClientMock struct {
	SubscribedTopics map[string]interface{}
	MessagesChan     chan *kafka.Message
	Closed           bool
	Error            error
	pausedPartitions map[string]kafka.TopicPartition
	Assignments      []kafka.TopicPartition
}

// NewKafkaConsumerClientMock creates a new instance
func NewKafkaConsumerClientMock(errorOrNil ...error) *KafkaConsumerClientMock {
	var err error
	if len(errorOrNil) == 1 {
		err = errorOrNil[0]
	}
	k := &KafkaConsumerClientMock{
		SubscribedTopics: map[string]interface{}{},
		MessagesChan:     make(chan *kafka.Message),
		Closed:           false,
		Error:            err,
	}
	return k
}

// SubscribeTopics mock
func (k *KafkaConsumerClientMock) SubscribeTopics(topics []string, callback kafka.RebalanceCb) error {
	if k.Error != nil {
		return k.Error
	}
	for _, topic := range topics {
		k.SubscribedTopics[topic] = callback
	}
	return nil
}

func (k *KafkaConsumerClientMock) ReadMessage(timeout time.Duration) (*kafka.Message, error) {
	if k.Error != nil {
		return nil, k.Error
	}
	select {
	case msg := <-k.MessagesChan:
		return msg, nil
	case <-time.After(timeout):
		return nil, kafka.NewError(kafka.ErrTimedOut, "", false)
	}
}

// Close mock
func (k *KafkaConsumerClientMock) Close() error {
	if k.Error != nil {
		return k.Error
	}
	k.Closed = true
	return nil
}

func (k *KafkaConsumerClientMock) Pause(topicPartitions []kafka.TopicPartition) error {
	ptpm := map[string]kafka.TopicPartition{}
PassedPartitionsLoop:
	for _, tp := range topicPartitions {
		for _, atp := range k.Assignments {
			if *tp.Topic == *atp.Topic && tp.Partition == atp.Partition {
				ptpm[fmt.Sprintf("%s%s", *atp.Topic, strconv.Itoa(int(atp.Partition)))] = atp
				continue PassedPartitionsLoop
			}
		}
		tp.Error = fmt.Errorf("passed TopicPartition not assigned: %+v", tp)
	}
	for key, v := range ptpm {
		k.pausedPartitions[key] = v
	}
	return nil
}

func (k *KafkaConsumerClientMock) Resume(topicPartitions []kafka.TopicPartition) error {
	ptpm := map[string]kafka.TopicPartition{}
PassedPartitionsLoop:
	for _, tp := range topicPartitions {
		for key, v := range k.pausedPartitions {
			if *tp.Topic == *v.Topic && tp.Partition == v.Partition {
				ptpm[key] = v
				continue PassedPartitionsLoop
			}
		}
		tp.Error = fmt.Errorf("passed TopicPartition not paused: %+v", tp)
	}
	for key := range ptpm {
		delete(k.pausedPartitions, key)
	}
	return nil
}

func (k *KafkaConsumerClientMock) Assign(partitions []kafka.TopicPartition) error {
	k.Assignments = partitions
	return nil
}

func (k *KafkaConsumerClientMock) Assignment() ([]kafka.TopicPartition, error) {
	return k.Assignments, nil
}

func (k *KafkaConsumerClientMock) CommitMessage(m *kafka.Message) ([]kafka.TopicPartition, error) {
	return nil, nil
}
