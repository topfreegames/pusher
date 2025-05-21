package extensions

import (
	"context"
	"crypto/tls"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
)

// a different redis DB for dedup keeps dedup keys separate from Rate Limiter keys stored in default DB = 0
const DedupRedisDB = 1
// Dedup struct
type dedup struct {
	redis             *redis.Client
	ttl               time.Duration
	statsReporters    []interfaces.StatsReporter
	l                 *logrus.Entry
	gamePercentages   map[string]int
}

func NewDedup(ttl time.Duration, config *viper.Viper, statsReporters []interfaces.StatsReporter, logger *logrus.Logger) dedup {
	host := config.GetString("dedup.redis.host")
	port := config.GetInt("dedup.redis.port")
	pwd := config.GetString("dedup.redis.password")
	disableTLS := config.GetBool("dedup.tls.disabled")

	addr := fmt.Sprintf("%s:%d", host, port)
	opts := &redis.Options{
		Addr:     addr,
		Password: pwd,
		DB:       DedupRedisDB,
	}

	if !disableTLS {
		opts.TLSConfig = &tls.Config{}
	}

	gamePercentages := make(map[string]int)
	games := config.GetStringMap("dedup.games")
	for game := range games {
		percentageConfigKey := "dedup.games." + game + ".percentage"

		config.SetDefault(percentageConfigKey, 0)
		gamePercentages[game] = config.GetInt(percentageConfigKey)
	}

	rdb := redis.NewClient(opts)

	return dedup{
		redis:          rdb,
		ttl:            ttl,
		statsReporters: statsReporters,
		l: logger.WithFields(logrus.Fields{
			"extension": "Dedup",
			"ttl":       ttl,
		}),
		gamePercentages:   gamePercentages,
	}
}

func (d dedup) IsUnique(ctx context.Context, device, msg, game, platform string) bool {
	d.l.Debug("Checking message deduplication")
	// Get percentage for dedup sampling for specific game
	percentage := d.gamePercentages[game]

	if percentage == 0 {
		d.l.Debug("Deduplication sampling percentage is 0, skipping deduplication check")
		return true
	}

	if percentage > 0 && percentage < 100 {
		d.l.Debug("Deduplication sampling percentage is above 0 and below 100, checking if should sample")
		h := fnv.New64a()
		h.Write([]byte(device))

		sum := h.Sum64()
		// Use the top 16 bits of the hash to determine if we should sample
		sampleValue := int(sum >> 48)
		samplePercentile := sampleValue % 100
		d.l.WithFields(logrus.Fields{
			"sampleValue":      sampleValue,
			"samplePercentile": samplePercentile,
			"percentage":       percentage,
		}).Debug("Deduplication sampling value and percentile")
		if samplePercentile >= percentage {
			d.l.Debug("Deduplication sampling percentage check didn't pass, skipping deduplication check")
			return true
		}
	}

	d.l.Debug("Deduplication sampling percentage check passed, proceeding with deduplication check")
	rdbKey := keyFor(device, msg)

	// Store the key in Redis with a placeholder value ("1")—the actual value is irrelevant, as we only need to check for key existence.
	unique, err := d.redis.SetNX(ctx, rdbKey, "1", d.ttl).Result()

	if err != nil {
		d.l.WithFields(logrus.Fields{
			"error":  err,
			"key":    rdbKey,
			"device": device,
			"game":   game,
		}).Error("Failed to check message dedup in Redis")

		StatsReporterDedupFailed(d.statsReporters, game, platform)

		// Return true on error to avoid blocking messages
		return true
	}

	return unique
}

func keyFor(device, msg string) string {
	h := fnv.New64a()
	h.Write([]byte(device))
	h.Write([]byte("|"))
	h.Write([]byte(msg))
	return strconv.FormatUint(h.Sum64(), 10)
}
