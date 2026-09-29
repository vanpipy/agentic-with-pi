package llm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
	"github.com/vanpiyp/awp/internal/llm/retryafter"
)

// redirectTransport rewrites every request's URL to point at target so a real
// HTTPRest can hit an httptest server without DNS rewrites or proxy config.
type redirectTransport struct{ target string }

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(r.target, "http://")
	return http.DefaultTransport.RoundTrip(cloned)
}

const endToEndScriptOK = "data: {\"type\":\"message_stop\"}\n\n"

func TestEndToEndRetryAfterHonoredThroughRealPublicStack(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"type":"rate_limit"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, endToEndScriptOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	rest := protocol.NewHTTPRestWithClient(client)
	p := providers.NewMiniMaxProvider("test-key-not-used")
	c := llm.NewCore(p, rest)
	rc := llm.NewRetryCore(c, llm.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		Multiplier:     2.0,
		Jitter:         0,
	})

	start := time.Now()
	events, err := rc.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error before stream: %v", err)
	}
	for range events {
	}
	elapsed := time.Since(start)

	if got := calls.Load(); got != 2 {
		t.Fatalf("server got %d calls, want 2 (1 fail + 1 success)", got)
	}
	if elapsed < 5*time.Second {
		t.Fatalf("elapsed = %v, want >= 5s (Retry-After: 5 must be honored through real stack)", elapsed)
	}
}

func TestEndToEndRetryAfterHintPropagatesViaHTTPErrorField(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit","message":"slow down"}}`)
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	rest := protocol.NewHTTPRestWithClient(client)
	p := providers.NewMiniMaxProvider("test-key-not-used")
	c := llm.NewCore(p, rest)

	_, err := c.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatalf("StreamChat returned nil error after 429")
	}

	llmErr := llm.Classify(err)
	if llmErr == nil {
		t.Fatalf("Classify returned nil")
	}
	if llmErr.Kind != llm.ErrorKindRateLimit {
		t.Fatalf("Kind = %v, want ErrorKindRateLimit", llmErr.Kind)
	}
	if llmErr.RetryAfter != retryafter.MaxRetryAfter {
		t.Fatalf("RetryAfter = %v, want capped at MaxRetryAfter (120s > 60s cap)", llmErr.RetryAfter)
	}
}
