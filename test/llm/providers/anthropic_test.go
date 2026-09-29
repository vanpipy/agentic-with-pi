package providers_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

func newAnthropicProvider() *providers.AnthropicProvider {
	return providers.NewAnthropicProvider("test-key")
}

func TestAnthropicProviderName(t *testing.T) {
	p := newAnthropicProvider()
	if p.Name() != "anthropic" {
		t.Errorf("Name = %q, want anthropic", p.Name())
	}
}

func TestAnthropicProviderBaseURL(t *testing.T) {
	p := newAnthropicProvider()
	if got := p.BaseURL(); got != "https://api.anthropic.com" {
		t.Errorf("BaseURL = %q", got)
	}
}

func TestAnthropicProviderPath(t *testing.T) {
	p := newAnthropicProvider()
	if got := p.Path(); got != "/v1/messages" {
		t.Errorf("Path = %q", got)
	}
}

func TestAnthropicProviderHeaders(t *testing.T) {
	p := newAnthropicProvider()
	h := p.Headers()
	if h["x-api-key"] != "test-key" {
		t.Errorf("x-api-key = %q", h["x-api-key"])
	}
	if h["anthropic-version"] != "2023-06-01" {
		t.Errorf("anthropic-version = %q", h["anthropic-version"])
	}
	if h["content-type"] != "application/json" {
		t.Errorf("content-type = %q", h["content-type"])
	}
}

func TestAnthropicProviderModels(t *testing.T) {
	p := newAnthropicProvider()
	models := p.Models()
	wantIDs := map[string]bool{
		"claude-opus-4-6":     false,
		"claude-opus-4-6[1m]": false,
		"claude-sonnet-4-6":   false,
		"claude-haiku-4-5":    false,
	}
	for _, m := range models {
		if _, ok := wantIDs[m.ID]; ok {
			wantIDs[m.ID] = true
		}
		if m.Vendor != "anthropic" {
			t.Errorf("model %s vendor = %q", m.ID, m.Vendor)
		}
		if !m.SupportsStreaming {
			t.Errorf("model %s should support streaming", m.ID)
		}
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Errorf("missing model %s", id)
		}
	}
}

