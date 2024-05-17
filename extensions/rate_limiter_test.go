package extensions

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter(t *testing.T) {
	rl := rateLimiter{
		redis: redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		}),
		limit: 1,
	}
	ctx := context.Background()
	t.Run("should return not-allowed when rate limit is reached", func(t *testing.T) {
		device := uuid.NewString()

		allowed := rl.Allow(ctx, device)
		assert.True(t, allowed)

		// Should not allow due to reaching limit of 1
		allowed = rl.Allow(ctx, device)
		assert.False(t, allowed)
	})

	t.Run("should increment current rate if limit is not reached", func(t *testing.T) {
		device := uuid.NewString()
		currMin := time.Now().Minute()

		allowed := rl.Allow(ctx, device)
		assert.True(t, allowed)

		key := fmt.Sprintf("%s:%d", device, currMin)
		actual, err := rl.redis.Get(ctx, key).Result()
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, "1", actual)

	})
}
