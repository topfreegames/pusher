package mocks

import "context"

type rateLimiterMock struct {
}

func NewRateLimiterMock() *rateLimiterMock {
	return &rateLimiterMock{}
}

func (rl *rateLimiterMock) Allow(ctx context.Context, device string) bool {
	return true
}
