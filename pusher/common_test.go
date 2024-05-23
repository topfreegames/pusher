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

package pusher

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Common Pusher", func() {
	Describe("[Unit]", func() {

		Describe("Wait Timeout", func() {
			It("Should wait for the waitgroup and return true if completed normally", func() {
				wg := &sync.WaitGroup{}
				wg.Add(1)

				go func() {
					time.Sleep(100 * time.Millisecond)
					wg.Done()
				}()

				res := WaitTimeout(wg, 1*time.Second)
				Expect(res).To(BeFalse())
			})

			It("Should wait for the waitgroup and return false if timed out", func() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				res := WaitTimeout(wg, 100*time.Millisecond)
				Expect(res).To(BeTrue())
			})
		})

		Describe("Graceful Shutdown", func() {
			It("Should do nothing if nil waitgroup", func() {
				wg := &sync.WaitGroup{}
				GracefulShutdown(wg, 1*time.Second)
			})

			It("Should wait for the waitgroup and return if completed normally", func() {
				wg := &sync.WaitGroup{}
				wg.Add(1)

				go func() {
					time.Sleep(100 * time.Millisecond)
					wg.Done()
				}()

				GracefulShutdown(wg, 1*time.Second)
			})

			It("Should wait for the waitgroup and return if timed out", func() {
				wg := &sync.WaitGroup{}
				wg.Add(1)
				GracefulShutdown(wg, 100*time.Millisecond)
			})
		})
	})
})
