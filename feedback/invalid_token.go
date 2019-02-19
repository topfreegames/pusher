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
	"strings"
	"time"

	raven "github.com/getsentry/raven-go"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// Metrics name sent by the Handler
const (
	MetricsTokensDeleteSuccess     = "tokens_delete_success"
	MetricsTokensDeleteError       = "tokens_delete_error"
	MetricsTokensDeleteNonexistent = "tokens_delete_nonexistent"
)

// InvalidToken represents a token with the necessary information to be deleted
type InvalidToken struct {
	Token    string
	Game     string
	Platform string
}

// InvalidTokenHandler takes the InvalidTokens from the InChannel and put them in a buffer.
// When the buffer is full or after a timeout, it is flushed, triggering the deletion
// of the tokens from the database
type InvalidTokenHandler struct {
	Logger        *log.Logger
	Config        *viper.Viper
	StatsReporter []interfaces.StatsReporter
	Client        *extensions.PGClient

	flushTime time.Duration

	InChan     chan *InvalidToken
	Buffer     []*InvalidToken
	bufferSize int

	run      bool
	stopChan chan bool
}

// NewInvalidTokenHandler returns a new InvalidTokenHandler instance
func NewInvalidTokenHandler(
	logger *log.Logger, cfg *viper.Viper, statsReporter []interfaces.StatsReporter,
	inChan chan *InvalidToken,
	dbOrNil ...interfaces.DB,
) (*InvalidTokenHandler, error) {
	h := &InvalidTokenHandler{
		Logger:        logger,
		Config:        cfg,
		StatsReporter: statsReporter,
		InChan:        inChan,
		stopChan:      make(chan bool),
	}

	var db interfaces.DB
	if len(dbOrNil) == 1 {
		db = dbOrNil[0]
	}

	err := h.configure(db)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (i *InvalidTokenHandler) loadConfigurationDefaults() {
	i.Config.SetDefault("feedbackListeners.invalidToken.flush.time.ms", 5000)
	i.Config.SetDefault("feedbackListeners.invalidToken.buffer.size", 1000)
}

func (i *InvalidTokenHandler) configure(db interfaces.DB) error {
	l := i.Logger.WithField("method", "configure")
	i.loadConfigurationDefaults()

	i.flushTime = time.Duration(i.Config.GetInt("feedbackListeners.invalidToken.flush.time.ms")) * time.Millisecond
	i.bufferSize = i.Config.GetInt("feedbackListeners.invalidToken.buffer.size")

	i.Buffer = make([]*InvalidToken, 0, i.bufferSize)

	var err error
	i.Client, err = extensions.NewPGClient("feedbackListeners.invalidToken.pg", i.Config, db)
	if err != nil {
		l.WithError(err).Error("failed to configure psql database")
		return err
	}

	l.Info("psql database configured")
	return nil
}

// Start starts to process the InvalidTokens from the intake channel
func (i *InvalidTokenHandler) Start() {
	l := i.Logger.WithField(
		"method", "start",
	)
	l.Info("starting invalid token handler")

	i.run = true
	go i.processMessages()
}

// Stop stops the Handler from consuming messages from the intake channel
func (i *InvalidTokenHandler) Stop() {
	i.run = false
	close(i.stopChan)
}

func (i *InvalidTokenHandler) processMessages() {
	l := i.Logger.WithFields(log.Fields{
		"method": "processMessages",
	})

	flushTicker := time.NewTicker(i.flushTime)
	defer flushTicker.Stop()

	for i.run {
		select {
		case tk, ok := <-i.InChan:
			if ok {
				i.Buffer = append(i.Buffer, tk)

				if len(i.Buffer) >= i.bufferSize {
					l.Debug("buffer is full")
					i.deleteTokens(i.Buffer)
					i.Buffer = make([]*InvalidToken, 0, i.bufferSize)
				}
			}

		case <-flushTicker.C:
			l.Debug("flush ticker")
			i.deleteTokens(i.Buffer)
			i.Buffer = make([]*InvalidToken, 0, i.bufferSize)

		case <-i.stopChan:
			break
		}
	}

	l.Info("stop processing Invalid Token Handler's in channel")
}

// deleteTokens groups tokens by game and platform and deletes them from the
// database. A DELETE query is fecthed for each pair <game, platform>. A best
// effort is applied for each deletion. If there's an error, a log error is
// written and the next <game,platform> is processed
func (i *InvalidTokenHandler) deleteTokens(tokens []*InvalidToken) {
	m := splitTokens(tokens)

	for platform, games := range m {
		for game, tks := range games {
			i.deleteTokensFromGame(tks, game, platform)
		}
	}
}

func splitTokens(tokens []*InvalidToken) map[string]map[string][]string {
	m := make(map[string]map[string][]string)
	m[APNSPlatform] = make(map[string][]string)
	m[GCMPlatform] = make(map[string][]string)

	for _, t := range tokens {
		m[t.Platform][t.Game] = append(m[t.Platform][t.Game], t.Token)
	}

	return m
}

func (i *InvalidTokenHandler) deleteTokensFromGame(tokens []string, game, platform string) error {
	l := i.Logger.WithFields(log.Fields{
		"method":   "deleteTokensFromGame",
		"game":     game,
		"platform": platform,
	})

	var queryBuild strings.Builder
	params := make([]interface{}, 0, len(tokens))
	queryBuild.WriteString(fmt.Sprintf("DELETE FROM %s WHERE token IN (", game+"_"+platform))
	for j, token := range tokens {
		queryBuild.WriteString(fmt.Sprintf("?%d", j))
		if j == len(tokens)-1 {
			queryBuild.WriteString(")")
		} else {
			queryBuild.WriteString(", ")
		}
		params = append(params, token)
	}

	queryBuild.WriteString(";")
	query := queryBuild.String()

	l.Debug("deleting tokens")
	res, err := i.Client.DB.Exec(query, params...)
	if err != nil && err.Error() != "pg: no rows in result set" {
		raven.CaptureError(err, map[string]string{
			"version": util.Version,
			"handler": "invalidToken",
		})

		l.WithError(err).Error("error deleting tokens")
		statsReporterReportMetricCount(i.StatsReporter,
			MetricsTokensDeleteError, int64(len(tokens)), game, platform)

		return err
	}

	if err != nil && err.Error() == "pg: no rows in result set" {
		statsReporterReportMetricCount(i.StatsReporter,
			MetricsTokensDeleteNonexistent, int64(len(tokens)),
			game, platform)

		return nil
	}

	if res.RowsAffected() != len(tokens) {
		statsReporterReportMetricCount(i.StatsReporter,
			MetricsTokensDeleteNonexistent, int64(len(tokens)-res.RowsAffected()),
			game, platform)

		statsReporterReportMetricCount(i.StatsReporter,
			MetricsTokensDeleteSuccess, int64(res.RowsAffected()), game, platform)

		return nil
	}

	statsReporterReportMetricCount(i.StatsReporter,
		MetricsTokensDeleteSuccess, int64(len(tokens)), game, platform)

	return nil
}
