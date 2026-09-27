package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestRetryCoreContextCanceledBeforeRetry(t *testing.T) {
	inner := &flakyCore{failures: 100}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rc.StreamChat(ctx, &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if inner.calls > 1 {
		t.Errorf("calls = %d, want <=1 when ctx already canceled", inner.calls)
	}
}

func TestRetryCoreContextCanceledDuringSleep(t *testing.T) {
	inner := &flakyCore{failures: 100}
	cfg := llm.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 500 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		Multiplier:     1.0,
		Jitter:         0,
	}
	rc := llm.NewRetryCore(inner, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := rc.StreamChat(ctx, &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected context deadline error")
	}
}

func TestRetryCoreMaxBackoffCapsBackoff(t *testing.T) {
	inner := &flakyCore{failures: 100}
	cfg := llm.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     150 * time.Millisecond,
		Multiplier:     100.0,
		Jitter:         0,
	}
	rc := llm.NewRetryCore(inner, cfg)

	start := time.Now()
	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if inner.calls != 6 {
		t.Errorf("calls = %d, want 6 (1 initial + 5 retries)", inner.calls)
	}
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, expected MaxBackoff to cap backoff growth", elapsed)
	}
}

func TestRetryCoreJitterPositive(t *testing.T) {
	inner := &flakyCore{failures: 100}
	cfg := llm.RetryConfig{
		MaxRetries:     1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
		Jitter:         0.5,
	}
	rc := llm.NewRetryCore(inner, cfg)

	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if inner.calls != 2 {
		t.Errorf("calls = %d, want 2", inner.calls)
	}
}

func TestRetryCoreJitterZeroNoOffset(t *testing.T) {
	inner := &flakyCore{failures: 100}
	cfg := llm.RetryConfig{
		MaxRetries:     1,
		InitialBackoff: 1 * time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     1.0,
		Jitter:         0,
	}
	rc := llm.NewRetryCore(inner, cfg)

	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
}

func TestRetryCoreStreamChatNonRetryableReturnsImmediately(t *testing.T) {
	inner := &recordingCore{streamErr: &llm.Error{
		Kind:    llm.ErrorKindClient,
		Code:    400,
		Message: "bad request",
	}}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1", inner.streamCalls)
	}
}

func TestRetryCoreUnknownErrorNotRetried(t *testing.T) {
	inner := &recordingCore{streamErr: errors.New("random")}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1 (unknown error not retried)", inner.streamCalls)
	}
}

func TestRetryCoreMaxRetriesZeroStillAttemptsOnce(t *testing.T) {
	inner := &flakyCore{failures: 5}
	rc := llm.NewRetryCore(inner, fastConfig(0))

	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retries with MaxRetries=0)", inner.calls)
	}
}
