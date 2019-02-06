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
	"errors"
	"fmt"
	"sync"

	raven "github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// Metrics name sent by TokenPG
const (
	MetricsTokensToDelete           = "tokens_to_delete"
	MetricsTokensDeleted            = "tokens_deleted"
	MetricsTokensDeletionError      = "tokens_deletion_error"
	MetricsTokensHandleAfterClosing = "tokens_handle_after_closing"
)

// TokenPG for deleting invalid tokens from the database
type TokenPG struct {
	Client *PGClient
	Config *viper.Viper
	Logger *logrus.Logger

	tokensToDelete chan *TokenMsg
	done           chan struct{}
	closed         bool
	wg             sync.WaitGroup

	StatsReporters []interfaces.StatsReporter
}

// TokenMsg represents a token to be deleted in the db
type TokenMsg struct {
	Token    string
	Game     string
	Platform string
}

// NewTokenPG for creating a new TokenPG instance
func NewTokenPG(config *viper.Viper,
	logger *logrus.Logger,
	statsReporters []interfaces.StatsReporter,
	dbOrNil ...interfaces.DB,
) (*TokenPG, error) {
	q := &TokenPG{
		Config:         config,
		Logger:         logger,
		StatsReporters: statsReporters,
	}
	var db interfaces.DB
	if len(dbOrNil) == 1 {
		db = dbOrNil[0]
	}
	err := q.configure(db)

	q.tokensToDelete = make(chan *TokenMsg, config.GetInt("invalidToken.pg.chanSize"))
	q.done = make(chan struct{})
	q.closed = false

	go q.tokenPGWorker()
	q.wg.Add(1)

	return q, err
}

func (t *TokenPG) loadConfigurationDefaults() {}

func (t *TokenPG) configure(db interfaces.DB) error {
	l := t.Logger.WithField("method", "configure")
	t.loadConfigurationDefaults()

	var err error
	t.Client, err = NewPGClient("invalidToken.pg", t.Config, db)
	if err != nil {
		l.WithError(err).Error("Failed to configure psql database.")
		return err
	}

	l.Info("Psql database configured")
	return nil
}

// tokenPGWorker picks TokenMsgs from the queue to delete them from the database
func (t *TokenPG) tokenPGWorker() {
	defer t.wg.Done()

	l := t.Logger.WithField("method", "listenToTokenMsgQueue")

	for {
		select {
		case tkMsg := <-t.tokensToDelete:
			err := t.deleteToken(tkMsg)
			if err != nil {
				l.WithError(err).Error("error deleting token")
				statsReporterReportMetricIncrement(t.StatsReporters,
					MetricsTokensDeletionError, tkMsg.Game, tkMsg.Platform,
				)
			} else {
				statsReporterReportMetricIncrement(t.StatsReporters,
					MetricsTokensDeleted, tkMsg.Game, tkMsg.Platform,
				)
			}

		case <-t.done:
			l.Warn("stopping tokenPGWorker")
			return
		}
	}
}

// HandleToken handles an invalid token. It sends it to a channel to be processed by
// the PGToken worker. If the TokenPG was stopped, an error is returned
func (t *TokenPG) HandleToken(token string, game string, platform string) error {
	l := t.Logger.WithFields(logrus.Fields{
		"method": "HandleToken",
		"token":  token,
	})

	tkMsg := &TokenMsg{
		Token:    token,
		Game:     game,
		Platform: platform,
	}

	if t.closed {
		e := errors.New("can't handle more tokens. The TokenPG has been stopped")
		l.Error(e)
		statsReporterReportMetricIncrement(t.StatsReporters,
			MetricsTokensHandleAfterClosing, tkMsg.Game, tkMsg.Platform,
		)
		return e
	}

	t.tokensToDelete <- tkMsg
	statsReporterReportMetricIncrement(t.StatsReporters,
		MetricsTokensToDelete, tkMsg.Game, tkMsg.Platform,
	)
	return nil
}

// deleteToken deletes the given token in the database
func (t *TokenPG) deleteToken(tkMsg *TokenMsg) error {
	l := t.Logger.WithFields(logrus.Fields{
		"method": "DeleteToken",
		"token":  tkMsg.Token,
	})

	l.Debug("deleting token")
	query := fmt.Sprintf("DELETE FROM %s WHERE token = ?0;", tkMsg.Game+"_"+tkMsg.Platform)
	_, err := t.Client.DB.Exec(query, tkMsg.Token)
	if err != nil && err.Error() != "pg: no rows in result set" {
		raven.CaptureError(err, map[string]string{
			"version":   util.Version,
			"extension": "token-pg",
		})

		return err
	}
	return nil
}

// Cleanup closes statsd connection
func (t *TokenPG) Cleanup() error {
	t.Client.Close()
	return nil
}

// Stop stops the TokenPG's worker
func (t *TokenPG) Stop() (err error) {
	l := t.Logger.WithFields(logrus.Fields{
		"method": "Stop",
	})

	err = t.closeChannel()

	if ut := len(t.tokensToDelete); ut > 0 {
		l.Warnf("%d tokens haven't been processed", ut)
	}

	t.closed = true

	return err
}

func (t *TokenPG) closeChannel() (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("tokenPG has already been stopped")
		}
	}()

	l := t.Logger.WithFields(logrus.Fields{
		"method": "closeChannel",
	})

	close(t.done)
	l.Info("waiting the worker to finish")
	t.wg.Wait()
	l.Info("tokenPG stopped")
	return nil
}

// HasJob returns if the TokenPG still has tokens to be processed
func (t *TokenPG) HasJob() bool {
	if len(t.tokensToDelete) > 0 {
		return true
	}

	return false
}
