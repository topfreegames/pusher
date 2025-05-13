package extensions

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/topfreegames/pusher/interfaces"
	mock_interfaces "github.com/topfreegames/pusher/mocks/interfaces"
	"go.uber.org/mock/gomock"
)

func TestNewDedup(t *testing.T) {
	// Setup
	logger, _ := test.NewNullLogger()
	logger.Level = logrus.DebugLevel
	ctrl := gomock.NewController(t)
	mockStatsReporter := mock_interfaces.NewMockStatsReporter(ctrl)
	statsReporters := []interfaces.StatsReporter{mockStatsReporter}

	t.Run("initializes with correct default config", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		config := viper.New()
		config.Set("dedup.redis.host", mr.Host())
		config.Set("dedup.redis.port", mr.Port())
		config.Set("dedup.redis.password", "")
		config.Set("dedup.tls.enabled", false)

		dedupTtl := 10 * time.Minute
		d := NewDedup(dedupTtl, config, statsReporters, logger)

		assert.Equal(t, dedupTtl, d.ttl)
		assert.NotNil(t, d.redis)
		assert.Equal(t, statsReporters, d.statsReporters)
	})
}

func TestIsUnique(t *testing.T) {
	// Setup
	logger, hook := test.NewNullLogger()
	logger.Level = logrus.DebugLevel
	ctrl := gomock.NewController(t)
	mockStatsReporter := mock_interfaces.NewMockStatsReporter(ctrl)
	statsReporters := []interfaces.StatsReporter{mockStatsReporter}

	t.Run("returns true for first-time messages", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		config := viper.New()
		config.Set("dedup.redis.host", mr.Host())
		config.Set("dedup.redis.port", mr.Port())
		config.Set("dedup.redis.password", "")
		config.Set("dedup.tls.enabled", false)

		d := NewDedup(10*time.Minute, config, statsReporters, logger)

		// First attempt should be unique
		unique := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
		assert.True(t, unique)

		// No metrics should be reported for unique messages
		hook.Reset()
	})

	t.Run("returns false for duplicate messages", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		config := viper.New()
		config.Set("dedup.redis.host", mr.Host())
		config.Set("dedup.redis.port", mr.Port())
		config.Set("dedup.redis.password", "")
		config.Set("dedup.tls.enabled", false)
		config.Set("dedup.default_percentage", 100) // 100% sampling

		d := NewDedup(10*time.Minute, config, statsReporters, logger)

		// First message should be unique
		unique1 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
		assert.True(t, unique1)

		// Second identical message should be detected as duplicate
		unique2 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
		assert.False(t, unique2)

		hook.Reset()
	})

	t.Run("respects configured ttl", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		config := viper.New()
		config.Set("dedup.redis.host", mr.Host())
		config.Set("dedup.redis.port", mr.Port())
		config.Set("dedup.redis.password", "")
		config.Set("dedup.tls.enabled", false)

		// Very short ttl for testing
		d := NewDedup(50*time.Millisecond, config, statsReporters, logger)

		// First message should be unique
		unique1 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
		assert.True(t, unique1)

		// Wait for expiration
		mr.FastForward(d.ttl + 1*time.Millisecond)

		// After expiration, same message should be unique again
		unique2 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
		assert.True(t, unique2)

		hook.Reset()
	})

	t.Run("respects sampling percentage", func(t *testing.T) {
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()

		config := viper.New()
		config.Set("dedup.redis.host", mr.Host())
		config.Set("dedup.redis.port", mr.Port())
		config.Set("dedup.redis.password", "")
		config.Set("dedup.tls.enabled", false)
		config.Set("dedup.default_percentage", 50) // 50% sampling

		d := NewDedup(10*time.Minute, config, statsReporters, logger)

		// Test multiple devices to verify sampling distribution
		sampledCount := 0
		totalDevices := 1000

		for i := 0; i < totalDevices; i++ {
			device := fmt.Sprintf("%d-test-device", i)
			message := "test-message"

			// Get keys count before
			keyCountBefore := len(mr.DB(1).Keys())

			// Call IsUnique
			d.IsUnique(context.Background(), device, message, "game", "platform")

			// Check if Redis was actually called (key was created)
			keyCountAfter := len(mr.DB(1).Keys())
			if keyCountAfter > keyCountBefore {
				sampledCount++
			}
		}

		// Sampling should be approximately 50%, allow some variance
		percentage := float64(sampledCount) / float64(totalDevices) * 100

		t.Logf("Sampled %d out of %d devices, percentage: %.2f%%", sampledCount, totalDevices, percentage)
		assert.InDelta(t, 50.0, percentage, 15.0, "Expected approximately 50% sampling")
	})
}

func TestKeyFor(t *testing.T) {
	t.Run("generates consistent hashes", func(t *testing.T) {
		hash1 := keyFor("device123", "message123")
		hash2 := keyFor("device123", "message123")
		assert.Equal(t, hash1, hash2)
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := keyFor("device123", "message123")
		hash2 := keyFor("device123", "message456")
		hash3 := keyFor("device456", "message123")

		assert.NotEqual(t, hash1, hash2)
		assert.NotEqual(t, hash1, hash3)
		assert.NotEqual(t, hash2, hash3)
	})
}
