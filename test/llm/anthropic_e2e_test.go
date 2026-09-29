package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
}

// TestEndToEndAnthropicProviderClassifierDispatch_ManualThinking
// pins the manual-thinking fallback for opus-4-5
// (capsManualWithEffort): thinking:{type:enabled,budget_tokens:4096}
// for effort=medium, plus output_config:{effort:medium}. Exercises
// the classifier-driven dispatch end-to-end through Core → provider
// → wire.
func TestEndToEndAnthropicProviderClassifierDispatch_ManualThinking(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-4-5",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
		MaxTokens:       64000,
		ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range events {
	}

	var body map[string]any
	json.Unmarshal(receivedBody, &body)
	thinking, ok := body["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking missing for opus-4-5: %+v", body)
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want enabled (caps.ManualThinking)", thinking["type"])
	}
	if budget := int(thinking["budget_tokens"].(float64)); budget != 4096 {
		t.Errorf("budget_tokens = %d, want 4096 (medium)", budget)
	}
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing for opus-4-5: %+v", body)
	}
	if outputConfig["effort"] != "medium" {
		t.Errorf("output_config.effort = %v, want medium", outputConfig["effort"])
	}
	if _, hasTemp := body["temperature"]; hasTemp {
		t.Errorf("temperature must be omitted when thinking is active, got %+v", body["temperature"])
	}
}

// TestEndToEndAnthropicProviderClassifierDispatch_XHighDefault
// pins that opus-4-7 (capsFull, XHighEffort=true) emits the xhigh
// effort by default even though the caller left ReasoningEffort
// empty. This is the highest-effort generation; we don't want to
// under-deliver silently.
func TestEndToEndAnthropicProviderClassifierDispatch_XHighDefault(t *testing.T) {
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "claude-opus-4-7",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range events {
	}

	var body map[string]any
	json.Unmarshal(receivedBody, &body)
	thinking, ok := body["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking missing for opus-4-7: %+v", body)
	}
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive (caps.AdaptiveThinking)", thinking["type"])
	}
	if _, hasBudget := thinking["budget_tokens"]; hasBudget {
		t.Errorf("adaptive envelope must omit budget_tokens, got %+v", thinking)
	}
	outputConfig, ok := body["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing for opus-4-7: %+v", body)
	}
	if outputConfig["effort"] != "xhigh" {
		t.Errorf("output_config.effort = %v, want xhigh (opus-4-7 default)", outputConfig["effort"])
	}
}

