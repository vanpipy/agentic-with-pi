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

// TestEndToEndFailoverAcrossDistinctProviders drives FailoverCore through
// real HTTPRest where route A is a MiniMaxProvider returning 429 and
// route B is an AnthropicProvider returning 200 + valid SSE. Verifies the
// core escalates across distinct providers (not just distinct keys on
// the same provider).
func TestEndToEndFailoverAcrossDistinctProviders(t *testing.T) {
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
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello from anthropic"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":3,"output_tokens":4}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srvB.Close()

	clientA := &http.Client{Transport: redirectTransport{target: srvA.URL}}
	clientB := &http.Client{Transport: redirectTransport{target: srvB.URL}}

	coreA := llm.NewCore(providers.NewMiniMaxProvider("mm-key"), protocol.NewHTTPRestWithClient(clientA))
	coreB := llm.NewCore(providers.NewAnthropicProvider("ap-key"), protocol.NewHTTPRestWithClient(clientB))

	routes := []llm.FailoverRoute{
		{Core: coreA, Route: failover.ModelRoute{Model: "claude-haiku-4-5", Provider: "minimax", APIMethod: "claude-api", Available: true}},
		{Core: coreB, Route: failover.ModelRoute{Model: "claude-haiku-4-5", Provider: "anthropic", APIMethod: "anthropic-api-key", Available: true}},
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
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat returned error before stream: %v", err)
	}

	var sawFailoverPrompt bool
	var sawChunk bool
	var sawContent string
	for ev := range events {
		if ev.Err != nil {
			if failover.ParseFailoverPromptMessage(ev.Err.Error()) != nil {
				sawFailoverPrompt = true
			}
		}
		if ev.Chunk != nil {
			sawChunk = true
			for _, ch := range ev.Chunk.Choices {
				if ch.Delta.Content != "" {
					sawContent += ch.Delta.Content
				}
			}
		}
	}

	if callsA.Load() < 2 {
		t.Errorf("route A (minimax): got %d calls, want >= 2 (retry budget exhausted)", callsA.Load())
	}
	if callsB.Load() != 1 {
		t.Errorf("route B (anthropic): got %d calls, want 1 (succeeded on first try)", callsB.Load())
	}
	if !sawFailoverPrompt {
		t.Errorf("expected failover prompt surfaced as synthetic error; got none")
	}
	if !sawChunk {
		t.Errorf("expected stream chunk from route B; got none")
	}
	if sawContent != "hello from anthropic" {
		t.Errorf("content = %q, want %q", sawContent, "hello from anthropic")
	}
}

// TestEndToEndFailoverDistinctProvidersBothFail verifies that when both
// distinct-provider routes fail, FailoverCore returns the last error
// rather than panicking or hanging.
func TestEndToEndFailoverDistinctProvidersBothFail(t *testing.T) {
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
	coreB := llm.NewCore(providers.NewAnthropicProvider("b"), protocol.NewHTTPRestWithClient(clientB))

	routes := []llm.FailoverRoute{
		{Core: coreA, Route: failover.ModelRoute{Model: "claude-haiku-4-5", Provider: "minimax", APIMethod: "claude-api", Available: true}},
		{Core: coreB, Route: failover.ModelRoute{Model: "claude-haiku-4-5", Provider: "anthropic", APIMethod: "anthropic-api-key", Available: true}},
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
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})

	var lastErr error
	for ev := range events {
		if ev.Err != nil {
			lastErr = ev.Err
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
