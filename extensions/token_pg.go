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
	"fmt"

	"github.com/Sirupsen/logrus"
	raven "github.com/getsentry/raven-go"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"
)

// TokenPG for sending metrics
type TokenPG struct {
	Client    *PGClient
	Config    *viper.Viper
	Logger    *logrus.Logger
	tableName string
}

// NewTokenPG for creating a new TokenPG instance
func NewTokenPG(config *viper.Viper, logger *logrus.Logger, dbOrNil ...interfaces.DB) (*TokenPG, error) {
	q := &TokenPG{
		Config: config,
		Logger: logger,
	}
	var db interfaces.DB
	if len(dbOrNil) == 1 {
		db = dbOrNil[0]
	}
	err := q.configure(db)
	return q, err
}

func (t *TokenPG) loadConfigurationDefaults() {}

func (t *TokenPG) configure(db interfaces.DB) error {
	l := t.Logger.WithField("method", "configure")
	t.loadConfigurationDefaults()
	t.tableName = t.Config.GetString("invalidToken.pg.table")

	var err error
	t.Client, err = NewPGClient("invalidToken.pg", t.Config, db)
	if err != nil {
		l.WithError(err).Error("Failed to configure psql database.")
		return err
	}

	l.Info("Psql database configured")
	return nil
}

// HandleToken handles an invalid token
func (t *TokenPG) HandleToken(token string) error {
	l := t.Logger.WithFields(logrus.Fields{
		"method": "HandleToken",
		"token":  token,
	})
	l.Debug("deleting token")
	query := fmt.Sprintf("DELETE FROM %s WHERE token = ?0;", t.tableName)
	_, err := t.Client.DB.Exec(query, token)
	if err != nil && err.Error() != "pg: no rows in result set" {
		raven.CaptureError(err, map[string]string{
			"version":   util.Version,
			"extension": "token-pg",
		})
		l.WithError(err).Error("error deleting token")
		return err
	}
	return nil
}

// Cleanup closes statsd connection
func (t *TokenPG) Cleanup() error {
	t.Client.Close()
	return nil
}
