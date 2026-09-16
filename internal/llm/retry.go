package llm

import (
	"context"
	"math/rand"
	"time"
)

type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         float64
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.25,
	}
}

type RetryCore struct {
	inner  Core
	config RetryConfig
}

func NewRetryCore(inner Core, config RetryConfig) *RetryCore {
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	return &RetryCore{inner: inner, config: config}
}

func (r *RetryCore) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	return r.inner.StreamChat(ctx, req)
}

func (r *RetryCore) withRetry(ctx context.Context, op func(context.Context) error) error {
	var lastErr error
	backoff := r.config.InitialBackoff
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !IsRetryable(err) {
			return err
		}
		if attempt == r.config.MaxRetries {
			break
		}
		sleep := r.jitter(backoff)
		timer := time.NewTimer(sleep)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
		next := time.Duration(float64(backoff) * r.config.Multiplier)
		if next > r.config.MaxBackoff {
			next = r.config.MaxBackoff
		}
		backoff = next
	}
	return lastErr
}

func (r *RetryCore) jitter(d time.Duration) time.Duration {
	if r.config.Jitter == 0 {
		return d
	}
	delta := float64(d) * r.config.Jitter
	offset := (rand.Float64()*2 - 1) * delta
	result := time.Duration(float64(d) + offset)
	if result < 0 {
		return d
	}
	return result
}
