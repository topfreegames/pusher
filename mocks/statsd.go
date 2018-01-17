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
	"sync"
	"time"
)

//StatsDClientMock should be used for tests that need to send xmpp messages to StatsD
type StatsDClientMock struct {
	Count   map[string]int
	Gauges  map[string]interface{}
	Timings map[string]interface{}
	Closed  bool
}

//NewStatsDClientMock creates a new instance
func NewStatsDClientMock() *StatsDClientMock {
	return &StatsDClientMock{
		Closed:  false,
		Count:   map[string]int{},
		Gauges:  map[string]interface{}{},
		Timings: map[string]interface{}{},
	}
}

var mutexCount, mutexGauges, mutexTimings sync.Mutex

//Incr stores the new count in a map
func (m *StatsDClientMock) Incr(bucket string, tags []string, rate float64) error {
	mutexCount.Lock()
	m.Count[bucket]++
	mutexCount.Unlock()
	return nil
}

//Gauge stores the count in a map
func (m *StatsDClientMock) Gauge(bucket string, value float64, tags []string, rate float64) error {
	mutexGauges.Lock()
	m.Gauges[bucket] = value
	mutexGauges.Unlock()
	return nil
}

//Timing stores the count in a map
func (m *StatsDClientMock) Timing(bucket string, value time.Duration, tags []string, rate float64) error {
	mutexTimings.Lock()
	m.Timings[bucket] = value
	mutexTimings.Unlock()
	return nil
}

//Close records that it is closed
func (m *StatsDClientMock) Close() error {
	m.Closed = true
	return nil
}
