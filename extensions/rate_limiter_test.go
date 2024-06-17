package extensions

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/topfreegames/pusher/interfaces"
	"github.com/topfreegames/pusher/util"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

var _ = FDescribe("Rate Limiter", func() {
	Describe("[Integration]", func() {
		logger, hook := test.NewNullLogger()
		logger.Level = logrus.DebugLevel
		configFile := os.Getenv("CONFIG_FILE")
		if configFile == "" {
			configFile = "../config/test.yaml"
		}
		config, err := util.NewViperWithConfigFile(configFile)
		Expect(err).NotTo(HaveOccurred())
		hook.Reset()
		game := "test"
		platform := "test"
		statsClients := []interfaces.StatsReporter{}

		Describe("Rate limiting", func() {
			It("should return not-allowed when rate limit is reached", func() {
				rl := NewRateLimiter(1, config, statsClients, logger)
				ctx := context.Background()
				device := uuid.NewString()
				allowed := rl.Allow(ctx, device, game, platform)
				Expect(allowed).To(BeTrue())

				// Should not allow due to reaching limit of 1
				allowed = rl.Allow(ctx, device, game, platform)
				Expect(allowed).To(BeFalse())
			})

			It("should increment current rate if limit is not reached", func() {
				rl := NewRateLimiter(10, config, statsClients, logger)
				ctx := context.Background()
				device := uuid.NewString()
				currMin := time.Now().Minute()

				allowed := rl.Allow(ctx, device, game, platform)
				Expect(allowed).To(BeTrue())

				key := fmt.Sprintf("%s:%d", device, currMin)
				actual, err := rl.redis.Get(ctx, key).Result()
				Expect(err).ToNot(HaveOccurred())
				Expect(actual).To(BeEquivalentTo("1"))
			})

			It("should return allowed if redis fails", func() {
				wrongConfig, err := util.NewViperWithConfigFile(configFile)
				Expect(err).NotTo(HaveOccurred())
				wrongConfig.Set("rateLimiter.redis.host", "unreachable")
				rl := NewRateLimiter(10, wrongConfig, statsClients, logger)
				ctx := context.Background()
				device := uuid.NewString()

				allowed := rl.Allow(ctx, device, game, platform)
				Expect(allowed).To(BeTrue())
			})

		})
	})
})
