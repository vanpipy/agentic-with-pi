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

func (r *RetryCore) StreamChat(ctx context.Context, req *ChatRequest) (<-chan LegacyStreamEvent, error) {
	backoff := r.config.InitialBackoff
	var (
		ch          <-chan LegacyStreamEvent
		connErr     error
		lastAttempt int
	)
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ch, connErr = r.inner.StreamChat(ctx, req)
		if connErr == nil {
			lastAttempt = attempt
			break
		}
		if !IsRetryable(connErr) || attempt == r.config.MaxRetries {
			return nil, connErr
		}
		if sleepErr := r.sleepBackoff(ctx, backoff); sleepErr != nil {
			return nil, sleepErr
		}
		backoff = r.growBackoff(backoff)
	}
	out := make(chan LegacyStreamEvent, 32)
	go r.forwardWithMidRetry(ctx, req, ch, out, lastAttempt, backoff)
	return out, nil
}

func (r *RetryCore) forwardWithMidRetry(ctx context.Context, req *ChatRequest, ch <-chan LegacyStreamEvent, out chan<- LegacyStreamEvent, attempt int, backoff time.Duration) {
	defer close(out)
	for {
		emittedCount := 0
		midRetry := false
		for ev := range ch {
			emittedCount++
			if ev.Err != nil {
				if emittedCount > 1 && IsRetryable(ev.Err) && attempt < r.config.MaxRetries {
					select {
					case out <- LegacyStreamEvent{Rollback: true}:
					case <-ctx.Done():
						return
					}
					midRetry = true
				} else {
					select {
					case out <- LegacyStreamEvent{Err: ev.Err}:
					case <-ctx.Done():
					}
					return
				}
				break
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
		if !midRetry {
			return
		}
		if sleepErr := r.sleepBackoff(ctx, backoff); sleepErr != nil {
			return
		}
		backoff = r.growBackoff(backoff)
		attempt++
		if attempt > r.config.MaxRetries {
			return
		}
		if err := ctx.Err(); err != nil {
			return
		}
		var connErr error
		ch, connErr = r.inner.StreamChat(ctx, req)
		if connErr != nil {
			select {
			case out <- LegacyStreamEvent{Err: connErr}:
			case <-ctx.Done():
			}
			return
		}
	}
}

func (r *RetryCore) sleepBackoff(ctx context.Context, backoff time.Duration) error {
	sleep := r.jitter(backoff)
	timer := time.NewTimer(sleep)
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	}
}

func (r *RetryCore) growBackoff(backoff time.Duration) time.Duration {
	next := time.Duration(float64(backoff) * r.config.Multiplier)
	if next > r.config.MaxBackoff {
		return r.config.MaxBackoff
	}
	return next
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
