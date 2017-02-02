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

import "time"

// A timeoutNode contains device token and the time when the request expires
type timeoutNode struct {
	unixTimeStamp int64
	DeviceToken   string
}

// TODO: remove this constant and get it from config file
var timeoutCte int64 = 10 * 1000

// newTimeoutNode for creating a new timeoutNode instance
func newTimeoutNode(
	deviceToken string,
) (*timeoutNode, error) {
	now := time.Now().Unix()
	node := &timeoutNode{
		unixTimeStamp: now + timeoutCte,
		DeviceToken:   deviceToken,
	}

	return node, nil
}

type TimeoutHeap []*timeoutNode

// Implements heap interface
func (th TimeoutHeap) Len() int           { return len(th) }
func (th TimeoutHeap) Less(i, j int) bool { return th[i].unixTimeStamp < th[j].unixTimeStamp }
func (th TimeoutHeap) Swap(i, j int)      { th[i], th[j] = th[j], th[i] }

func (th *TimeoutHeap) Push(x interface{}) {
	node := x.(*timeoutNode)
	*th = append(*th, node)
}

func (th *TimeoutHeap) Pop() interface{} {
	old := *th
	n := len(old)
	node := old[n-1]
	*th = old[0 : n-1]
	return node
}

// Timeout Heap functions
// Peek last item without removing it
func (th *TimeoutHeap) Peek() interface{} {
	n := len(*th)
	return (*th)[n-1]
}

// If heap has expired Request, remove it and return deviceToken
func (th *TimeoutHeap) HasExpiredRequest() (string, bool) {
	node := th.Peek().(*timeoutNode)
	now := time.Now().Unix()

	if now < node.unixTimeStamp {
		return "", false
	} else {
		th.Pop()
		return (*node).DeviceToken, true
	}
}
