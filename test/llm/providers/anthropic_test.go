package providers_test

import (
	"context"
	"encoding/json"
	"errors"
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
		{"claude-opus-4-6", 128000},
		{"claude-opus-4-6[1m]", 128000},
		{"claude-sonnet-4-6", 64000},
		{"claude-haiku-4-5", 64000},
		// Unknown Claude generations fall back to the classifier
		// (anthropic.MaxOutputTokens), which returns the legacy
		// 32K default for generations the LARGE_OUTPUT_PREFIXES
		// table does not match.
		{"claude-future-unknown", 32768},
		{"claude-opus-7", 32768},
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
	// haiku-4-5 max output = 64K per the published limits table
	// (anthropic.rs:198-200 in jcode).
	if got["max_tokens"].(float64) != 64000 {
		t.Errorf("default max_tokens = %v, want 64000 (haiku-4-5)", got["max_tokens"])
	}
}

func TestAnthropicProviderConvertRequestThinkingAdaptiveForReasoningModels(t *testing.T) {
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
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive", thinking["type"])
	}
	if _, hasBudget := thinking["budget_tokens"]; hasBudget {
		t.Errorf("adaptive thinking must omit budget_tokens, got %+v", thinking)
	}
	outputConfig, ok := got["output_config"].(map[string]any)
	if !ok {
		t.Fatalf("output_config missing for reasoning model: %+v", got)
	}
	// opus-4-6 caps include XHighEffort, so the default is "xhigh".
	if outputConfig["effort"] != "high" {
		t.Errorf("output_config.effort = %v, want high (opus-4-6 default; XHighEffort=false)", outputConfig["effort"])
	}
	if _, hasTemperature := got["temperature"]; hasTemperature {
		t.Errorf("temperature must be omitted when thinking is active, got %+v", got["temperature"])
	}
}

func TestAnthropicProviderConvertRequestThinkingAdaptiveNoBudget(t *testing.T) {
	// Adaptive thinking does not carry budget_tokens — the model
	// decides its own budget. This test pins that the wire shape
	// omits the field even when the caller passes MaxTokens=1
	// (which would have hit the legacy floor logic).
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
	thinking, ok := got["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking missing: %+v", got)
	}
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive", thinking["type"])
	}
	if _, hasBudget := thinking["budget_tokens"]; hasBudget {
		t.Errorf("adaptive thinking must omit budget_tokens, got %+v", thinking)
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

func TestAnthropicProviderConvertRequestHonorsReasoningEffortOverride(t *testing.T) {
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:           "claude-opus-4-6",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: "low",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking := got["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive", thinking["type"])
	}
	outputConfig := got["output_config"].(map[string]any)
	if outputConfig["effort"] != "low" {
		t.Errorf("output_config.effort = %v, want low", outputConfig["effort"])
	}
}

