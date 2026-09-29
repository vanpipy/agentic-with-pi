package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

// TestEndToEndAnthropicProviderBetaHeaderForOneMContext asserts the
// anthropic-beta header is sent when the requested model has
// BetaHeaders configured (claude-opus-4-6[1m]) and absent for
// standard-context models. Follows the Sprint 4 audit Option 2
// (BetaHeaders threading into the request path).
func TestEndToEndAnthropicProviderBetaHeaderForOneMContext(t *testing.T) {
	var observed sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Store(r.Header.Get("anthropic-beta"), true)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	for _, model := range []string{"claude-opus-4-6[1m]", "claude-opus-4-6"} {
		events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
			Model:    model,
			Messages: []llm.Message{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			t.Fatalf("StreamChat(%s): %v", model, err)
		}
		for range events {
		}
	}

	seenBeta, seenNoBeta := false, false
	observed.Range(func(k, _ any) bool {
		if k == "context-1m-2025-08-07" {
			seenBeta = true
		} else if k == "" {
			seenNoBeta = true
		}
		return true
	})

	if !seenBeta {
		t.Errorf("expected at least one request with anthropic-beta: context-1m-2025-08-07, got none")
	}
	if !seenNoBeta {
		t.Errorf("expected at least one request without anthropic-beta header, got none")
	}
}

// TestEndToEndAnthropicProviderDrivingRealHTTPRest verifies the full
// pipeline: AnthropicProvider -> protocol.HTTPRest -> httptest. The
// test asserts the request body shape (model, system as text blocks,
// thinking block on opus-4-6) and that the SSE response is surfaced
// as LegacyStreamEvents with the expected content and a usage chunk.
func TestEndToEndAnthropicProviderDrivingRealHTTPRest(t *testing.T) {
	var receivedBody []byte
	var receivedHeaders http.Header
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-opus-4-6","stop_reason":null,"usage":{"input_tokens":17,"output_tokens":1}}}
`+"\n\n"+`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}
`+"\n\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}
`+"\n\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}
`+"\n\n"+`data: {"type":"content_block_stop","index":0}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":17,"output_tokens":12}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("test-key"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model: "claude-opus-4-6",
		Messages: []llm.Message{
			{Role: "system", Content: "be helpful"},
			{Role: "user", Content: "hi"},
		},
		MaxTokens: 1024,
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}

	var gotContent string
	var gotUsage *llm.Usage
	var gotStopReason llm.FinishReason
	var gotFinishReasonCount int
	for ev := range events {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		if ev.Chunk == nil {
			continue
		}
		for _, ch := range ev.Chunk.Choices {
			if ch.Delta.Content != "" {
				gotContent += ch.Delta.Content
			}
			if ch.FinishReason != 0 {
				gotStopReason = ch.FinishReason
				gotFinishReasonCount++
			}
		}
		if ev.Chunk.Usage != nil {
			gotUsage = ev.Chunk.Usage
		}
	}

	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
	if receivedHeaders.Get("x-api-key") != "test-key" {
		t.Errorf("x-api-key = %q", receivedHeaders.Get("x-api-key"))
	}
	if receivedHeaders.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("anthropic-version = %q", receivedHeaders.Get("anthropic-version"))
	}

	var body map[string]any
	if err := json.Unmarshal(receivedBody, &body); err != nil {
		t.Fatalf("body parse: %v\n%s", err, receivedBody)
	}
	if body["model"] != "claude-opus-4-6" {
		t.Errorf("body.model = %v", body["model"])
	}
	if body["max_tokens"].(float64) != 1024 {
		t.Errorf("body.max_tokens = %v", body["max_tokens"])
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("body.thinking missing: %+v", body)
	}
	if thinking["type"] != "adaptive" {
		t.Errorf("body.thinking.type = %v, want adaptive", thinking["type"])
	}
	if _, hasBudget := thinking["budget_tokens"]; hasBudget {
		t.Errorf("adaptive thinking must omit budget_tokens, got %+v", thinking)
	}
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("body.output_config missing: %+v", body)
	}
	if outputConfig["effort"] != "high" {
		t.Errorf("body.output_config.effort = %v, want high (opus-4-6 default)", outputConfig["effort"])
	}
	systemBlocks := body["system"].([]any)
	if len(systemBlocks) != 1 {
		t.Fatalf("body.system blocks = %v", body["system"])
	}
	block := systemBlocks[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "be helpful" {
		t.Errorf("system block = %v", block)
	}

	if gotContent != "hello world" {
		t.Errorf("content = %q, want %q", gotContent, "hello world")
	}
	if gotUsage == nil {
		t.Errorf("usage missing")
	} else {
		if gotUsage.PromptTokens != 17 {
			t.Errorf("usage.prompt_tokens = %d", gotUsage.PromptTokens)
		}
		if gotUsage.CompletionTokens != 12 {
			t.Errorf("usage.completion_tokens = %d", gotUsage.CompletionTokens)
		}
	}
	if gotStopReason != llm.FinishReasonStop {
		t.Errorf("stop_reason = %v, want Stop", gotStopReason)
	}
	if gotFinishReasonCount != 1 {
		t.Errorf("got %d finish reasons, want exactly 1", gotFinishReasonCount)
	}
}

// TestEndToEndAnthropicProviderHaikuOmitsThinking is the negative
// counterpart: haiku-4-5 has supportsThinking=false, so the request
// body must not contain a "thinking" field.
func TestEndToEndAnthropicProviderHaikuOmitsThinking(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("hk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range events {
	}

	var body map[string]any
	json.Unmarshal(receivedBody, &body)
	if _, ok := body["thinking"]; ok {
		t.Errorf("thinking should be omitted for haiku-4-5: %+v", body)
	}
	if body["model"] != "claude-haiku-4-5" {
		t.Errorf("model = %v", body["model"])
	}
}

// TestEndToEndAnthropicProviderToolUseRoundTrip exercises the
// content_block_start + content_block_delta + content_block_stop
// tool_use sequence end-to-end, verifying the assembled tool call
// appears in the final chunk.
func TestEndToEndAnthropicProviderToolUseRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_test","name":"bash","input":{}}}
`+"\n\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":\"ls\"}"}}
`+"\n\n"+`data: {"type":"content_block_stop","index":0}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":3,"output_tokens":5}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, _ := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{{Role: "user", Content: "run"}},
		Tools: []llm.ToolDef{{
			Type:     "function",
			Function: llm.FunctionDef{Name: "bash"},
		}},
	})

	var toolCalls []llm.ToolCall
	var stopReason llm.FinishReason
	for ev := range events {
		if ev.Err != nil {
			t.Fatalf("stream err: %v", ev.Err)
		}
		if ev.Chunk == nil {
			continue
		}
		for _, ch := range ev.Chunk.Choices {
			toolCalls = append(toolCalls, ch.Delta.ToolCalls...)
			if ch.FinishReason != 0 {
				stopReason = ch.FinishReason
			}
		}
	}
	if len(toolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1: %+v", len(toolCalls), toolCalls)
	}
	if toolCalls[0].ID != "toolu_test" {
		t.Errorf("tool call ID = %q", toolCalls[0].ID)
	}
	if toolCalls[0].Function.Name != "bash" {
		t.Errorf("tool name = %q", toolCalls[0].Function.Name)
	}
	if toolCalls[0].Function.Arguments != `{"cmd":"ls"}` {
		t.Errorf("tool args = %q", toolCalls[0].Function.Arguments)
	}
	if stopReason != llm.FinishReasonToolUse {
		t.Errorf("stop reason = %v, want ToolUse", stopReason)
	}
}
