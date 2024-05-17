package extensions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type rateLimiter struct {
	redis *redis.Client
	limit int
	l     *logrus.Entry
}

func NewRateLimiter(config *viper.Viper, logger *logrus.Logger) rateLimiter {
	addr := config.GetString("rateLimiter.redis.host")
	pwd := config.GetString("rateLimiter.redis.password")
	limit := config.GetInt("rateLimiter.limit")

	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: pwd,
	})

	return rateLimiter{
		redis: rdb,
		limit: limit,
		l: logger.WithFields(logrus.Fields{
			"extension": "RateLimiter",
			"limit":     limit,
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
		l.WithError(err).Info("could not get current rate in redis")
		return true
	}
	if errors.Is(err, redis.Nil) {
		// First time
		val = "0"
	}

	current, err := strconv.Atoi(val)
	if err != nil {
		// Something went wrong, return true to avoid blocking notifications.
		l.WithError(err).Info("current rate is invalid")
		return true
	}

	if current >= r.limit {
		return false
	}

	_, err = r.redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, deviceKey)
		pipe.Expire(ctx, deviceKey, 1*time.Minute)
		return nil
	})
	if err != nil {
		// Allow the operation even if the transaction fails, to avoid blocking notifications.
		l.WithError(err).Info("increment to current rate failed")
	}

	return true
}