func TestAnthropicProviderConvertRequestNoneEffortOmitsOutputConfig(t *testing.T) {
	// "none" must still emit adaptive thinking but drop
	// output_config entirely (no point sending an effort block that
	// asks for no effort).
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:           "claude-opus-4-6",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking := got["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" {
		t.Errorf("thinking.type = %v, want adaptive", thinking["type"])
	}
	if _, ok := got["output_config"]; ok {
		t.Errorf("output_config must be omitted when effort=none, got %+v", got["output_config"])
	}
}

func TestAnthropicProviderConvertRequestStrips1mSuffix(t *testing.T) {
	p := newAnthropicProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:    "claude-opus-4-6[1m]",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	if got["model"] != "claude-opus-4-6" {
		t.Errorf("model = %v, want claude-opus-4-6 (suffix stripped)", got["model"])
	}
}

func TestAnthropicProviderDefaultReasoningEffort(t *testing.T) {
	p := newAnthropicProvider()
	cases := []struct {
		model string
		want  string
	}{
		{"claude-opus-4-6", "high"},   // no xhigh (caps=NoXhigh), default high
		{"claude-opus-4-7", "xhigh"},  // caps=Full, default xhigh
		{"claude-opus-5", "low"},      // opus-5 ladder starts low
		{"claude-opus-5-5", "medium"}, // opus-5-5 sweet spot
		{"claude-fable-5", "high"},
		{"claude-fable-5-1", "high"},
		{"claude-haiku-4-5", ""},  // haiku is not an opus/fable
		{"claude-sonnet-4-6", ""}, // sonnet is not an opus/fable
	}
	for _, tc := range cases {
		if got := p.DefaultReasoningEffort(tc.model); got != tc.want {
			t.Errorf("DefaultReasoningEffort(%s) = %q, want %q", tc.model, got, tc.want)
		}
	}
}

func TestAnthropicProviderConvertRequestManualThinkingFallback(t *testing.T) {
	// opus-4-5 has capsManualWithEffort (ManualThinking=true,
	// OutputEffort=true, no AdaptiveThinking). It must emit the
	// legacy `thinking: {type: enabled, budget_tokens: N}`
	// envelope with a budget that scales to the requested effort.
	// exercise the helper at every effort tier.
	p := newAnthropicProvider()
	cases := []struct {
		effort        string
		wantBudgetMin int
		wantBudgetMax int
	}{
		{"minimal", 1024, 1024},
		{"low", 1024, 1024},
		{"medium", 4096, 4096},
		{"high", 8192, 8192},
		{"xhigh", 16384, 16384},
	}
	for _, tc := range cases {
		body, err := p.ConvertRequest(&llm.ChatRequest{
			Model:           "claude-opus-4-5",
			Messages:        []llm.Message{{Role: "user", Content: "hi"}},
			MaxTokens:       64000,
			ReasoningEffort: tc.effort,
		})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		json.Unmarshal(body, &got)
		thinking := got["thinking"].(map[string]any)
		if thinking["type"] != "enabled" {
			t.Errorf("opus-4-5 effort=%s: thinking.type = %v, want enabled", tc.effort, thinking["type"])
		}
		budget := int(thinking["budget_tokens"].(float64))
		if budget < tc.wantBudgetMin || budget > tc.wantBudgetMax {
			t.Errorf("opus-4-5 effort=%s: budget = %d, want [%d, %d]", tc.effort, budget, tc.wantBudgetMin, tc.wantBudgetMax)
		}
	}
}

func TestAnthropicProviderConvertRequestManualThinkingBudgetClamped(t *testing.T) {
	// The budget is clamped to maxTokens-1 and the manual envelope
	// is omitted when the resulting budget would drop below the
	// 1024-token floor. With maxTokens=2000 and effort=high
	// (desired 8192), the budget is 1999 — still above the floor,
	// so the thinking block is emitted.
	p := newAnthropicProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:           "claude-opus-4-5",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
		MaxTokens:       2000,
		ReasoningEffort: "high",
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking, ok := got["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking missing: %+v", got)
	}
	budget := int(thinking["budget_tokens"].(float64))
	if budget != 1999 {
		t.Errorf("budget = %d, want 1999 (maxTokens-1 clamp)", budget)
	}

	// Now push max_tokens below the floor — thinking must be
	// omitted entirely.
	body, _ = p.ConvertRequest(&llm.ChatRequest{
		Model:           "claude-opus-4-5",
		Messages:        []llm.Message{{Role: "user", Content: "hi"}},
		MaxTokens:       1024,
		ReasoningEffort: "high",
	})
	got = nil // json.Unmarshal merges into the existing map; reset
	// to drop stale keys from the previous body.
	json.Unmarshal(body, &got)
	if _, ok := got["thinking"]; ok {
		t.Errorf("thinking must be omitted when budget would be < 1024, got %+v", got["thinking"])
	}
}

