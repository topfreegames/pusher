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
	"sync"
	"time"

	"github.com/spf13/viper"
)

type timeoutNode struct {
	UnixTimeStamp int64
	DeviceToken   string
	index         int
}

var timeoutCte int64
var mutex sync.Mutex

// TimeoutHeap is a array of timeoutNode, which has request ID and expiration time
type TimeoutHeap []*timeoutNode

func (th *TimeoutHeap) newTimeoutNode(deviceToken string) *timeoutNode {
	now := getNowInUnixMilliseconds()
	node := &timeoutNode{
		UnixTimeStamp: now + timeoutCte,
		DeviceToken:   deviceToken,
	}

	return node
}

// Helper functions
func getNowInUnixMilliseconds() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// Len returns the length of the heap
func (th TimeoutHeap) Len() int { return len(th) }

// Less returns true if request at index i expires before request at index j
func (th TimeoutHeap) Less(i, j int) bool { return th[i].UnixTimeStamp < th[j].UnixTimeStamp }

// Swap swaps requests at index i and j
func (th TimeoutHeap) Swap(i, j int) {
	th[i], th[j] = th[j], th[i]
	th[i].index = i
	th[j].index = j
}

// Push receives device token string and pushes it to heap
func (th *TimeoutHeap) Push(x interface{}) {
	node := x.(*timeoutNode)

	n := len(*th)
	node.index = n
	*th = append(*th, node)
}

// Pop pops the device token of the next request that expires
func (th *TimeoutHeap) Pop() interface{} {
	old := *th
	n := len(old)
	node := old[n-1]
	node.index = -1
	*th = old[0 : n-1]

	return node
}

// Empty returns true if heap has  no elements
func (th *TimeoutHeap) Empty() bool {
	mutex.Lock()
	defer mutex.Unlock()
	return th.Len() == 0
}

func (th *TimeoutHeap) completeHasExpiredRequest() (string, int64, bool) {
	mutex.Lock()
	defer mutex.Unlock()

	if len(*th) == 0 {
		return "", 0, false
	}

	now := getNowInUnixMilliseconds()
	node := (*th)[0]

	if now < node.UnixTimeStamp {
		return "", 0, false
	}
	heap.Pop(th)
	return node.DeviceToken, node.UnixTimeStamp, true
}

// For thread safe guarantee, use only the methods below from this api

// NewTimeoutHeap creates and returns a new TimeoutHeap
func NewTimeoutHeap(config *viper.Viper) *TimeoutHeap {
	th := make(TimeoutHeap, 0)
	heap.Init(&th)
	timeoutCte = int64(config.GetInt("feedback.cache.requestTimeout"))

	return &th
}

// AddRequest pushes new request
func (th *TimeoutHeap) AddRequest(deviceToken string) {
	mutex.Lock()
	node := th.newTimeoutNode(deviceToken)
	heap.Push(th, node)
	mutex.Unlock()
}

// HasExpiredRequest removes expired request, if any
func (th *TimeoutHeap) HasExpiredRequest() (string, bool) {
	deviceToken, _, has := th.completeHasExpiredRequest()
	return deviceToken, has
}
