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

import "github.com/RobotsAndPencils/buford/push"

//APNSPushQueueMock should be used for tests that need to send pushs to APNS
type APNSPushQueueMock struct {
	Responses chan push.Response
}

//NewAPNSPushQueueMock creates a new instance
func NewAPNSPushQueueMock() *APNSPushQueueMock {
	return &APNSPushQueueMock{
		Responses: make(chan push.Response),
	}
}

//Push records the sent message in the MessagesSent collection
func (m *APNSPushQueueMock) Push(deviceToken string, headers *push.Headers, payload []byte) {
}

//Close records that it is closed
func (m *APNSPushQueueMock) Close() {
	close(m.Responses)
}