func TestAnthropicProviderConvertRequestContextWindowViaClassifier(t *testing.T) {
	p := newAnthropicProvider()
	// Spec table is the source of truth for known models (1M for
	// opus-4-6[1m]). Unknown generations fall back to 200K.
	cases := []struct {
		model string
		want  int
	}{
		{"claude-opus-4-6", 200000},
		{"claude-opus-4-6[1m]", 1000000},
		{"claude-future-9-9", 200000}, // unknown, default
	}
	for _, tc := range cases {
		if got := p.ContextWindow(tc.model); got != tc.want {
			t.Errorf("ContextWindow(%s) = %d, want %d", tc.model, got, tc.want)
		}
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
	if p.NativeCompactMode("claude-opus-4-6") != "" {
		t.Errorf("NativeCompactMode = %q, want empty (KindClient — no server endpoint)", p.NativeCompactMode("claude-opus-4-6"))
	}
	caps := p.NativeCompactCapabilities("claude-opus-4-6")
	if caps.Kind != llm.KindClient {
		t.Errorf("NativeCompactCapabilities.Kind = %d, want %d (KindClient)", caps.Kind, llm.KindClient)
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

func TestAnthropicProviderNativeCompactCapabilities(t *testing.T) {
	p := newAnthropicProvider()
	caps := p.NativeCompactCapabilities("claude-opus-4-6")
	if caps.Kind == llm.KindNative {
		t.Errorf("NativeCompactCapabilities.Kind = KindNative, want non-native (no server endpoint exposed)")
	}
}

func TestAnthropicProviderNativeCompactReturnsUnsupportedError(t *testing.T) {
	p := newAnthropicProvider()
	_, err := p.NativeCompact(context.Background(), "claude-opus-4-6", nil, "", "")
	if !errors.Is(err, llm.ErrNativeCompactionUnsupported) {
		t.Fatalf("err = %v, want ErrNativeCompactionUnsupported", err)
	}
}

func TestAnthropicProviderAvailableReasoningEfforts(t *testing.T) {
	p := newAnthropicProvider()
	// opus-4-6 caps are EFFORT_NO_XHIGH: output_config effort
	// ladder excludes "xhigh" but accepts the others. The provider
	// delegates to anthropic.AvailableReasoningEfforts so this is
	// also a smoke test for the classifier integration.
	got := p.AvailableReasoningEfforts("claude-opus-4-6")
	if len(got) == 0 {
		t.Fatalf("reasoning efforts = empty, want non-empty for opus-4-6")
	}
	for _, e := range got {
		if e == "xhigh" {
			t.Errorf("opus-4-6 must not advertise xhigh (caps.XHighEffort=false)")
		}
	}
}

func TestAnthropicProviderAvailableServiceTiers(t *testing.T) {
	p := newAnthropicProvider()
	if got := p.AvailableServiceTiers("claude-opus-4-6"); got != nil {
		t.Errorf("service tiers = %v, want nil", got)
	}
}

func TestAnthropicProviderRecoverRequest_AlwaysOnModel(t *testing.T) {
	p := newAnthropicProvider()
	for _, model := range []string{"claude-opus-5-5", "claude-fable-5-1", "Claude-Opus-5-5", "claude-opus-5.5"} {
		t.Run(model, func(t *testing.T) {
			req := &llm.ChatRequest{Model: model, ReasoningEffort: "medium"}
			err := &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"output_config is not supported on this model"}}`}
			if !p.RecoverRequest(req, err) {
				t.Fatal("RecoverRequest = false, want true")
			}
			if !req.RetryDropOutputConfig {
				t.Errorf("RetryDropOutputConfig = false, want true (always-on model must keep thinking envelope)")
			}
			if req.RetryDropThinking {
				t.Errorf("RetryDropThinking = true, want false (always-on model must keep thinking envelope)")
			}
		})
	}
}

func TestAnthropicProviderRecoverRequest_NonAlwaysOnModel(t *testing.T) {
	p := newAnthropicProvider()
	for _, model := range []string{"claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5"} {
		t.Run(model, func(t *testing.T) {
			req := &llm.ChatRequest{Model: model, ReasoningEffort: "high"}
			err := &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"thinking with extended budget is not supported"}}`}
			if !p.RecoverRequest(req, err) {
				t.Fatal("RecoverRequest = false, want true")
			}
			if !req.RetryDropThinking {
				t.Errorf("RetryDropThinking = false, want true (non-always-on must drop both)")
			}
			if req.RetryDropOutputConfig {
				t.Errorf("RetryDropOutputConfig = true, want false (non-always-on should drop both, not keep thinking)")
			}
		})
	}
}