func TestAnthropicProviderContextWindow(t *testing.T) {
	p := newAnthropicProvider()
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-6", 200000},
		{"claude-opus-4-6[1m]", 1000000},
		{"claude-sonnet-4-6", 200000},
		{"claude-haiku-4-5", 200000},
		{"unknown-model", 200000},
	}
	for _, tc := range cases {
		if got := p.ContextWindow(tc.model); got != tc.want {
			t.Errorf("ContextWindow(%s) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestAnthropicProviderMaxOutputTokens(t *testing.T) {
	p := newAnthropicProvider()
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-6", 32000},
		{"claude-opus-4-6[1m]", 32000},
		{"claude-sonnet-4-6", 16000},
		{"claude-haiku-4-5", 8192},
		{"unknown-model", 8192},
	}
	for _, tc := range cases {
		if got := p.MaxOutputTokens(tc.model); got != tc.want {
			t.Errorf("MaxOutputTokens(%s) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestAnthropicProviderBetaHeaders(t *testing.T) {
	p := newAnthropicProvider()
	if got := p.BetaHeaders("claude-opus-4-6"); len(got) != 0 {
		t.Errorf("opus-4-6 beta headers = %v, want empty", got)
	}
	if got := p.BetaHeaders("claude-opus-4-6[1m]"); len(got) != 1 || got[0] != "context-1m-2025-08-07" {
		t.Errorf("opus-4-6[1m] beta headers = %v, want context-1m-2025-08-07", got)
	}
	if got := p.BetaHeaders("claude-haiku-4-5"); len(got) != 0 {
		t.Errorf("haiku-4-5 beta headers = %v, want empty", got)
	}
}

func TestAnthropicProviderModelCapabilities(t *testing.T) {
	p := newAnthropicProvider()
	caps := p.ModelCapabilities("claude-opus-4-6[1m]")
	if caps.ContextWindow != 1000000 {
		t.Errorf("context window = %d", caps.ContextWindow)
	}
	if caps.ContextMode != llm.ContextOptIn1M {
		t.Errorf("context mode = %v, want ContextOptIn1M", caps.ContextMode)
	}
	if !caps.SupportsThinking {
		t.Errorf("should support thinking")
	}
	if len(caps.BetaHeaders) != 1 || caps.BetaHeaders[0] != "context-1m-2025-08-07" {
		t.Errorf("beta headers = %v", caps.BetaHeaders)
	}
}

func TestAnthropicProviderModelCapabilitiesUnknown(t *testing.T) {
	p := newAnthropicProvider()
	caps := p.ModelCapabilities("unknown")
	if caps.ContextWindow != 200000 {
		t.Errorf("default context window = %d", caps.ContextWindow)
	}
	if caps.ContextMode != llm.ContextStandard {
		t.Errorf("default context mode = %v", caps.ContextMode)
	}
}

func TestAnthropicProviderConvertRequestBasicChat(t *testing.T) {
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "claude-haiku-4-5",
		Messages: []llm.Message{
			{Role: "user", Content: "hello"},
		},
		MaxTokens:   1024,
		Temperature: 0.7,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "claude-haiku-4-5" {
		t.Errorf("model = %v", got["model"])
	}
	if got["max_tokens"].(float64) != 1024 {
		t.Errorf("max_tokens = %v", got["max_tokens"])
	}
	if got["temperature"].(float64) < 0.699 || got["temperature"].(float64) > 0.701 {
		t.Errorf("temperature = %v, want ~0.7", got["temperature"])
	}
}

func TestAnthropicProviderConvertRequestDefaultsMaxTokens(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	if got["max_tokens"].(float64) != 4096 {
		t.Errorf("default max_tokens = %v, want 4096", got["max_tokens"])
	}
}

func TestAnthropicProviderConvertRequestThinkingEnabledForReasoningModels(t *testing.T) {
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:     "claude-opus-4-6",
		Messages:  []llm.Message{{Role: "user", Content: "hi"}},
		MaxTokens: 16000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking, ok := got["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking missing for reasoning model: %+v", got)
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %v", thinking["type"])
	}
	budget := thinking["budget_tokens"].(float64)
	if budget != 8000 {
		t.Errorf("thinking.budget_tokens = %v, want 8000 (maxTokens/2)", budget)
	}
}

func TestAnthropicProviderConvertRequestThinkingFloor(t *testing.T) {
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:     "claude-opus-4-6",
		Messages:  []llm.Message{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking := got["thinking"].(map[string]any)
	if thinking["budget_tokens"].(float64) != 1024 {
		t.Errorf("budget_tokens = %v, want 1024 (floor)", thinking["budget_tokens"])
	}
}

func TestAnthropicProviderConvertRequestThinkingOmittedForNonReasoning(t *testing.T) {
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	if _, ok := got["thinking"]; ok {
		t.Errorf("thinking should be omitted for haiku: %+v", got)
	}
}

func TestAnthropicProviderConvertRequestExtractsSystemPrompt(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model: "claude-haiku-4-5",
		Messages: []llm.Message{
			{Role: "system", Content: "you are a pirate"},
			{Role: "user", Content: "hello"},
		},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	systemBlocks := got["system"].([]any)
	if len(systemBlocks) != 1 {
		t.Fatalf("system blocks = %v", got["system"])
	}
	block := systemBlocks[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "you are a pirate" {
		t.Errorf("system block = %v", block)
	}
}

func TestAnthropicProviderConvertRequestToolResultAsUserMessage(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model: "claude-haiku-4-5",
		Messages: []llm.Message{
			{Role: "user", Content: "weather?"},
			{Role: "tool", ToolCallID: "toolu_1", Content: "sunny, 22C"},
		},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	msgs := got["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	toolMsg := msgs[1].(map[string]any)
	if toolMsg["role"] != "user" {
		t.Errorf("tool msg role = %v, want user", toolMsg["role"])
	}
	content := toolMsg["content"].([]any)
	if content[0].(map[string]any)["type"] != "tool_result" {
		t.Errorf("first block type = %v", content[0])
	}
}

func TestAnthropicProviderConvertRequestIgnoresUnknownRoles(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model: "claude-haiku-4-5",
		Messages: []llm.Message{
			{Role: "function", Content: "should be dropped"},
			{Role: "user", Content: "hi"},
		},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	msgs := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 msg, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "user" {
		t.Errorf("role = %v", msgs[0])
	}
	content := msgs[0].(map[string]any)["content"].([]any)
	if len(content) != 1 {
		t.Errorf("user content = %v, want single text block (no leak from dropped role)", content)
	}
}

func TestAnthropicProviderConvertRequestNoMessagesDefaultUser(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:    "claude-haiku-4-5",
		Messages: []llm.Message{},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	msgs := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 default message, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "user" {
		t.Errorf("default role = %v", msgs[0])
	}
}

func TestAnthropicProviderConvertRequestToolChoice(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:      "claude-haiku-4-5",
		Messages:   []llm.Message{{Role: "user", Content: "hi"}},
		Tools:      []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "f"}}},
		ToolChoice: &llm.ToolChoice{Mode: "tool", Name: "f"},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	tc := got["tool_choice"].(map[string]any)
	if tc["type"] != "tool" || tc["name"] != "f" {
		t.Errorf("tool_choice = %v", tc)
	}
}

func TestAnthropicProviderConvertRequestStopSequences(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:         "claude-haiku-4-5",
		Messages:      []llm.Message{{Role: "user", Content: "hi"}},
		StopSequences: []string{"END", "STOP"},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	ss := got["stop_sequences"].([]any)
	if len(ss) != 2 || ss[0] != "END" || ss[1] != "STOP" {
		t.Errorf("stop_sequences = %v", ss)
	}
}

func TestAnthropicProviderConvertRequestUnknownModelNoThinking(t *testing.T) {
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:    "no-such-model",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	if _, ok := got["thinking"]; ok {
		t.Errorf("thinking should be omitted for unknown model: %+v", got)
	}
}

func TestAnthropicProviderConvertResponseDelegates(t *testing.T) {
	p := newAnthropicProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("done should be false")
	}
	if chunk == nil {
		t.Fatal("nil chunk")
	}
	if len(chunk.Choices) != 1 || chunk.Choices[0].Delta.Content != "hello" {
		t.Errorf("chunk = %+v", chunk)
	}
}

func TestAnthropicProviderConvertResponseMessageStopDelegates(t *testing.T) {
	p := newAnthropicProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"message_stop"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("done should be true for message_stop")
	}
	if chunk != nil {
		t.Errorf("chunk should be nil for message_stop, got %+v", chunk)
	}
}

func TestAnthropicProviderCompleteSplitDelegates(t *testing.T) {
	p := newAnthropicProvider()
	blocks, err := p.CompleteSplit("part1\npart2")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
}

func TestAnthropicProviderSupportsCacheControl(t *testing.T) {
	p := newAnthropicProvider()
	if !p.SupportsCacheControl("claude-opus-4-6") {
		t.Error("opus-4-6 should support cache")
	}
	if !p.SupportsCacheControl("claude-haiku-4-5") {
		t.Error("haiku-4-5 should support cache")
	}
	if p.SupportsCacheControl("unknown-model") {
		t.Error("unknown model should not support cache")
	}
}

func TestAnthropicProviderSupportsNativeCompact(t *testing.T) {
	p := newAnthropicProvider()
	if p.SupportsNativeCompact("claude-opus-4-6") {
		t.Error("native compact should be false (no modelSpec supports it)")
	}
}

func TestAnthropicProviderAvailableReasoningEfforts(t *testing.T) {
	p := newAnthropicProvider()
	if got := p.AvailableReasoningEfforts("claude-opus-4-6"); got != nil {
		t.Errorf("reasoning efforts = %v, want nil", got)
	}
}

func TestAnthropicProviderAvailableServiceTiers(t *testing.T) {
	p := newAnthropicProvider()
	if got := p.AvailableServiceTiers("claude-opus-4-6"); got != nil {
		t.Errorf("service tiers = %v, want nil", got)
	}
}
