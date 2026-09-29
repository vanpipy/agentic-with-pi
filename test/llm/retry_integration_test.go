package llm_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/retryafter"
)

// scriptedFailingCore returns a transport-level HTTPError once, then succeeds.
type scriptedFailingCore struct {
	calls    int
	failOnce error
	script   []llm.LegacyStreamEvent
}

func (s *scriptedFailingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.LegacyStreamEvent, error) {
	s.calls++
	if s.calls == 1 && s.failOnce != nil {
		return nil, s.failOnce
	}
	ch := make(chan llm.LegacyStreamEvent, len(s.script)+1)
	for _, ev := range s.script {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

// scriptedMidStreamCore emits `pre`, then an error event carrying `streamErr`.
// On the second call it emits `post` cleanly. Exercises forwardWithMidRetry.
type scriptedMidStreamCore struct {
	calls     int
	streamErr error
	pre       []llm.LegacyStreamEvent
	post      []llm.LegacyStreamEvent
}

func (s *scriptedMidStreamCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.LegacyStreamEvent, error) {
	s.calls++
	var events []llm.LegacyStreamEvent
	if s.calls == 1 {
		events = s.pre
	} else {
		events = s.post
	}
	ch := make(chan llm.LegacyStreamEvent, len(events)+1)
	for _, ev := range events {
		ch <- ev
	}
	if s.calls == 1 && s.streamErr != nil {
		ch <- llm.LegacyStreamEvent{Err: s.streamErr}
	}
	close(ch)
	return ch, nil
}

func TestRetryAfterHTTPErrorCapturesRetryAfterHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit","message":"slow down"}}`)
	}))
	defer srv.Close()

	rest := protocol.NewHTTPRest()
	_, err := rest.Send(context.Background(), &protocol.Request{
		URL: srv.URL + "/v1/messages",
	})
	var httpErr *protocol.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusTooManyRequests)
	}
	if httpErr.RetryAfter != 5*time.Second {
		t.Fatalf("RetryAfter = %v, want 5s", httpErr.RetryAfter)
	}
}

func TestRetryAfterHTTPErrorHandlesMissingHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit"}}`)
	}))
	defer srv.Close()

	rest := protocol.NewHTTPRest()
	_, err := rest.Send(context.Background(), &protocol.Request{URL: srv.URL + "/v1/messages"})
	var httpErr *protocol.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.RetryAfter != 0 {
		t.Fatalf("RetryAfter = %v, want 0 (no header)", httpErr.RetryAfter)
	}
}

func TestRetryAfterHTTPErrorCapsOversizedHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "999999")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit"}}`)
	}))
	defer srv.Close()

	rest := protocol.NewHTTPRest()
	_, err := rest.Send(context.Background(), &protocol.Request{URL: srv.URL + "/v1/messages"})
	var httpErr *protocol.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T", err)
	}
	if httpErr.RetryAfter != retryafter.MaxRetryAfter {
		t.Fatalf("RetryAfter = %v, want %v", httpErr.RetryAfter, retryafter.MaxRetryAfter)
	}
}

