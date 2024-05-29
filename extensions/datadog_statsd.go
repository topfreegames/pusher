/*
 * Copyright (c) 2018 TFG Co <backend@tfgco.com>
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
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
)

// StatsD for sending metrics
type StatsD struct {
	Client interfaces.StatsDClient
	Config *viper.Viper
	Logger *logrus.Logger
}

// NewStatsD for creating a new StatsD instance
func NewStatsD(config *viper.Viper, logger *logrus.Logger, clientOrNil ...interfaces.StatsDClient) (*StatsD, error) {
	q := &StatsD{
		Config: config,
		Logger: logger,
	}
	var client interfaces.StatsDClient
	if len(clientOrNil) == 1 {
		client = clientOrNil[0]
	}
	err := q.configure(client)
	return q, err
}

var _ interfaces.StatsReporter = &StatsD{}

func (s *StatsD) loadConfigurationDefaults() {
	s.Config.SetDefault("stats.statsd.host", "localhost:8125")
	s.Config.SetDefault("stats.statsd.prefix", "test")
	s.Config.SetDefault("stats.statsd.buflen", 1)
}

func (s *StatsD) configure(client interfaces.StatsDClient) error {
	s.loadConfigurationDefaults()

	host := s.Config.GetString("stats.statsd.host")
	prefix := s.Config.GetString("stats.statsd.prefix")
	buflen := s.Config.GetInt("stats.statsd.buflen")

	l := s.Logger.WithFields(logrus.Fields{
		"host":   host,
		"prefix": prefix,
		"buflen": buflen,
	})

	if client == nil {
		ddClient, err := statsd.NewBuffered(host, buflen)
		ddClient.Namespace = prefix
		client = ddClient

		if err != nil {
			l.WithError(err).Error("Error configuring statsd client.")
			return err
		}
	}

	s.Client = client
	l.Info("StatsD client configured")
	return nil
}

// HandleNotificationSent stores notification count in StatsD
func (s *StatsD) HandleNotificationSent(game string, platform string) {
	s.Client.Incr("sent", []string{fmt.Sprintf("platform:%s", platform), fmt.Sprintf("game:%s", game)}, 1)
}

// HandleNotificationSuccess stores notifications success in StatsD
func (s *StatsD) HandleNotificationSuccess(game string, platform string) {
	s.Client.Incr("ack", []string{fmt.Sprintf("platform:%s", platform), fmt.Sprintf("game:%s", game)}, 1)
}

// HandleNotificationFailure stores each type of failure
func (s *StatsD) HandleNotificationFailure(game string, platform string, err *errors.PushError) {
	s.Client.Incr(
		"failed",
		[]string{fmt.Sprintf("platform:%s", platform), fmt.Sprintf("game:%s", game), fmt.Sprintf("reason:%s", err.Key)},
		1,
	)
}

// InitializeFailure notifu error when is impossible tho initilizer an app
func (s *StatsD) InitializeFailure(game string, platform string) {
	s.Client.Incr("initialize_failure", []string{fmt.Sprintf("platform:%s", platform), fmt.Sprintf("game:%s", game)}, 1)
}

// ReportGoStats reports go stats in statsd
func (s *StatsD) ReportGoStats(
	numGoRoutines int,
	allocatedAndNotFreed, heapObjects, nextGCBytes, pauseGCNano uint64,
) {
	hostname, _ := os.Hostname()
	tags := []string{fmt.Sprintf("hostname:%s", hostname)}
	s.Client.Gauge("num_goroutine", float64(numGoRoutines), tags, 1)
	s.Client.Gauge("allocated_not_freed", float64(allocatedAndNotFreed), tags, 1)
	s.Client.Gauge("heap_objects", float64(heapObjects), tags, 1)
	s.Client.Gauge("next_gc_bytes", float64(nextGCBytes), tags, 1)
}

func (s *StatsD) ReportSendNotificationLatency(latencyMs time.Duration, game string, platform string, labels ...string) {
	metricLabels := []string{fmt.Sprintf("platform:%s", platform), fmt.Sprintf("game:%s", game)}
	for i := 0; i < len(labels); i += 2 {
		metricLabels = append(labels, fmt.Sprintf("%s:%s", labels[i], labels[i+1]))
	}
	s.Client.Timing(
		"send_notification_latency",
		latencyMs,
		metricLabels,
		1,
	)
}

// ReportMetricGauge reports a metric as a Gauge with hostname, game and platform
// as tags
func (s *StatsD) ReportMetricGauge(
	metric string, value float64,
	game, platform string,
) {
	hostname, _ := os.Hostname()
	tags := []string{
		fmt.Sprintf("hostname:%s", hostname),
	}

	if game != "" {
		tags = append(tags, fmt.Sprintf("game:%s", game))
	}

	if platform != "" {
		tags = append(tags, fmt.Sprintf("platform:%s", platform))
	}

	s.Client.Gauge(metric, value, tags, 1)
}

// ReportMetricCount reports a metric as a Count with hostname, game and platform
// as tags
func (s *StatsD) ReportMetricCount(
	metric string, value int64,
	game, platform string,
) {
	hostname, _ := os.Hostname()
	tags := []string{
		fmt.Sprintf("hostname:%s", hostname),
	}

	if game != "" {
		tags = append(tags, fmt.Sprintf("game:%s", game))
	}

	if platform != "" {
		tags = append(tags, fmt.Sprintf("platform:%s", platform))
	}

	s.Client.Count(metric, value, tags, 1)
}

// Cleanup closes statsd connection
func (s *StatsD) Cleanup() error {
	s.Client.Close()
	return nil
}
