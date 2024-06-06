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
)

type rateLimiter struct {
	redis    *redis.Client
	rpmLimit int
	l        *logrus.Entry
}

func NewRateLimiter(config *viper.Viper, logger *logrus.Logger) rateLimiter {
	host := config.GetString("rateLimiter.redis.host")
	port := config.GetInt("rateLimiter.redis.port")
	pwd := config.GetString("rateLimiter.redis.password")
	limit := config.GetInt("rateLimiter.limit.rpm")

	addr := fmt.Sprintf("%s:%d", host, port)
	rdb := redis.NewClient(&redis.Options{
		Addr:      addr,
		Password:  pwd,
		TLSConfig: &tls.Config{},
	})

	return rateLimiter{
		redis:    rdb,
		rpmLimit: limit,
		l: logger.WithFields(logrus.Fields{
			"extension": "RateLimiter",
			"rpmLimit":  limit,
		}),
	}
}

// Allow checks Redis for the current rate a given device has in the current minute
// If the rate is lower than the limit, the message is allowed. Otherwise, it is not allowed.
// Reference: https://redis.io/glossary/rate-limiting/
func (r rateLimiter) Allow(ctx context.Context, device string) bool {
	deviceKey := fmt.Sprintf("%s:%d", device, time.Now().Minute())
	l := r.l.WithFields(logrus.Fields{
		"deviceKey": deviceKey,
		"method":    "Allow",
	})

	val, err := r.redis.Get(ctx, deviceKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		// Something went wrong, return true to avoid blocking notifications.
		l.WithError(err).Error("could not get current rate in redis")
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
		return true
	}

	if current >= r.rpmLimit {
		return false
	}

	_, err = r.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, deviceKey)
		pipe.Expire(ctx, deviceKey, 1*time.Minute)
		return nil
	})
	if err != nil {
		// Allow the operation even if the transaction fails, to avoid blocking notifications.
		l.WithError(err).Error("increment to current rate failed")
	}

	l.WithField("currentRate", current).Debug("current rate allows message")
	return true
}
