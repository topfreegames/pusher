package extensions

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
)

//Dedup struct
type Dedup struct {
	redis *redis.Client
	timeframe time.Duration
	statsReporters []interfaces.StatsReporter
	l              *logrus.Entry
}

func NewDedup(timeframe time.Duration, config *viper.Viper, statsReporters []interfaces.StatsReporter, logger *logrus.Logger) Dedup {
	host := config.GetString("dedup.redis.host")
	port := config.GetInt("dedup.redis.port")
	pwd := config.GetString("dedup.redis.password")
	disableTLS := config.GetBool("dedup.tls.disabled")

	addr := fmt.Sprintf("%s:%d", host, port)
	opts := &redis.Options{
		Addr:     addr,
		Password: pwd,
		DB: 1,
	}

	//receive which game
	//what percentage of messages that will be dedupped
	// how to know if the current message being processed is within the intended % of messages to be analysed

	if !disableTLS {
		opts.TLSConfig = &tls.Config{}
	}

	rdb := redis.NewClient(opts)

	return Dedup{
		redis:          rdb,
		timeframe:      timeframe,
		statsReporters: statsReporters,
		l: logger.WithFields(logrus.Fields{
			"extension": "Dedup",
			"timeframe": timeframe,
		}),
	}
}

func (d Dedup) IsUnique(ctx context.Context, device, msg, game, platform string) bool {
	rdbKey := Sha256Hex(device, msg)

	// Store the key in Redis with a placeholder value ("1")—the actual value is irrelevant, as we only need to check for key existence.
	unique, err := d.redis.SetNX(ctx, rdbKey, "1", d.timeframe).Result()

	if err != nil {
		d.l.WithFields(logrus.Fields{
            "error": err,
            "key": rdbKey,
            "device": device,
            "game": game,
        }).Error("Failed to check message dedup in Redis")
        
        StatsReporterDedupFailed(d.statsReporters, game, platform)
        
        // Return true on error to avoid blocking messages
		return true
	}

	return unique
}


func Sha256Hex(deviceId, msg string) string {
	
	digest := fmt.Sprintf("%s|%s", deviceId, msg)
	h := sha256.New()
	h.Write([]byte(digest))

	return hex.EncodeToString(h.Sum(nil))
}
