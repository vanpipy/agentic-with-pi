package llm

import (
	"context"

	"golang.org/x/time/rate"
)

type RateLimitConfig struct {
	RatePerSec float64
	Burst      int
}

type rateLimitedCore struct {
	inner Core
	lim   *rate.Limiter
}

func NewRateLimitedCore(inner Core, cfg RateLimitConfig) Core {
	rps := cfg.RatePerSec
	if rps <= 0 {
		rps = 5
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = 1
	}
	return &rateLimitedCore{
		inner: inner,
		lim:   rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (c *rateLimitedCore) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	if err := c.lim.Wait(ctx); err != nil {
		return nil, err
	}
	return c.inner.StreamChat(ctx, req)
}