// TestEndToEndAnthropicProviderOneMSuffixStripAndBeta verifies
// that the `[1m]` suffix is stripped from the wire model field and
// the anthropic-beta header carries the context-1m opt-in. The
// header is added by core.headersFor from provider.BetaHeaders.
func TestEndToEndAnthropicProviderOneMSuffixStripAndBeta(t *testing.T) {
	var receivedBody []byte
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedHeader = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":5,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "claude-opus-4-6[1m]",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range events {
	}

	var body map[string]any
	json.Unmarshal(receivedBody, &body)
	if body["model"] != "claude-opus-4-6" {
		t.Errorf("wire model = %v, want claude-opus-4-6 (suffix stripped)", body["model"])
	}
	if receivedHeader != "context-1m-2025-08-07" {
		t.Errorf("anthropic-beta header = %q, want context-1m-2025-08-07", receivedHeader)
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

func TestEndToEndAnthropicProviderSelfHealRetryAlwaysOnModel(t *testing.T) {
	// Server returns 400 (output_config not supported) on the first
	// call; 200 on the second call. StreamChat should self-heal:
	// retry once with thinking kept and output_config dropped. The
	// second request body must NOT contain `output_config` and MUST
	// contain `thinking: {type: adaptive}`.
	var calls atomic.Int32
	var bodies [2]string
	var bodiesMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		bodiesMu.Lock()
		bodies[n-1] = string(body)
		bodiesMu.Unlock()
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"output_config.effort is not supported on this model"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}
`+"\n\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1,"output_tokens":1}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-5-5",
		ReasoningEffort: "medium",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var gotContent string
	var gotErr error
	for ev := range events {
		if ev.Err != nil {
			gotErr = ev.Err
			continue
		}
		if ev.Chunk != nil && len(ev.Chunk.Choices) > 0 {
			gotContent += ev.Chunk.Choices[0].Delta.Content
		}
	}
	if gotErr != nil {
		t.Fatalf("consumer saw error after retry: %v", gotErr)
	}
	if gotContent != "hi" {
		t.Errorf("content = %q, want %q (second call should succeed)", gotContent, "hi")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2 (1 fail + 1 self-heal success)", got)
	}

	bodiesMu.Lock()
	defer bodiesMu.Unlock()

	var firstBody, secondBody map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &firstBody); err != nil {
		t.Fatalf("unmarshal first body: %v\n%s", err, bodies[0])
	}
	if err := json.Unmarshal([]byte(bodies[1]), &secondBody); err != nil {
		t.Fatalf("unmarshal second body: %v\n%s", err, bodies[1])
	}
	if firstBody["output_config"] == nil {
		t.Errorf("first body missing output_config (expected it; self-heal drops it on retry only):\n%s", bodies[0])
	}
	if firstBody["thinking"] == nil {
		t.Errorf("first body missing thinking envelope:\n%s", bodies[0])
	}
	if secondBody["output_config"] != nil {
		t.Errorf("second body still has output_config, want omitted (self-heal must drop it):\n%s", bodies[1])
	}
	if secondBody["thinking"] == nil {
		t.Errorf("second body missing thinking envelope (always-on must keep thinking on retry):\n%s", bodies[1])
	}
}

func TestEndToEndAnthropicProviderSelfHealRetryNonAlwaysOnModel(t *testing.T) {
	// Server returns 400 (thinking is not supported) on the first
	// call; 200 on the second call. StreamChat must self-heal: retry
	// once with both `thinking` and `output_config` dropped.
	var calls atomic.Int32
	var bodies [2]string
	var bodiesMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		bodiesMu.Lock()
		bodies[n-1] = string(body)
		bodiesMu.Unlock()
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"thinking with extended budget is not supported"}}`)
			return
		}
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

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-4-7",
		ReasoningEffort: "high",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var gotContent string
	for ev := range events {
		if ev.Err != nil {
			t.Fatalf("consumer saw error: %v", ev.Err)
		}
		if ev.Chunk != nil && len(ev.Chunk.Choices) > 0 {
			gotContent += ev.Chunk.Choices[0].Delta.Content
		}
	}
	if gotContent != "ok" {
		t.Errorf("content = %q, want %q", gotContent, "ok")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}

	bodiesMu.Lock()
	defer bodiesMu.Unlock()

	var firstBody, secondBody map[string]any
	if err := json.Unmarshal([]byte(bodies[0]), &firstBody); err != nil {
		t.Fatalf("unmarshal first body: %v", err)
	}
	if err := json.Unmarshal([]byte(bodies[1]), &secondBody); err != nil {
		t.Fatalf("unmarshal second body: %v", err)
	}
	if firstBody["thinking"] == nil {
		t.Errorf("first body missing thinking envelope:\n%s", bodies[0])
	}
	if secondBody["thinking"] != nil {
		t.Errorf("second body still has thinking, want omitted (non-always-on must drop both on retry):\n%s", bodies[1])
	}
	if secondBody["output_config"] != nil {
		t.Errorf("second body still has output_config, want omitted:\n%s", bodies[1])
	}
}

func TestEndToEndAnthropicProviderSelfHealRetryDoesNotRetryOnUnrecoverableError(t *testing.T) {
	// Server returns 401 on every call. StreamChat must NOT retry
	// (auth errors are not recoverable) and must surface the error
	// synchronously so the caller sees it before getting the stream
	// channel.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-5-5",
		ReasoningEffort: "medium",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		// The error must reach the caller synchronously, not via the
		// channel (auth errors are not self-healable).
		for range events {
		}
		t.Fatalf("StreamChat: expected sync error, got nil")
	}
	if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (no retry on auth error)", got)
	}
}