func TestAnthropicProviderRecoverRequest_NonReasoningError(t *testing.T) {
	p := newAnthropicProvider()
	cases := []struct {
		name string
		err  error
	}{
		{"auth_error", &providerError{msg: "401 Unauthorized: invalid api key"}},
		{"rate_limit", &providerError{msg: "429 Too Many Requests"}},
		{"server_500", &providerError{msg: "500 Internal Server Error"}},
		{"schema_mismatch", &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"messages.0.content.0.type: must be 'text'"}}`}},
		{"thinking_unsupported_but_not_a_400", &providerError{msg: "thinking field is not supported (network glitch)"}},
		{"nil_err", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &llm.ChatRequest{Model: "claude-opus-5-5"}
			if p.RecoverRequest(req, tc.err) {
				t.Errorf("RecoverRequest = true, want false (non-reasoning error must not trigger recovery)")
			}
			if req.RetryDropThinking || req.RetryDropOutputConfig {
				t.Errorf("req mutated: RetryDropThinking=%v RetryDropOutputConfig=%v", req.RetryDropThinking, req.RetryDropOutputConfig)
			}
		})
	}
}

func TestAnthropicProviderConvertRequestHonorsRetryDropFlags(t *testing.T) {
	p := newAnthropicProvider()

	// RetryDropOutputConfig on an always-on model: keeps thinking,
	// drops output_config.
	req := &llm.ChatRequest{
		Model:                 "claude-opus-5-5",
		ReasoningEffort:       "medium",
		RetryDropOutputConfig: true,
		Messages:              []llm.Message{{Role: "user", Content: "hi"}},
	}
	body, err := p.ConvertRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["thinking"] == nil {
		t.Errorf("thinking missing, want adaptive envelope kept: body=%s", string(body))
	}
	if got["output_config"] != nil {
		t.Errorf("output_config present, want omitted: body=%s", string(body))
	}

	// RetryDropThinking on a non-always-on model: drops both.
	req = &llm.ChatRequest{
		Model:             "claude-opus-4-7",
		ReasoningEffort:   "high",
		RetryDropThinking: true,
		Messages:          []llm.Message{{Role: "user", Content: "hi"}},
	}
	body, err = p.ConvertRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["thinking"] != nil {
		t.Errorf("thinking present, want omitted: body=%s", string(body))
	}
	if got["output_config"] != nil {
		t.Errorf("output_config present, want omitted: body=%s", string(body))
	}
}

// providerError is a minimal error type that prints `msg` verbatim
// (so the substring heuristics in isReasoningUnsupportedError can be
// tested deterministically).
type providerError struct {
	msg string
}

func (e *providerError) Error() string { return e.msg }

func TestAnthropicProviderRecoverRequest_EmptyAndNonClaudeModels(t *testing.T) {
	p := newAnthropicProvider()
	cases := []struct {
		name  string
		model string
		err   error
	}{
		{"empty_model", "", &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`}},
		{"unknown_model", "future-claude-9-9", &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`}},
		{"non_claude", "gpt-5", &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`}},
		{"anthropic_version", "claude-9-9-future", &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"output_config is not supported"}}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &llm.ChatRequest{Model: tc.model}
			if !p.RecoverRequest(req, tc.err) {
				t.Fatalf("RecoverRequest(%q) = false, want true (reasoning-not-supported error should always trigger recovery)", tc.model)
			}
			if req.RetryDropOutputConfig {
				t.Errorf("RetryDropOutputConfig = true, want false (non-always-on must drop both)")
			}
			if !req.RetryDropThinking {
				t.Errorf("RetryDropThinking = false, want true")
			}
		})
	}
}

func TestAnthropicProviderConvertRequest_RetryDropOutputConfigOnManualOnlyModel(t *testing.T) {
	// Opus-4-5 uses manual thinking (caps.ManualThinking=true,
	// caps.AdaptiveThinking=false). When self-heal sets
	// RetryDropOutputConfig on a manual-only model, the switch
	// branch must not silently inject an adaptive envelope. The
	// original error targeted output_config; the model never used
	// it, so the safest retry shape is: no thinking, no
	// output_config.
	p := newAnthropicProvider()
	req := &llm.ChatRequest{
		Model:                 "claude-opus-4-5",
		ReasoningEffort:       "high",
		RetryDropOutputConfig: true,
		Messages:              []llm.Message{{Role: "user", Content: "hi"}},
	}
	body, err := p.ConvertRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["output_config"] != nil {
		t.Errorf("output_config present, want omitted: %s", string(body))
	}
	if got["thinking"] != nil {
		t.Errorf("thinking present, want omitted (manual-only model has no adaptive envelope to keep): %s", string(body))
	}
}

func TestAnthropicProviderConvertRequest_RetryDropThinkingOnUnknownModel(t *testing.T) {
	// Future Claude model the classifier has never seen. Self-heal
	// set RetryDropThinking. The retry must produce a clean body
	// with no thinking/output_config and no classifier-dependent
	// logic.
	p := newAnthropicProvider()
	req := &llm.ChatRequest{
		Model:             "claude-future-9-9",
		ReasoningEffort:   "medium",
		RetryDropThinking: true,
		Messages:          []llm.Message{{Role: "user", Content: "hi"}},
	}
	body, err := p.ConvertRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["thinking"] != nil {
		t.Errorf("thinking present, want omitted: %s", string(body))
	}
	if got["output_config"] != nil {
		t.Errorf("output_config present, want omitted: %s", string(body))
	}
}

