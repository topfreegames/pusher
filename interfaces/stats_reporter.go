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

package interfaces

import (
	"time"

	"github.com/topfreegames/pusher/errors"
)

// StatsReporter interface for making stats reporters pluggable easily.
type StatsReporter interface {
	InitializeFailure(game, platform string)
	HandleNotificationSent(game, platform, topic string)
	HandleNotificationSuccess(game, platform string)
	HandleNotificationFailure(game, platform string, err *errors.PushError)
	ReportGoStats(numGoRoutines int, allocatedAndNotFreed, heapObjects, nextGCBytes, pauseGCNano uint64)
	ReportMetricGauge(metric string, value float64, game string, platform string)
	ReportMetricCount(metric string, value int64, game, platform string)
	NotificationRateLimitReached(game string, platform string)
	NotificationRateLimitFailed(game string, platform string)
	ReportSendNotificationLatency(latencyMs time.Duration, game string, platform string, labels ...string)
	ReportFirebaseLatency(latencyMs time.Duration, game string, labels ...string)
}
