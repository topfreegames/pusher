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
	"encoding/json"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
)

func handleTokenError(token, service, appName string, logger *logrus.Logger, db interfaces.DB) error {
	l := logger.WithFields(logrus.Fields{
		"method": "handleTokenError",
		"token":  token,
	})
	l.Debug("deleting token")
	query := fmt.Sprintf("DELETE FROM %s_%s WHERE token = ?0;", appName, service)
	_, err := db.Exec(query, token)
	if err != nil && err.Error() != "pg: no rows in result set" {
		l.WithError(err).Error("error deleting token")
		return err
	}
	return nil
}

func sendToFeedbackReporters(feedbackReporters []interfaces.FeedbackReporter, res interface{}) error {
	jres, err := json.Marshal(res)
	if err != nil {
		return err
	}
	if feedbackReporters != nil {
		for _, feedbackReporter := range feedbackReporters {
			feedbackReporter.SendFeedback(jres)
		}
	}
	return nil
}

func statsReporterHandleNotificationSent(statsReporters []interfaces.StatsReporter) {
	for _, statsReporter := range statsReporters {
		statsReporter.HandleNotificationSent()
	}
}

func statsReporterHandleNotificationSuccess(statsReporters []interfaces.StatsReporter) {
	for _, statsReporter := range statsReporters {
		statsReporter.HandleNotificationSuccess()
	}
}

func statsReporterHandleNotificationFailure(statsReporters []interfaces.StatsReporter, err *errors.PushError) {
	for _, statsReporter := range statsReporters {
		statsReporter.HandleNotificationFailure(err)
	}
}
