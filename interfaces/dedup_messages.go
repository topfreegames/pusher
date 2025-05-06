package interfaces

import (
	"context"
)

// Dedup interface for deduplicating notifications per device.
type Dedup interface {
	IsUnique(ctx context.Context, device, msg, game, platform string) bool
}