func TestEndToEndAnthropicProviderSelfHealRetrySurfacesRetryError(t *testing.T) {
	// Both calls return 400 (a recoverable shape). The second
	// attempt fails too. StreamChat must surface the second
	// error synchronously and not loop forever.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported on this model"}}`)
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-4-7",
		ReasoningEffort: "high",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		for range events {
		}
		t.Fatalf("StreamChat: expected sync error from second attempt, got nil")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("err = %v, want 400 with 'not supported' message", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2 (first fail + retry fail, no infinite loop)", got)
	}
}

func TestEndToEndAnthropicProviderSelfHealRetryCancelledMidRetry(t *testing.T) {
	// First call returns 400 (recoverable). Second call hangs.
	// Caller cancels ctx while the retry is in flight. StreamChat
	// must not hang; the cancellation must terminate the retry
	// without producing channel events.
	var calls atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`)
			return
		}
		<-release
	}))
	defer srv.Close()
	defer close(release)

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		events, err := core.StreamChat(ctx, &llm.ChatRequest{
			Model:    "claude-opus-4-7",
			Messages: []llm.Message{{Role: "user", Content: "hi"}},
		})
		if err != nil {
			done <- err
			return
		}
		for ev := range events {
			if ev.Err != nil {
				done <- ev.Err
				return
			}
		}
		done <- nil
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("StreamChat did not return after ctx cancel (calls=%d)", calls.Load())
	}
}

func TestEndToEndAnthropicProviderSelfHealRetryPreservesBetaHeader(t *testing.T) {
	// opus-4-6[1m] requires the `anthropic-beta: context-1m-2025-08-07`
	// header. The retry must still send it; otherwise the second
	// attempt would be misclassified as a 200K-context model and
	// rejected for input length.
	var calls atomic.Int32
	var observed sync.Map // call# -> header value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		observed.Store(n-1, r.Header.Get("anthropic-beta"))
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"output_config.effort is not supported on this model"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-4-6[1m]",
		ReasoningEffort: "high",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range events {
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
	for n := int32(0); n < 2; n++ {
		v, ok := observed.Load(n)
		if !ok {
			t.Errorf("call %d header not captured", n)
			continue
		}
		if v != "context-1m-2025-08-07" {
			t.Errorf("call %d anthropic-beta = %q, want %q (1m beta must persist through self-heal retry)", n, v, "context-1m-2025-08-07")
		}
	}
}

func TestEndToEndAnthropicProviderSelfHealRetryPreservesTools(t *testing.T) {
	// Self-heal must NOT silently strip tools from the retry
	// request. The user's tool definitions must be present on
	// both attempts.
	var calls atomic.Int32
	var bodies [2]string
	var bodiesMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		bodiesMu.Lock()
		bodies[n-1] = string(body)
		bodiesMu.Unlock()
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}
`+"\n\n"+`data: {"type":"message_stop"}
`+"\n\n")
	}))
	defer srv.Close()

	client := &http.Client{Transport: redirectTransport{target: srv.URL}}
	core := llm.NewCore(providers.NewAnthropicProvider("tk"), protocol.NewHTTPRestWithClient(client))

	tool := llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "read_file",
			Description: "read a file from disk",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
		},
	}
	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:           "claude-opus-4-7",
		ReasoningEffort: "high",
		Tools:           []llm.ToolDef{tool},
		Messages:        []llm.Message{{Role: "user", Content: "read foo.txt"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	for range events {
	}

	bodiesMu.Lock()
	defer bodiesMu.Unlock()
	for n := 0; n < 2; n++ {
		if !strings.Contains(bodies[n], `"read_file"`) {
			t.Errorf("call %d missing tool definition:\n%s", n, bodies[n])
		}
		if !strings.Contains(bodies[n], `"tools"`) {
			t.Errorf("call %d missing tools key:\n%s", n, bodies[n])
		}
	}
}

// (Was added to test/llm/anthropic_e2e_test.go by mistake; see
// test/llm/providers/anthropic_test.go for the proper location.)
