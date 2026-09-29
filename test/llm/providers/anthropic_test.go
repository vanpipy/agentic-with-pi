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
