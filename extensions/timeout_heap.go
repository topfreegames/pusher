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
	"container/heap"
	"github.com/spf13/viper"
	"sync"
	"time"
)

// A timeoutNode contains device token and the time when the request expires
type timeoutNode struct {
	UnixTimeStamp int64
	DeviceToken   string
	index         int
}

var timeoutCte int64
var mutex sync.Mutex

type timeoutHeap []*timeoutNode

func (th *timeoutHeap) newTimeoutNode(
	deviceToken string,
) *timeoutNode {
	var now int64 = getNowInUnixMilliseconds()
	node := &timeoutNode{
		UnixTimeStamp: now + timeoutCte,
		DeviceToken:   deviceToken,
	}

	return node
}

func getNowInUnixMilliseconds() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// Len returns how many elements are in the heap
func (th timeoutHeap) Len() int { return len(th) }

// Less returns true if element at index i has timeout before element at index j
func (th timeoutHeap) Less(i, j int) bool { return th[i].UnixTimeStamp < th[j].UnixTimeStamp }

// Swap in the heap elements at index i and j
func (th timeoutHeap) Swap(i, j int) {
	th[i], th[j] = th[j], th[i]
	th[i].index = i
	th[j].index = j
}

// Push receives device token string and pushes it to heap
func (th *timeoutHeap) Push(x interface{}) {
	node := x.(*timeoutNode)

	n := len(*th)
	node.index = n
	*th = append(*th, node)
}

// Pop pops the device token of the next request that expires
func (th *timeoutHeap) Pop() interface{} {
	old := *th
	n := len(old)
	node := old[n-1]
	node.index = -1
	*th = old[0 : n-1]

	return node
}

func (th *timeoutHeap) empty() bool {
	return th.Len() == 0
}

func (th *timeoutHeap) completeHasExpiredRequest() (string, int64, bool) {
	mutex.Lock()
	defer mutex.Unlock()

	if th.empty() {
		return "", 0, false
	}

	now := getNowInUnixMilliseconds()
	node := (*th)[0]

	if now < node.UnixTimeStamp {
		return "", 0, false
	} else {
		heap.Pop(th)
		return node.DeviceToken, node.UnixTimeStamp, true
	}
}

// API: Timeout Heap functions
// For thread safe guarantee, use only the methods below from this api

// NewTimeoutHeap creates and returns a new timeoutHeap
func NewTimeoutHeap(
	config *viper.Viper,
) *timeoutHeap {
	th := make(timeoutHeap, 0)
	heap.Init(&th)
	timeoutCte = int64(config.GetInt("feedback.cache.requestTimeout"))

	return &th
}

// AddRequest pushes new request
func (th *timeoutHeap) AddRequest(deviceToken string) {
	mutex.Lock()
	node := th.newTimeoutNode(deviceToken)
	heap.Push(th, node)
	mutex.Unlock()
}

// HasExpiredRequest removes expired requests, if any, and return deviceToken
func (th *timeoutHeap) HasExpiredRequest() (string, bool) {
	deviceToken, _, has := th.completeHasExpiredRequest()
	return deviceToken, has
}
