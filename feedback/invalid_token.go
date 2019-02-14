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
	Logger *log.Logger
	Config *viper.Viper
	Client *extensions.PGClient

	FlushTicker *time.Ticker
	InChan      *chan *InvalidToken
	Buffer      []*InvalidToken
	bufferSize  int
	run         bool
	stopChan    chan bool
}

// NewInvalidTokenHandler returns a new InvalidTokenHandler instance
func NewInvalidTokenHandler(
	logger *log.Logger, cfg *viper.Viper,
	inChan *chan *InvalidToken,
	dbOrNil ...interfaces.DB,

) (*InvalidTokenHandler, error) {
	h := &InvalidTokenHandler{
		Logger: logger,
		Config: cfg,
		InChan: inChan,
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
	l := i.Logger.WithField("operation", "configure")
	i.loadConfigurationDefaults()

	flushTime := time.Duration(i.Config.GetInt("feedbackListeners.invalidToken.flush.time.ms")) * time.Millisecond
	i.bufferSize = i.Config.GetInt("feedbackListeners.invalidToken.buffer.size")

	i.FlushTicker = time.NewTicker(flushTime)
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
	i.run = true
	go i.processMessages()
}

func (i *InvalidTokenHandler) processMessages() {
	l := i.Logger.WithFields(log.Fields{
		"operation": "processMessages",
	})

	for i.run {
		select {
		case tk := <-*i.InChan:
			i.Buffer = append(i.Buffer, tk)

			if len(i.Buffer) >= i.bufferSize {
				l.Debug("buffer is full")
				go i.deleteTokens(i.Buffer)
				i.Buffer = make([]*InvalidToken, 0, i.bufferSize)
			}

		case <-i.FlushTicker.C:
			l.Debug("flush ticker")
			go i.deleteTokens(i.Buffer)
			i.Buffer = make([]*InvalidToken, 0, i.bufferSize)

		case <-i.stopChan:
			break
		}
	}
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
		"operation": "deleteTokensFromGame",
		"game":      game,
		"platform":  platform,
	})

	var queryBuild strings.Builder
	queryBuild.WriteString(fmt.Sprintf("DELETE FROM %s WHERE token IN (", game+"_"+platform))
	for j := range tokens {
		queryBuild.WriteString(fmt.Sprintf("?%d", j))
		if j == len(tokens)-1 {
			queryBuild.WriteString(")")
		} else {
			queryBuild.WriteString(", ")
		}
	}

	queryBuild.WriteString(";")
	query := queryBuild.String()

	l.Debug("deleting tokens")
	_, err := i.Client.DB.Exec(query, []interface{}{tokens}...)
	if err != nil && err.Error() != "pg: no rows in result set" {
		raven.CaptureError(err, map[string]string{
			"version": util.Version,
			"handler": "invalidToken",
		})

		l.WithError(err).Error("error deleting tokens")
		return err
	}

	return nil
}
