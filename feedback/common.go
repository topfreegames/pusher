/*
 * Copyright (c) 2019 TFG Co <backend@tfgco.com>
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

package feedback

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

type statsReporterInitializer func(*viper.Viper, *logrus.Logger, interfaces.StatsDClient) (interfaces.StatsReporter, error)

// AvailableStatsReporters contains functions to initialize all stats reporters
var AvailableStatsReporters = map[string]statsReporterInitializer{
	"statsd": func(config *viper.Viper, logger *logrus.Logger, clientOrNil interfaces.StatsDClient) (interfaces.StatsReporter, error) {
		return extensions.NewStatsD(config, logger, clientOrNil)
	},
}

func configureStatsReporters(
	config *viper.Viper, logger *logrus.Logger,
	clientOrNil interfaces.StatsDClient,
) ([]interfaces.StatsReporter, error) {
	var reporters []interfaces.StatsReporter
	reporterNames := config.GetStringSlice("stats.reporters")
	for _, reporterName := range reporterNames {
		reporterFunc, ok := AvailableStatsReporters[reporterName]
		if !ok {
			return nil, fmt.Errorf("failed to initialize %s. Stats Reporter not available", reporterName)
		}

		r, err := reporterFunc(config, logger, clientOrNil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize %s. %s", reporterName, err.Error())
		}
		reporters = append(reporters, r)
	}

	return reporters, nil
}

func statsReporterReportMetricCount(
	statsReporters []interfaces.StatsReporter,
	metric string, value int64, game, platform string,
) {
	for _, statsReporter := range statsReporters {
		statsReporter.ReportMetricCount(metric, value, game, platform)
	}
}

func statsReporterReportMetricGauge(
	statsReporters []interfaces.StatsReporter,
	metric string, value float64, game string, platform string,
) {
	for _, statsReporter := range statsReporters {
		statsReporter.ReportMetricGauge(metric, value, game, platform)
	}
}