func TestRetryAfterHTTPErrorStreamCapturesRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"type":"overloaded"}}`)
	}))
	defer srv.Close()

	rest := protocol.NewHTTPRest()
	_, err := rest.Stream(context.Background(), &protocol.Request{URL: srv.URL + "/v1/messages"})
	var httpErr *protocol.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError from Stream, got %T", err)
	}
	if httpErr.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %v, want 3s", httpErr.RetryAfter)
	}
}

func TestClassifyPopulatesRetryAfterFromHTTPError(t *testing.T) {
	httpErr := &protocol.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		Body:       []byte(`rate limit`),
		RetryAfter: 7 * time.Second,
	}
	e := llm.Classify(httpErr)
	if e == nil {
		t.Fatalf("Classify returned nil")
	}
	if e.Kind != llm.ErrorKindRateLimit {
		t.Fatalf("Kind = %v, want ErrorKindRateLimit", e.Kind)
	}
	if e.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter = %v, want 7s", e.RetryAfter)
	}
	if got := e.Error(); !strings.Contains(got, "retry-after: 7s") {
		t.Fatalf("Error() = %q, expected to contain %q", got, "retry-after: 7s")
	}
}

func TestRetryAfterFromAnyExtractsFromHTTPError(t *testing.T) {
	httpErr := &protocol.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		Body:       []byte(`rate limit`),
		RetryAfter: 9 * time.Second,
	}
	got := llm.RetryAfterFromAny(httpErr)
	if got != 9*time.Second {
		t.Fatalf("RetryAfterFromAny = %v, want 9s", got)
	}
}

func TestRetryAfterFromAnyPrefersExplicitWrapOverHTTPField(t *testing.T) {
	httpErr := &protocol.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 1 * time.Second,
	}
	wrapped := retryafter.WithRetryAfter(httpErr, 20*time.Second)
	got := llm.RetryAfterFromAny(wrapped)
	if got < 19*time.Second || got > 20*time.Second {
		t.Fatalf("RetryAfterFromAny = %v, want ~20s (explicit wrap wins)", got)
	}
}

func TestRetryCoreHonorsServerHintInBackoff(t *testing.T) {
	hint := 150 * time.Millisecond
	httpErr := &protocol.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		Body:       []byte(`rate limit`),
		RetryAfter: hint,
	}
	inner := &scriptedFailingCore{
		failOnce: httpErr,
		script: []llm.LegacyStreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "ok"}}}}},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	start := time.Now()
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("StreamChat = %v", err)
	}
	for range events {
	}
	elapsed := time.Since(start)

	if elapsed < hint {
		t.Fatalf("elapsed = %v, want >= %v (hint must be honored)", elapsed, hint)
	}
	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2 (1 fail + 1 success)", inner.calls)
	}
}

func TestRetryCoreFallsBackToExponentialWhenNoHint(t *testing.T) {
	inner := &scriptedFailingCore{
		failOnce: errors.New("connection reset by peer"),
		script: []llm.LegacyStreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "ok"}}}}},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	start := time.Now()
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("StreamChat = %v", err)
	}
	for range events {
	}
	elapsed := time.Since(start)

	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2", inner.calls)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, want fast (no hint, small backoff)", elapsed)
	}
}

func TestRetryCoreMidStreamRollbackHonorsRetryAfterHint(t *testing.T) {
	hint := 100 * time.Millisecond
	httpErr := &protocol.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		Body:       []byte(`rate limit`),
		RetryAfter: hint,
	}
	inner := &scriptedMidStreamCore{
		streamErr: httpErr,
		pre: []llm.LegacyStreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "partial"}}}}},
		},
		post: []llm.LegacyStreamEvent{
			{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "ok"}}}}},
		},
	}
	rc := llm.NewRetryCore(inner, fastConfig(3))

	start := time.Now()
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	if err != nil {
		t.Fatalf("StreamChat = %v", err)
	}
	var got []llm.LegacyStreamEvent
	for ev := range events {
		got = append(got, ev)
	}
	elapsed := time.Since(start)

	if elapsed < hint {
		t.Fatalf("elapsed = %v, want >= %v (mid-stream hint must be honored)", elapsed, hint)
	}
	if len(got) < 2 {
		t.Fatalf("got %d events, want at least 2 (rollback + post-retry content)", len(got))
	}
	if !got[1].Rollback {
		t.Fatalf("got[1].Rollback = false, want true (rollback emitted before retry)")
	}
}

func TestRetryCoreGivesUpWhenHintStillExceedsMaxRetries(t *testing.T) {
	httpErr := &protocol.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		RetryAfter: 1 * time.Second,
	}
	inner := &scriptedFailingCore{failOnce: httpErr}
	rc := llm.NewRetryCore(inner, fastConfig(0))

	start := time.Now()
	_, err := rc.StreamChat(context.Background(), &llm.ChatRequest{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error when MaxRetries=0")
	}
	if inner.calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retries allowed)", inner.calls)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("elapsed = %v, want near 0 (no retries)", elapsed)
	}
}
