package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)


type recordingCore struct {
	streamCalls  int
	streamEvents []llm.StreamEvent
	streamErr    error
}

func (r *recordingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	r.streamCalls++
	if r.streamErr != nil {
		return nil, r.streamErr
	}
	ch := make(chan llm.StreamEvent, len(r.streamEvents))
	for _, ev := range r.streamEvents {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func fastConfig(maxRetries int) llm.RetryConfig {
	return llm.RetryConfig{
		MaxRetries:     maxRetries,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
		Jitter:         0,
	}
}

func okStream(content string) []llm.StreamEvent {
	return []llm.StreamEvent{
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
			Delta: llm.Message{Content: content},
		}}}},
	}
}

func drain(events <-chan llm.StreamEvent) (firstErr error, totalContent string) {
	for ev := range events {
		if ev.Err != nil && firstErr == nil {
			firstErr = ev.Err
		}
		if ev.Chunk != nil {
			for _, c := range ev.Chunk.Choices {
				totalContent += c.Delta.Content
			}
		}
	}
	return
}

func TestRetryStreamForwardsEvents(t *testing.T) {
	inner := &recordingCore{streamEvents: okStream("hello")}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, content := drain(events)
	if content != "hello" {
		t.Errorf("content = %q, want hello", content)
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1", inner.streamCalls)
	}
}

func TestRetryStreamPropagatesStreamError(t *testing.T) {
	inner := &recordingCore{
		streamEvents: []llm.StreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "x"}}}}},
			{Err: errors.New("mid-stream fail")},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, _ := rc.StreamChat(context.Background(), &llm.ChatRequest{})

	firstErr, _ := drain(events)
	if firstErr == nil {
		t.Fatal("expected stream error")
	}
	if firstErr.Error() != "mid-stream fail" {
		t.Errorf("err = %v", firstErr)
	}
}

func TestDefaultRetryConfigHasThreeRetries(t *testing.T) {
	cfg := llm.DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.InitialBackoff <= 0 {
		t.Errorf("InitialBackoff must be > 0")
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		t.Errorf("MaxBackoff < InitialBackoff")
	}
}

func TestRetryCoreConstructorClampsNegative(t *testing.T) {
	cfg := fastConfig(0)
	cfg.MaxRetries = -1
	rc := llm.NewRetryCore(&recordingCore{}, cfg)
	if rc == nil {
		t.Fatal("constructor returned nil")
	}
}

type flakyCore struct {
	failures int
	calls    int
}

func (f *flakyCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, &llm.Error{
			Kind:    llm.ErrorKindServer,
			Code:    529,
			Message: "overloaded_error",
		}
	}
	return okStreamChannel("recovered"), nil
}

func okStreamChannel(content string) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: content},
	}}}}
	close(ch)
	return ch
}

func TestRetryCoreRetriesOnTransientFailure(t *testing.T) {
	inner := &flakyCore{failures: 2}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	_, content := drain(events)
	if content != "recovered" {
		t.Errorf("content = %q, want recovered", content)
	}
	if inner.calls != 3 {
		t.Errorf("calls = %d, want 3 (2 failures + 1 success)", inner.calls)
	}
}

func TestRetryCoreGivesUpAfterMaxRetries(t *testing.T) {
	inner := &flakyCore{failures: 10}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if inner.calls != 4 {
		t.Errorf("calls = %d, want 4 (1 initial + 3 retries)", inner.calls)
	}
}

func TestRetryCoreDoesNotRetryNonRetryableError(t *testing.T) {
	inner := &recordingCore{streamErr: &llm.Error{
		Kind:    llm.ErrorKindAuth,
		Code:    401,
		Message: "invalid api key",
	}}
	rc := llm.NewRetryCore(inner, fastConfig(3))
	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.streamCalls != 1 {
		t.Errorf("calls = %d, want 1 (no retry for auth error)", inner.streamCalls)
	}
}
