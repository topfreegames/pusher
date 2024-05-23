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
	"regexp"

	"github.com/topfreegames/pusher/errors"
	"github.com/topfreegames/pusher/interfaces"
)

var topicRegex = regexp.MustCompile("push-([^-_]+)[-_]([^-_]+)")

// ParsedTopic contains game and platform extracted from topic name
type ParsedTopic struct {
	Platform string
	Game     string
}

func getGameAndPlatformFromTopic(topic string) ParsedTopic {
	res := topicRegex.FindStringSubmatch(topic)
	return ParsedTopic{
		Platform: res[2],
		Game:     res[1],
	}
}

// GetGameAndPlatformFromTopic returns the game and plaform specified in the Kafka topic
func GetGameAndPlatformFromTopic(topic string) ParsedTopic {
	return getGameAndPlatformFromTopic(topic)
}

func sendToFeedbackReporters(feedbackReporters []interfaces.FeedbackReporter, res interface{}, topic ParsedTopic) error {
	jres, err := json.Marshal(res)
	if err != nil {
		return err
	}

	for _, feedbackReporter := range feedbackReporters {
		feedbackReporter.SendFeedback(topic.Game, topic.Platform, jres)
	}

	return nil
}

func statsReporterHandleNotificationSent(statsReporters []interfaces.StatsReporter, game string, platform string) {
	for _, statsReporter := range statsReporters {
		statsReporter.HandleNotificationSent(game, platform)
	}
}

func statsReporterHandleNotificationSuccess(statsReporters []interfaces.StatsReporter, game string, platform string) {
	for _, statsReporter := range statsReporters {
		statsReporter.HandleNotificationSuccess(game, platform)
	}
}

func statsReporterHandleNotificationFailure(
	statsReporters []interfaces.StatsReporter,
	game string,
	platform string,
	err *errors.PushError,
) {
	for _, statsReporter := range statsReporters {
		statsReporter.HandleNotificationFailure(game, platform, err)
	}
}
