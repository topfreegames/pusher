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

package mocks

import (
	"github.com/sideshow/apns2"
	"github.com/topfreegames/pusher/structs"
)

// APNSPushQueueMock should be used for tests that need to send pushs to APNS.
type APNSPushQueueMock struct {
	responseChannel    chan *structs.ResponseWithMetadata
	Closed             bool
	PushedNotification *apns2.Notification
}

// NewAPNSPushQueueMock creates a new instance.
func NewAPNSPushQueueMock() *APNSPushQueueMock {
	return &APNSPushQueueMock{
		responseChannel: make(chan *structs.ResponseWithMetadata),
	}
}

// Push records the sent message in the MessagesSent collection
func (m *APNSPushQueueMock) Push(n *apns2.Notification) {
	m.PushedNotification = n
}

func (m *APNSPushQueueMock) Configure() error {
	return nil
}

// ResponseChannel returns responseChannel
func (m *APNSPushQueueMock) ResponseChannel() chan *structs.ResponseWithMetadata {
	return m.ResponseChannel()
}

// Close records that it is closed
func (m *APNSPushQueueMock) Close() {
	close(m.responseChannel)
	m.Closed = true
}
