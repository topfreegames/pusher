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
	uuid "github.com/satori/go.uuid"
	gcm "github.com/topfreegames/go-gcm"
)

//GCMClientMock should be used for tests that need to send xmpp messages to GCM
type GCMClientMock struct {
	MessagesSent []gcm.XMPPMessage
	Closed       bool
}

//NewGCMClientMock creates a new instance
func NewGCMClientMock() *GCMClientMock {
	return &GCMClientMock{
		Closed:       false,
		MessagesSent: []gcm.XMPPMessage{},
	}
}

//SendXMPP records the sent message in the MessagesSent collection
func (m *GCMClientMock) SendXMPP(msg gcm.XMPPMessage) (string, int, error) {
	m.MessagesSent = append(m.MessagesSent, msg)
	return uuid.NewV4().String(), 0, nil
}

//Close records that it is closed
func (m *GCMClientMock) Close() error {
	m.Closed = true
	return nil
}
