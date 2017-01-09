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
	"time"

	"github.com/Sirupsen/logrus/hooks/test"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kafka Extension", func() {
	logger, hook := test.NewNullLogger()

	BeforeEach(func() {
		hook.Reset()
	})

	Describe("Creating new client", func() {
		It("should return connected client", func() {
			client := NewKafka("../config/test.yaml", logger)

			Expect(client.Brokers).NotTo(HaveLen(0))
			Expect(client.Topics).To(HaveLen(1))
			Expect(client.ConsumerGroup).NotTo(BeNil())
			Expect(client.msgChan).NotTo(BeClosed())
		})
	})

	Describe("ConsumeLoop", func() {
		It("should consume message and add it to msgChan", func() {
			client := NewKafka("../config/test.yaml", logger)
			Expect(client).NotTo(BeNil())
			defer client.StopConsuming()
			go client.ConsumeLoop()

			time.Sleep(10 * time.Second)
			p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": client.Brokers})
			Expect(err).NotTo(HaveOccurred())
			err = p.Produce(
				&kafka.Message{
					TopicPartition: kafka.TopicPartition{
						Topic:     &client.Topics[0],
						Partition: kafka.PartitionAny,
					},
					Value: []byte("Hello Go!")},
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(client.msgChan, 10*time.Second).Should(Receive(Equal([]byte("Hello Go!"))))
		})
	})
})
