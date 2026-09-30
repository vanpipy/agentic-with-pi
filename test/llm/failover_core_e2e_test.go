package llm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/failover"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

// TestEndToEndFailoverSwitchesProviderOnRateLimit drives the FailoverCore
// through real HTTPRest + MiniMaxProvider + httptest: route A returns 429
// forever, route B returns 200 + a real streaming payload. The test asserts
// that FailoverCore escalates to route B after route A exhausts retries,
// and that the synthetic failover prompt is surfaced as a stream error
// error on the output channel before route B's stream drains.
func TestEndToEndFailoverSwitchesProviderOnRateLimit(t *testing.T) {
	var callsA, callsB atomic.Int32

	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsA.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit"}}`)
	}))
	defer srvA.Close()

	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsB.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello from B"}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srvB.Close()

	clientA := &http.Client{Transport: redirectTransport{target: srvA.URL}}
	clientB := &http.Client{Transport: redirectTransport{target: srvB.URL}}

	coreA := llm.NewCore(providers.NewMiniMaxProvider("test-key-A"), protocol.NewHTTPRestWithClient(clientA))
	coreB := llm.NewCore(providers.NewMiniMaxProvider("test-key-B"), protocol.NewHTTPRestWithClient(clientB))

	routes := []llm.FailoverRoute{
		{Core: coreA, Route: failover.ModelRoute{Model: "MiniMax-M3", Provider: "Anthropic", APIMethod: "claude-api", Available: true}},
		{Core: coreB, Route: failover.ModelRoute{Model: "MiniMax-M3", Provider: "OpenAI", APIMethod: "openai-oauth", Available: true}},
	}
	fc := llm.NewFailoverCore(routes, llm.FailoverConfig{
		Retry: llm.RetryConfig{
			MaxRetries:     1,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     5 * time.Millisecond,
			Multiplier:     1.0,
			Jitter:         0,
		},
		MaxRoutes: 2,
	})

	events, err := fc.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error before stream: %v", err)
	}

	var sawFailoverPrompt bool
	var sawChunk bool
	for ev := range events {
		if e, ok := ev.(llm.EventErr); ok {
			if failover.ParseFailoverPromptMessage(e.Err.Error()) != nil {
				sawFailoverPrompt = true
			}
		}
		if _, ok := ev.(llm.EventTextDelta); ok {
			sawChunk = true
		}
	}

	if callsA.Load() < 2 {
		t.Errorf("route A: got %d calls, want >= 2 (retry budget exhausted)", callsA.Load())
	}
	if callsB.Load() != 1 {
		t.Errorf("route B: got %d calls, want 1 (succeeded on first try)", callsB.Load())
	}
	if !sawFailoverPrompt {
		t.Errorf("expected failover prompt surfaced as synthetic error; got none")
	}
	if !sawChunk {
		t.Errorf("expected stream chunk from route B; got none")
	}
}

// TestEndToEndFailoverStopsAfterMaxRoutes verifies the loop gives up once
// MaxRoutes is exhausted and surfaces the last error.
func TestEndToEndFailoverStopsAfterMaxRoutes(t *testing.T) {
	var callsA, callsB atomic.Int32
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsA.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"authentication_error"}}`)
	}))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsB.Add(1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"type":"forbidden"}}`)
	}))
	defer srvB.Close()

	clientA := &http.Client{Transport: redirectTransport{target: srvA.URL}}
	clientB := &http.Client{Transport: redirectTransport{target: srvB.URL}}

	coreA := llm.NewCore(providers.NewMiniMaxProvider("a"), protocol.NewHTTPRestWithClient(clientA))
	coreB := llm.NewCore(providers.NewMiniMaxProvider("b"), protocol.NewHTTPRestWithClient(clientB))

	routes := []llm.FailoverRoute{
		{Core: coreA, Route: failover.ModelRoute{Model: "MiniMax-M3", Provider: "Anthropic", APIMethod: "claude-api", Available: true}},
		{Core: coreB, Route: failover.ModelRoute{Model: "MiniMax-M3", Provider: "OpenAI", APIMethod: "openai-oauth", Available: true}},
	}
	fc := llm.NewFailoverCore(routes, llm.FailoverConfig{
		Retry: llm.RetryConfig{
			MaxRetries:     0,
			InitialBackoff: 1 * time.Millisecond,
			MaxBackoff:     5 * time.Millisecond,
		},
		MaxRoutes: 2,
	})

	events, _ := fc.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})

	var lastErr error
	for ev := range events {
		if e, ok := ev.(llm.EventErr); ok {
			lastErr = e.Err
		}
	}
	if lastErr == nil {
		t.Fatalf("expected final error, got nil")
	}
	if callsA.Load() != 1 {
		t.Errorf("route A: got %d calls, want 1", callsA.Load())
	}
	if callsB.Load() != 1 {
		t.Errorf("route B: got %d calls, want 1", callsB.Load())
	}
}
