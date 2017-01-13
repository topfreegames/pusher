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

import "github.com/confluentinc/confluent-kafka-go/kafka"

// KafkaProducerClientMock  should be used for tests that need to send messages to Kafka
type KafkaProducerClientMock struct {
	EventsChan   chan kafka.Event
	ProduceChan  chan *kafka.Message
	SentMessages int
}

// MockEvent implements kafka.Event
type MockEvent struct {
	Message *kafka.Message
}

//String returns string
func (m *MockEvent) String() string {
	return string(m.Message.Value)
}

// NewKafkaProducerClientMock creates a new instance
func NewKafkaProducerClientMock() *KafkaProducerClientMock {
	k := &KafkaProducerClientMock{
		EventsChan:   make(chan kafka.Event),
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
			k.EventsChan <- msg
		}
	}()
}

// Events returns the mock events channel
func (k *KafkaProducerClientMock) Events() chan kafka.Event {
	return k.EventsChan
}

// ProduceChannel returns the mock produce channel
func (k *KafkaProducerClientMock) ProduceChannel() chan *kafka.Message {
	return k.ProduceChan
}

// KafkaConsumerClientMock  should be used for tests that need to send messages to Kafka
type KafkaConsumerClientMock struct {
	SubscribedTopics   map[string]interface{}
	EventsChan         chan kafka.Event
	AssignedPartitions []kafka.TopicPartition
	Closed             bool
	Error              error
}

// NewKafkaConsumerClientMock creates a new instance
func NewKafkaConsumerClientMock(errorOrNil ...error) *KafkaConsumerClientMock {
	var err error
	if len(errorOrNil) == 1 {
		err = errorOrNil[0]
	}
	k := &KafkaConsumerClientMock{
		SubscribedTopics:   map[string]interface{}{},
		EventsChan:         make(chan kafka.Event),
		AssignedPartitions: []kafka.TopicPartition{},
		Closed:             false,
		Error:              err,
	}
	return k
}

//SubscribeTopics mock
func (k *KafkaConsumerClientMock) SubscribeTopics(topics []string, callback kafka.RebalanceCb) error {
	if k.Error != nil {
		return k.Error
	}
	for _, topic := range topics {
		k.SubscribedTopics[topic] = callback
	}
	return nil
}

//Events mock
func (k *KafkaConsumerClientMock) Events() chan kafka.Event {
	return k.EventsChan
}

//Assign mock
func (k *KafkaConsumerClientMock) Assign(partitions []kafka.TopicPartition) error {
	if k.Error != nil {
		return k.Error
	}
	k.AssignedPartitions = partitions
	return nil
}

//Unassign mock
func (k *KafkaConsumerClientMock) Unassign() error {
	if k.Error != nil {
		return k.Error
	}
	k.AssignedPartitions = []kafka.TopicPartition{}
	return nil
}

//Close mock
func (k *KafkaConsumerClientMock) Close() error {
	if k.Error != nil {
		return k.Error
	}
	k.Closed = true
	return nil
}
