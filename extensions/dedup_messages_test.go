package extensions

import (
	"context"
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
        config.Set("dedup.tls.disabled", true)
        
        timeframe := 10 * time.Minute
        d := NewDedup(timeframe, config, statsReporters, logger)
        
        assert.Equal(t, timeframe, d.timeframe)
        assert.NotNil(t, d.redis)
        assert.Equal(t, statsReporters, d.statsReporters)
    })
}

func TestDedupIsUnique(t *testing.T) {
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
        config.Set("dedup.tls.disabled", true)
        
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
        config.Set("dedup.tls.disabled", true)
        
        d := NewDedup(10*time.Minute, config, statsReporters, logger)
        
        // First message should be unique
        unique1 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
        assert.True(t, unique1)
            
        // Second identical message should be detected as duplicate
        unique2 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
        assert.False(t, unique2)
        
        hook.Reset()
    })
    
    t.Run("respects configured timeframe", func(t *testing.T) {
        mr, err := miniredis.Run()
        require.NoError(t, err)
        defer mr.Close()
        
        config := viper.New()
        config.Set("dedup.redis.host", mr.Host())
        config.Set("dedup.redis.port", mr.Port())
        config.Set("dedup.redis.password", "")
        config.Set("dedup.tls.disabled", true)
        
        // Very short timeframe for testing
        d := NewDedup(50*time.Millisecond, config, statsReporters, logger)
        
        // First message should be unique
        unique1 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
        assert.True(t, unique1)
        
        // Wait for expiration
		mr.FastForward(d.timeframe + 1*time.Millisecond)
        
        // After expiration, same message should be unique again
        unique2 := d.IsUnique(context.Background(), "device123", "message123", "game", "platform")
        assert.True(t, unique2)
        
        hook.Reset()
    })
}

func TestSha256Hex(t *testing.T) {
    t.Run("generates consistent hashes", func(t *testing.T) {
        hash1 := Sha256Hex("device123", "message123")
        hash2 := Sha256Hex("device123", "message123")
        assert.Equal(t, hash1, hash2)
    })
    
    t.Run("different inputs produce different hashes", func(t *testing.T) {
        hash1 := Sha256Hex("device123", "message123")
        hash2 := Sha256Hex("device123", "message456")
        hash3 := Sha256Hex("device456", "message123")
        
        assert.NotEqual(t, hash1, hash2)
        assert.NotEqual(t, hash1, hash3)
        assert.NotEqual(t, hash2, hash3)
    })
}
