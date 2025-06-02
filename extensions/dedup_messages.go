package extensions

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
)

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

	log := logger.WithFields(logrus.Fields{
		"extension": "Dedup",
		"host":      host,
		"port":      port,
		"disableTLS": disableTLS,
	})

	addr := fmt.Sprintf("%s:%d", host, port)
	opts := &redis.Options{
		Addr:     addr,
		Password: pwd,
	}

	if !disableTLS {
		opts.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	log.Debug("Instantiating Redis client for deduplication")

	gamesRaw := []byte(config.GetString("dedup.games"))

	var gamePercentages map[string]int
	err := json.Unmarshal(gamesRaw, &gamePercentages)

	if err != nil {
		log.WithFields(logrus.Fields{
			"error": err,
		}).Error("Failed to unmarshal dedup games config")
	}

	rdb := redis.NewClient(opts)

	return dedup{
		redis:          rdb,
		ttl:            ttl,
		statsReporters: statsReporters,
		l: log,
		gamePercentages:   gamePercentages,
	}
}

func (d dedup) IsUnique(ctx context.Context, device, msg, game, platform string) bool {
	// Get percentage for dedup sampling for specific game
	percentage := d.gamePercentages[game]

	log := d.l.WithFields(logrus.Fields{
		"percentage": percentage,
		"device":    device,
		"msg":      msg,
		"game":     game,
	})

	if percentage == 0 {
		log.Debug("Deduplication sampling percentage is 0, skipping deduplication check")
		return true
	}

	if percentage > 0 && percentage < 100 {
		h := fnv.New64a()
		h.Write([]byte(device))

		sum := h.Sum64()
		// Use the top 16 bits of the hash to determine if we should sample
		sampleValue := int(sum >> 48)
		samplePercentile := sampleValue % 100
		log.WithFields(logrus.Fields{
			"sampleValue":      sampleValue,
			"samplePercentile": samplePercentile,
			"percentage":       percentage,
		}).Debug("Deduplication sampling value and percentile")
		if samplePercentile >= percentage {
			log.Debug("Deduplication sampling percentage check didn't pass, skipping deduplication check")
			return true
		}
	}

	rdbKey := keyFor(device, msg)

	rArgs := &redis.SetArgs{
		Mode: `NX`, 
		TTL:  d.ttl,
	}

	// Store the key in Redis with a placeholder value ("1")—the actual value is irrelevant, as we only need to check for key existence.
	unique, err := d.redis.SetArgs(ctx, rdbKey, "1", *rArgs).Result();
	
	if err != nil {
		if errors.Is(err, redis.Nil) {
			log.WithField("key", rdbKey).Debug("Message is not unique, key already exists in Redis")
			return false

		} else {
			log.WithFields(logrus.Fields{
				"error": err,
				"key":   rdbKey,
			}).Error("Failed to check uniqueness in Redis")
			return true // Allow the operation to proceed even if Redis check fails, to avoid blocking notifications.
		}
	}

	if unique == "OK" {
		log.WithField("key", rdbKey).Debug("Message is unique, key created in Redis")
		return true
	}

	return false
}

func keyFor(device, msg string) string {
	h := fnv.New64a()
	h.Write([]byte(device))
	h.Write([]byte("|"))
	h.Write([]byte(msg))
	return strconv.FormatUint(h.Sum64(), 10)
}
