package llm

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/vanpiyp/awp/internal/llm/retryafter"
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
	baseMs := r.config.InitialBackoff.Milliseconds()
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
		hint := RetryAfterFromAny(connErr)
		delay := r.computeDelay(attempt+1, baseMs, hint)
		if hint > 0 {
			slog.Debug("llm: retry-after hint honored",
				"attempt", attempt, "hint", hint, "delay", delay)
		}
		if sleepErr := r.sleep(ctx, delay, hint > 0); sleepErr != nil {
			return nil, sleepErr
		}
	}
	out := make(chan LegacyStreamEvent, 32)
	go r.forwardWithMidRetry(ctx, req, ch, out, lastAttempt, baseMs)
	return out, nil
}

func (r *RetryCore) forwardWithMidRetry(ctx context.Context, req *ChatRequest, ch <-chan LegacyStreamEvent, out chan<- LegacyStreamEvent, attempt int, baseMs int64) {
	defer close(out)
	for {
		emittedCount := 0
		midRetry := false
		var lastErr error
		for ev := range ch {
			emittedCount++
			if ev.Err != nil {
				lastErr = ev.Err
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
		hint := RetryAfterFromAny(lastErr)
		delay := r.computeDelay(attempt+1, baseMs, hint)
		if hint > 0 {
			slog.Debug("llm: mid-retry retry-after hint honored",
				"attempt", attempt, "hint", hint, "delay", delay)
		}
		if sleepErr := r.sleep(ctx, delay, hint > 0); sleepErr != nil {
			return
		}
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

func (r *RetryCore) computeDelay(attempt int, baseMs int64, hint time.Duration) time.Duration {
	d := retryafter.RetryDelay(attempt, baseMs, hint)
	if hint <= 0 && d > r.config.MaxBackoff {
		return r.config.MaxBackoff
	}
	return d
}

func (r *RetryCore) sleep(ctx context.Context, delay time.Duration, noJitter bool) error {
	if !noJitter {
		delay = r.jitter(delay)
	}
	timer := time.NewTimer(delay)
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	}
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
