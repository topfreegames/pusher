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
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/alexcesaro/statsd"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// StatsD for sending metrics
type StatsD struct {
	Client     interfaces.StatsDClient
	Config     *viper.Viper
	ConfigFile string
	Logger     *logrus.Logger
}

// NewStatsD for creating a new StatsD instance
func NewStatsD(configFile string, logger *logrus.Logger, clientOrNil ...interfaces.StatsDClient) (*StatsD, error) {
	q := &StatsD{
		ConfigFile: configFile,
		Logger:     logger,
	}
	var client interfaces.StatsDClient
	if len(clientOrNil) == 1 {
		client = clientOrNil[0]
	}
	err := q.configure(client)
	return q, err
}

func (s *StatsD) loadConfigurationDefaults() {
	s.Config.SetDefault("stats.statsd.host", "localhost:8125")
	s.Config.SetDefault("stats.statsd.prefix", "test")
	s.Config.SetDefault("stats.statsd.flushIntervalMs", 5000)
}

func (s *StatsD) configure(client interfaces.StatsDClient) error {
	s.Config = util.NewViperWithConfigFile(s.ConfigFile)
	s.loadConfigurationDefaults()

	host := s.Config.GetString("stats.statsd.host")
	prefix := s.Config.GetString("stats.statsd.prefix")
	flushIntervalMs := s.Config.GetInt("stats.statsd.flushIntervalMs")
	flushPeriod := time.Duration(flushIntervalMs) * time.Millisecond

	l := s.Logger.WithFields(logrus.Fields{
		"host":            host,
		"prefix":          prefix,
		"flushIntervalMs": flushIntervalMs,
	})

	if client == nil {
		var err error
		client, err = statsd.New(statsd.Address(host), statsd.FlushPeriod(flushPeriod), statsd.Prefix(prefix))

		if err != nil {
			l.WithError(err).Error("Error configuring statsd client.")
			return err
		}
	}

	s.Client = client
	l.Info("StatsD client configured")
	return nil
}

//HandleNotificationSent stores notification count in StatsD
func (s *StatsD) HandleNotificationSent() {
	s.Client.Increment("sent")
}

//HandleNotificationSuccess stores notifications success in StatsD
func (s *StatsD) HandleNotificationSuccess() {
	s.Client.Increment("ack")
}

//HandleNotificationFailure stores each type of failure
func (s *StatsD) HandleNotificationFailure(err *errors.PushError) {
	s.Client.Increment("failed")
	s.Client.Increment(err.Key)
}

//ReportGoStats reports go stats in statsd
func (s *StatsD) ReportGoStats(
	numGoRoutines int,
	allocatedAndNotFreed, heapObjects, nextGCBytes, pauseGCNano uint64,
) {
	s.Client.Gauge("num_goroutine", numGoRoutines)
	s.Client.Gauge("allocated_not_freed", allocatedAndNotFreed)
	s.Client.Gauge("heap_objects", heapObjects)
	s.Client.Gauge("next_gc_bytes", nextGCBytes)
	s.Client.Timing("gc_pause_duration_ms", pauseGCNano/1000000)
}

//Cleanup closes statsd connection
func (s *StatsD) Cleanup() error {
	s.Client.Close()
	return nil
}