func TestMiniMaxProviderRecoverRequestAlwaysReturnsFalse(t *testing.T) {
	// MiniMax uses the legacy manual envelope with no output_config
	// block. There is no recovery surface — every error must
	// surface verbatim.
	p := providers.NewMiniMaxProvider("tk")
	cases := []error{
		nil,
		errors.New("network"),
		&providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`},
		&providerError{msg: "401 Unauthorized"},
	}
	for _, err := range cases {
		req := &llm.ChatRequest{Model: "claude-opus-5-5"}
		if p.RecoverRequest(req, err) {
			t.Errorf("MiniMax.RecoverRequest(%v) = true, want false", err)
		}
		if req.RetryDropThinking || req.RetryDropOutputConfig {
			t.Errorf("MiniMax.RecoverRequest mutated req for err %v", err)
		}
	}
}

func TestAnthropicProviderRecoverRequest_DoesNotMutateModel(t *testing.T) {
	// The retry path runs ConvertRequest on the mutated request,
	// which calls Strip1mSuffix on ar.Model but must NOT mutate
	// req.Model (the caller may still reference it after the
	// request returns).
	p := newAnthropicProvider()
	req := &llm.ChatRequest{
		Model:           "claude-opus-4-6[1m]",
		ReasoningEffort: "high",
	}
	originalModel := req.Model
	_ = p.RecoverRequest(req, &providerError{msg: `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`})
	if req.Model != originalModel {
		t.Errorf("req.Model mutated: %q -> %q (RecoverRequest must not strip suffix from caller-visible field)", originalModel, req.Model)
	}
	if !req.RetryDropThinking {
		t.Errorf("RetryDropThinking = false, want true")
	}
}

func TestRecoverRequest_NegativeErrorMessages(t *testing.T) {
	// The heuristic must NOT classify the following as
	// recoverable: they are not about output_config/thinking/
	// effort rejection.
	p := newAnthropicProvider()
	nonRecoverable := []string{
		"thinking must be enabled for this model",
		"thinking is required",
		"thinking is disabled for this model",
		"output_config value 'high' is invalid",
		"output_config has wrong type",
		"missing required field: model",
		"invalid api key",
		"context length exceeded",
		"prompt is too long",
		"max_tokens too large for this model",
		// Mixed but not about thinking/effort/output_config rejection:
		"messages.0.content.0.type: must be 'text'",
		"tool_choice is invalid",
	}
	for _, msg := range nonRecoverable {
		t.Run(msg, func(t *testing.T) {
			err := &providerError{msg: msg}
			req := &llm.ChatRequest{Model: "claude-opus-5-5"}
			if p.RecoverRequest(req, err) {
				t.Errorf("RecoverRequest(%q) = true, want false (non-reasoning error must not trigger recovery)", msg)
			}
			if req.RetryDropThinking || req.RetryDropOutputConfig {
				t.Errorf("RecoverRequest mutated req for non-reasoning error %q", msg)
			}
		})
	}
}

func TestRecoverRequest_RecoverableMixedErrorMessages(t *testing.T) {
	// Errors that mention a recoverable field + "not supported" /
	// "does not support" + the Anthropic 400 envelope must trigger
	// recovery. The heuristic is substring-based and accepts a
	// loose match.
	p := newAnthropicProvider()
	type tc struct {
		model string
		body  string
	}
	cases := []tc{
		{"claude-opus-5-5", `{"type":"error","error":{"type":"invalid_request_error","message":"output_config.effort is not supported on this model"}}`},
		{"claude-opus-4-5", `{"type":"error","error":{"type":"invalid_request_error","message":"thinking is not supported"}}`},
		{"claude-opus-4-7", `{"type":"error","error":{"type":"invalid_request_error","message":"effort does not support 'xhigh' on this generation"}}`},
		{"claude-opus-4-5", `{"type":"error","error":{"type":"invalid_request_error","message":"this model does not support thinking"}}`},
		{"claude-opus-4-7", `{"type":"error","error":{"type":"invalid_request_error","message":"output_config is not supported"}}`},
		{"claude-opus-4-7", `{"type":"error","error":{"type":"invalid_request_error","message":"tools not supported with output_config on this model"}}`},
	}
	for _, c := range cases {
		t.Run(c.body, func(t *testing.T) {
			err := &providerError{msg: c.body}
			req := &llm.ChatRequest{Model: c.model}
			if !p.RecoverRequest(req, err) {
				t.Errorf("RecoverRequest(model=%q, body=%q) = false, want true", c.model, c.body)
			}
		})
	}
}
