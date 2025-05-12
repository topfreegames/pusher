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

// Dedup struct
type dedup struct {
	redis             *redis.Client
	ttl         time.Duration
	statsReporters    []interfaces.StatsReporter
	l                 *logrus.Entry
	defaultPercentage int
	gamePercentages   map[string]int
}

func NewDedup(ttl time.Duration, config *viper.Viper, statsReporters []interfaces.StatsReporter, logger *logrus.Logger) dedup {
	host := config.GetString("dedup.redis.host")
	port := config.GetInt("dedup.redis.port")
	pwd := config.GetString("dedup.redis.password")
	enableTLS := config.GetBool("dedup.tls.enabled")

	addr := fmt.Sprintf("%s:%d", host, port)
	opts := &redis.Options{
		Addr:     addr,
		Password: pwd,
		DB:       1,
	}

	if enableTLS {
		opts.TLSConfig = &tls.Config{}
	}

	defaultPercentage := config.GetInt("dedup.default_percentage")
	if defaultPercentage < 0 {
		defaultPercentage = 0
	}

	gamePercentages := make(map[string]int)
	games := config.GetStringMap("dedup.games")
	for game := range games {
		if config.IsSet("dedup.games." + game + ".percentage") {
			gamePercentages[game] = config.GetInt("dedup.games." + game + ".percentage")
		}
	}

	rdb := redis.NewClient(opts)

	return dedup{
		redis:          rdb,
		ttl:      ttl,
		statsReporters: statsReporters,
		l: logger.WithFields(logrus.Fields{
			"extension": "Dedup",
			"ttl": ttl,
		}),
		defaultPercentage: defaultPercentage,
		gamePercentages:   gamePercentages,
	}
}

func (d dedup) IsUnique(ctx context.Context, device, msg, game, platform string) bool {

	// Get percentage for dedup sampling for specific game
	percentage := d.defaultPercentage

	if p, exists := d.gamePercentages[game]; exists {
		percentage = p
	}

	if percentage == 0 {
		return true
	}

	if percentage < 100 {
		h := sha256.Sum256([]byte(device))
		sampleValue := int(h[0])*256 + int(h[1])

		samplePercentile := sampleValue % 100

		if samplePercentile >= percentage {
			return true
		}
	}

	rdbKey := Sha256Hex(device, msg)

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

func Sha256Hex(deviceId, msg string) string {

	digest := fmt.Sprintf("%s|%s", deviceId, msg)
	h := sha256.New()
	h.Write([]byte(digest))

	return hex.EncodeToString(h.Sum(nil))
}
