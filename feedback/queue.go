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
)

// QueueMessage defines the interface that should be implemented by the type
// produced by a Queue
type QueueMessage interface {
	GetGame() string
	GetPlatform() string
	GetValue() []byte
}

// Queue interface for making new queues pluggable easily
type Queue interface {
	MessagesChannel() chan QueueMessage
	ConsumeLoop(ctx context.Context) error
	StopConsuming()
	Cleanup() error
	PendingMessagesWaitGroup() *sync.WaitGroup
}

// KafkaMessage implements the FeedbackMessage interface
type KafkaMessage struct {
	Game     string
	Platform string
	Value    []byte
}

// GetGame returns the message's Game
func (k *KafkaMessage) GetGame() string {
	return k.Game
}

// GetPlatform returns the message's Platform
func (k *KafkaMessage) GetPlatform() string {
	return k.Platform
}

// GetValue returns the message's Value
func (k *KafkaMessage) GetValue() []byte {
	return k.Value
}
