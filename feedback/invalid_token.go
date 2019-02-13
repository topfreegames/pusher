package feedback

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/extensions"
	"github.com/topfreegames/pusher/interfaces"
)

type InvalidToken struct {
	Token    string
	Game     string
	Platform string
}

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
	i.Config.SetDefault("invalidToken.flush.time.ms", 5000)
	i.Config.SetDefault("invalidToken.buffer.size", 1000)
}

func (i *InvalidTokenHandler) configure(db interfaces.DB) error {
	l := i.Logger.WithField("operation", "configure")
	i.loadConfigurationDefaults()

	flushTime := time.Duration(i.Config.GetInt("invalidToken.flush.time.ms")) * time.Millisecond
	i.bufferSize = i.Config.GetInt("invalidToken.buffer.size")
	fmt.Println("flush time:", flushTime)
	fmt.Println("buffre size:", i.bufferSize)

	i.FlushTicker = time.NewTicker(flushTime)
	i.Buffer = make([]*InvalidToken, 0, i.bufferSize)

	var err error
	i.Client, err = extensions.NewPGClient("invalidToken.pg", i.Config, db)
	if err != nil {
		l.WithError(err).Error("failed to configure psql database")
		return err
	}

	l.Info("psql database configured")
	return nil
}

func (i *InvalidTokenHandler) Start() {
	i.run = true
}

func (i *InvalidTokenHandler) processMessages() {
	for i.run {
		select {
		case tk := <-*i.InChan:
			i.Buffer = append(i.Buffer, tk)

			if len(i.Buffer) >= i.bufferSize {
				go i.deleteTokens(i.Buffer)
				i.Buffer = make([]*InvalidToken, 0, i.bufferSize)
			}

		case <-i.FlushTicker.C:
			go i.deleteTokens(i.Buffer)
			i.Buffer = make([]*InvalidToken, 0, i.bufferSize)

		case <-i.stopChan:
			break
		}
	}
}

func (i *InvalidTokenHandler) deleteTokens(tokens []*InvalidToken) {
	// l := i.Logger.WithFields(log.Fields{
	// 	"operation": "deleteTokens",
	// })

}
