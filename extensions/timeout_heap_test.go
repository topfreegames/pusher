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
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sync"
	"time"
)

var _ = Describe("[Unit]", func() {
	Describe("TimeoutHeap", func() {
		It("should push a new timeout node to heap", func() {
			th := NewTimeoutHeap()

			var deviceToken string = "string_device"
			th.AddRequest(deviceToken)
			Ω(th.Len()).Should(Equal(1))

			returnedNode := heap.Pop(th).(*timeoutNode)
			Ω(returnedNode.DeviceToken).Should(Equal(deviceToken))
		})

		It("should pop timeout nodes in order of timeout", func() {
			th := NewTimeoutHeap()

			tokens := [3]string{"test_device_1", "test_device_2", "test_device_3"}
			var node *timeoutNode
			for _, token := range tokens {
				th.AddRequest(token)
				time.Sleep(100 * time.Millisecond)
			}

			Ω(th.Len()).Should(Equal(3))

			for i := 0; i < 3; i++ {
				node = heap.Pop(th).(*timeoutNode)
				Ω(node.DeviceToken).Should(Equal(tokens[i]))
			}
		})

		It("should return true if heap is empty", func() {
			th := NewTimeoutHeap()
			Ω(th.empty()).Should(BeTrue())

			th.AddRequest("token")
			Ω(th.empty()).Should(BeFalse())
		})

		It("should return nodes in order of time stamp from threads", func() {
			th := NewTimeoutHeap()
			var wg sync.WaitGroup

			startPushing := func(threadId int) {
				for i := 0; i < 3; i++ {
					th.AddRequest(fmt.Sprintf("thread_%d_token_%d", threadId, i))
					time.Sleep(10 * time.Millisecond)
				}
				wg.Done()
			}

			for i := 1; i <= 3; i++ {
				wg.Add(1)
				go startPushing(i)
			}

			wg.Wait()

			var min int64 = 0
			for _, timeout, has := th.completeHasExpiredRequest(); has; {
				Ω(min).Should(BeNumerically("<=", timeout))
				min = timeout

				_, timeout, has = th.completeHasExpiredRequest()
			}
		})
	})

	Describe("Timeout expiration", func() {
		It("should remove node request after timeout", func() {
			th := NewTimeoutHeap()
			var wg sync.WaitGroup

			startPushing := func(threadId int) {
				for i := 0; i < 3; i++ {
					th.AddRequest(fmt.Sprintf("thread_%d_token_%d", threadId, i))
					time.Sleep(10 * time.Millisecond)
				}
				wg.Done()
			}

			for i := 1; i <= 3; i++ {
				wg.Add(1)
				go startPushing(i)
			}

			wg.Wait()

			startComsuming := func() {
				for token, has := th.HasExpiredRequest(); has; {
					Ω(token).Should(ContainSubstring("thread"))
					Ω(token).Should(ContainSubstring("token"))
					token, has = th.HasExpiredRequest()
				}
			}

			for i := 0; i < 3; i++ {
				go startComsuming()
			}
		})
	})
})
