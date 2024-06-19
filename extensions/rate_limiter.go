package extensions

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/topfreegames/pusher/interfaces"
)

type rateLimiter struct {
	redis          *redis.Client
	rpmLimit       int
	statsReporters []interfaces.StatsReporter
	l              *logrus.Entry
}

func NewRateLimiter(limit int, config *viper.Viper, statsReporters []interfaces.StatsReporter, logger *logrus.Logger) rateLimiter {
	host := config.GetString("rateLimiter.redis.host")
	port := config.GetInt("rateLimiter.redis.port")
	pwd := config.GetString("rateLimiter.redis.password")
	disableTLS := config.GetBool("rateLimiter.tls.disabled")

	addr := fmt.Sprintf("%s:%d", host, port)
	opts := &redis.Options{
		Addr:     addr,
		Password: pwd,
	}

	// TLS for integration tests running in containers can raise connection errors.
	// Not recommended to disable TLS for production.
	if !disableTLS {
		opts.TLSConfig = &tls.Config{}
	}

	rdb := redis.NewClient(opts)

	return rateLimiter{
		redis:          rdb,
		rpmLimit:       limit,
		statsReporters: statsReporters,
		l: logger.WithFields(logrus.Fields{
			"extension": "RateLimiter",
			"rpmLimit":  limit,
		}),
	}
}

// Allow checks Redis for the current rate a given device has in the current minute
// If the rate is lower than the limit, the message is allowed. Otherwise, it is not allowed.
// Reference: https://redis.io/glossary/rate-limiting/
func (r rateLimiter) Allow(ctx context.Context, device string, game string, platform string) bool {
	deviceKey := fmt.Sprintf("%s:%d", device, time.Now().Minute())
	l := r.l.WithFields(logrus.Fields{
		"deviceKey": deviceKey,
		"method":    "Allow",
	})

	val, err := r.redis.Get(ctx, deviceKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		// Something went wrong, return true to avoid blocking notifications.
		l.WithError(err).Error("could not get current rate in redis")
		statsReporterNotificationRateLimitFailed(r.statsReporters, game, platform)
		return true
	}
	if errors.Is(err, redis.Nil) {
		// First time
		val = "0"
	}

	current, err := strconv.Atoi(val)
	if err != nil {
		// Something went wrong, return true to avoid blocking notifications.
		l.WithError(err).Error("current rate is invalid")
		statsReporterNotificationRateLimitFailed(r.statsReporters, game, platform)
		return true
	}

	if current >= r.rpmLimit {
		return false
	}

	_, err = r.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, deviceKey)
		pipe.Expire(ctx, deviceKey, time.Minute)
		return nil
	})
	if err != nil {
		// Allow the operation even if the transaction fails, to avoid blocking notifications.
		l.WithError(err).Error("increment to current rate failed")
		statsReporterNotificationRateLimitFailed(r.statsReporters, game, platform)
	}

	l.WithField("currentRate", current).Debug("current rate allows message")
	return true
}
