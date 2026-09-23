package providers_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

func newProvider() *providers.MiniMaxProvider {
	return providers.NewMiniMaxProvider("test-key")
}

func TestProviderMetadata(t *testing.T) {
	p := newProvider()
	if p.Name() != "minimax" {
		t.Errorf("Name = %q, want minimax", p.Name())
	}
	if p.BaseURL() != "https://api.minimaxi.com/anthropic" {
		t.Errorf("BaseURL = %q", p.BaseURL())
	}
	if p.Path() != "/v1/messages" {
		t.Errorf("Path = %q", p.Path())
	}
	h := p.Headers()
	if h["x-api-key"] != "test-key" {
		t.Errorf("x-api-key = %q", h["x-api-key"])
	}
	if h["anthropic-version"] != "2023-06-01" {
		t.Errorf("anthropic-version = %q", h["anthropic-version"])
	}
}

func TestModelsIncludesMiniMaxM3(t *testing.T) {
	p := newProvider()
	models := p.Models()
	if len(models) == 0 {
		t.Fatal("Models() returned empty")
	}
	var found bool
	for _, m := range models {
		if m.ID == "MiniMax-M3" {
			found = true
			if !m.SupportsTool {
				t.Error("MiniMax-M3 should support tools")
			}
			if !m.SupportsStreaming {
				t.Error("MiniMax-M3 should support streaming")
			}
		}
	}
	if !found {
		t.Error("MiniMax-M3 not in Models()")
	}
}

func TestConvertRequestBasicChat(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
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
	if got["model"] != "MiniMax-M3" {
		t.Errorf("model = %v", got["model"])
	}
	if got["max_tokens"].(float64) != 1024 {
		t.Errorf("max_tokens = %v", got["max_tokens"])
	}
	if got["temperature"].(float64) != 0.7 {
		t.Errorf("temperature = %v", got["temperature"])
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v", got["messages"])
	}
	msg := msgs[0].(map[string]any)
	if msg["role"] != "user" || msg["content"] != "hello" {
		t.Errorf("msg = %v", msg)
	}
}

func TestConvertRequestDefaultMaxTokens(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "x"}},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	if got["max_tokens"].(float64) != 4096 {
		t.Errorf("default max_tokens = %v, want 4096", got["max_tokens"])
	}
}

func TestConvertRequestExtractsSystemPrompt(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{Role: "system", Content: "you are a pirate"},
			{Role: "user", Content: "hello"},
		},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	if got["system"] != "you are a pirate" {
		t.Errorf("system = %v, want top-level", got["system"])
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Errorf("messages len = %d, want 1 (system extracted)", len(msgs))
	}
}

func TestConvertRequestToolResultAsUserMessage(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{Role: "user", Content: "weather?"},
			{Role: "tool", ToolCallID: "toolu_123", Content: "sunny, 22C"},
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
	block := content[0].(map[string]any)
	if block["type"] != "tool_result" {
		t.Errorf("block.type = %v", block["type"])
	}
	if block["tool_use_id"] != "toolu_123" {
		t.Errorf("tool_use_id = %v", block["tool_use_id"])
	}
}

func TestConvertRequestToolChoice(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:      "MiniMax-M3",
		Messages:   []llm.Message{{Role: "user", Content: "hi"}},
		Tools:      []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "f"}}},
		ToolChoice: &llm.ToolChoice{Mode: "tool", Name: "f"},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	tc := got["tool_choice"].(map[string]any)
	if tc["type"] != "tool" {
		t.Errorf("type = %v, want tool", tc["type"])
	}
	if tc["name"] != "f" {
		t.Errorf("name = %v, want f", tc["name"])
	}
}

func TestConvertRequestToolChoiceOmitted(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	if _, ok := got["tool_choice"]; ok {
		t.Error("tool_choice should be omitted when nil")
	}
}

func TestConvertRequestStopSequences(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model:         "MiniMax-M3",
		Messages:      []llm.Message{{Role: "user", Content: "hi"}},
		StopSequences: []string{"END", "STOP"},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	ss := got["stop_sequences"].([]any)
	if len(ss) != 2 || ss[0] != "END" || ss[1] != "STOP" {
		t.Errorf("stop_sequences = %v, want [END STOP]", ss)
	}
}

func TestConvertRequestMultipleSystemMessages(t *testing.T) {
	p := newProvider()
	body, _ := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{Role: "system", Content: "be brief"},
			{Role: "system", Content: "be friendly"},
			{Role: "user", Content: "hi"},
		},
	})
	var got map[string]any
	json.Unmarshal(body, &got)
	want := "be brief\n\nbe friendly"
	if got["system"] != want {
		t.Errorf("system = %v, want %q", got["system"], want)
	}
}

func TestConvertResponseMessageDeltaUsage(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":12,"output_tokens":34}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("message_delta should not be done")
	}
	if chunk.Usage == nil {
		t.Fatal("Usage should be set")
	}
	if chunk.Usage.PromptTokens != 12 {
		t.Errorf("PromptTokens = %d, want 12", chunk.Usage.PromptTokens)
	}
	if chunk.Usage.CompletionTokens != 34 {
		t.Errorf("CompletionTokens = %d, want 34", chunk.Usage.CompletionTokens)
	}
	if len(chunk.Choices) == 0 || chunk.Choices[0].FinishReason != llm.FinishReasonStop {
		t.Errorf("FinishReason = %v, want Stop", chunk.Choices)
	}
}

func TestConvertResponseMessageDeltaOnlyUsage(t *testing.T) {
	p := newProvider()
	chunk, _, err := p.ConvertResponse([]byte(`{"type":"message_delta","usage":{"input_tokens":5,"output_tokens":7}}`))
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Usage == nil || chunk.Usage.CompletionTokens != 7 {
		t.Errorf("Usage = %+v, want CompletionTokens=7", chunk.Usage)
	}
	if len(chunk.Choices) != 0 {
		t.Errorf("expected no choices, got %d", len(chunk.Choices))
	}
}

func TestConvertRequestEnablesThinkingForReasoningModel(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:     "MiniMax-M3",
		Messages:  []llm.Message{{Role: "user", Content: "hello"}},
		MaxTokens: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	thinking, ok := got["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking field missing: %v", got)
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %v, want enabled", thinking["type"])
	}
	if budget, _ := thinking["budget_tokens"].(float64); budget != 2048 {
		t.Errorf("thinking.budget_tokens = %v, want 2048 (half of max_tokens 4096)", budget)
	}
}

func TestConvertRequestCappedThinkingBudget(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:     "MiniMax-M3",
		Messages:  []llm.Message{{Role: "user", Content: "hello"}},
		MaxTokens: 32000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking := got["thinking"].(map[string]any)
	if budget, _ := thinking["budget_tokens"].(float64); budget != 8192 {
		t.Errorf("thinking.budget_tokens = %v, want 8192 (capped)", budget)
	}
}

func TestConvertRequestEnablesThinkingWithDefaultMaxTokens(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(body, &got)
	thinking, ok := got["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking should be enabled by default for MiniMax-M3, got %v", got)
	}
	if budget, _ := thinking["budget_tokens"].(float64); budget != 2048 {
		t.Errorf("thinking.budget_tokens = %v, want 2048 (half of default 4096)", budget)
	}
}
