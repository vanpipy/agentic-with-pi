package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

type recordingCore struct {
	chatCalls     int
	chatResponses []*llm.ChatResponse
	chatErrors    []error
	streamCalls   int
	streamEvents  []llm.StreamEvent
	streamErr     error
}

func (r *recordingCore) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	r.chatCalls++
	idx := r.chatCalls - 1
	var err error
	if idx < len(r.chatErrors) {
		err = r.chatErrors[idx]
	}
	var resp *llm.ChatResponse
	if idx < len(r.chatResponses) && r.chatResponses[idx] != nil {
		resp = r.chatResponses[idx]
	}
	return resp, err
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

func TestRetryNoRetryOnSuccess(t *testing.T) {
	inner := &recordingCore{
		chatResponses: []*llm.ChatResponse{{Choices: []llm.Choice{{Message: llm.Message{Content: "ok"}}}}},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	resp, err := rc.Chat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Choices[0].Message.Content)
	}
	if inner.chatCalls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on success)", inner.chatCalls)
	}
}

func TestRetryNoRetryOnPermanentError(t *testing.T) {
	inner := &recordingCore{
		chatErrors: []error{&llm.Error{Kind: llm.ErrorKindAuth, Code: 401}},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	_, err := rc.Chat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.chatCalls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on auth)", inner.chatCalls)
	}
}

func TestRetryRecoversOnTransientError(t *testing.T) {
	inner := &recordingCore{
		chatErrors: []error{
			&llm.Error{Kind: llm.ErrorKindServer, Code: 500},
			&llm.Error{Kind: llm.ErrorKindRateLimit, Code: 429},
			&llm.Error{Kind: llm.ErrorKindNetwork},
		},
		chatResponses: []*llm.ChatResponse{
			nil, nil, nil,
			{Choices: []llm.Choice{{Message: llm.Message{Content: "recovered"}}}},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	resp, err := rc.Chat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.Content != "recovered" {
		t.Errorf("Content = %q, want recovered", resp.Choices[0].Message.Content)
	}
	if inner.chatCalls != 4 {
		t.Errorf("calls = %d, want 4 (1 + 3 retries)", inner.chatCalls)
	}
}

func TestRetryExhaustedReturnsLastError(t *testing.T) {
	lastErr := &llm.Error{Kind: llm.ErrorKindServer, Code: 500, Message: "last"}
	inner := &recordingCore{
		chatErrors: []error{
			&llm.Error{Kind: llm.ErrorKindServer, Code: 500},
			&llm.Error{Kind: llm.ErrorKindServer, Code: 502},
			&llm.Error{Kind: llm.ErrorKindServer, Code: 503},
			lastErr,
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	_, err := rc.Chat(context.Background(), &llm.ChatRequest{})
	if err != lastErr {
		t.Errorf("err = %v, want %v", err, lastErr)
	}
	if inner.chatCalls != 4 {
		t.Errorf("calls = %d, want 4 (1 + 3 retries)", inner.chatCalls)
	}
}

func TestRetryStopsOnNonRetryableMidLoop(t *testing.T) {
	inner := &recordingCore{
		chatErrors: []error{
			&llm.Error{Kind: llm.ErrorKindServer, Code: 500},
			&llm.Error{Kind: llm.ErrorKindClient, Code: 400},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	_, err := rc.Chat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	llmErr, ok := err.(*llm.Error)
	if !ok || llmErr.Kind != llm.ErrorKindClient {
		t.Errorf("err = %v, want client (immediate return)", err)
	}
	if inner.chatCalls != 2 {
		t.Errorf("calls = %d, want 2 (one server, then client — no more retries)", inner.chatCalls)
	}
}

func TestRetryContextCancelStopsRetry(t *testing.T) {
	inner := &recordingCore{
		chatErrors: []error{&llm.Error{Kind: llm.ErrorKindServer, Code: 500}},
	}
	rc := llm.NewRetryCore(inner, llm.RetryConfig{
		MaxRetries:     10,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Multiplier:     2.0,
		Jitter:         0,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := rc.Chat(ctx, &llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected error from context cancel")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if inner.chatCalls != 1 {
		t.Errorf("calls = %d, want 1 (cancelled before first retry)", inner.chatCalls)
	}
}

func TestStreamChatDoesNotRetry(t *testing.T) {
	streamErr := errors.New("stream failed")
	inner := &recordingCore{streamErr: streamErr}
	rc := llm.NewRetryCore(inner, fastConfig(5))

	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != streamErr {
		t.Errorf("err = %v, want %v", err, streamErr)
	}
	if inner.streamCalls != 1 {
		t.Errorf("streamCalls = %d, want 1 (no retry)", inner.streamCalls)
	}
}

func TestStreamChatForwardsEvents(t *testing.T) {
	events := []llm.StreamEvent{
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "hello"}}}}},
		{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: " world"}}}}},
	}
	inner := &recordingCore{streamEvents: events}
	rc := llm.NewRetryCore(inner, fastConfig(5))

	ch, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for ev := range ch {
		for _, c := range ev.Chunk.Choices {
			got = append(got, c.Delta.Content)
		}
	}
	if len(got) != 2 || got[0] != "hello" || got[1] != " world" {
		t.Errorf("got %v, want [hello  world]", got)
	}
}

func TestDefaultRetryConfigHasThreeRetries(t *testing.T) {
	cfg := llm.DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
}
