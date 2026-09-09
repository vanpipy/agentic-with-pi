package providers_test

import (
	"encoding/json"
	"strings"
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

func TestConvertResponseTextAndReasoning(t *testing.T) {
	p := newProvider()
	respJSON := `{
		"id": "msg_01",
		"model": "MiniMax-M3",
		"stop_reason": "end_turn",
		"content": [
			{"type": "thinking", "thinking": "let me think"},
			{"type": "text", "text": "answer"}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, err := p.ConvertResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	c := resp.Choices[0]
	if c.Message.Content != "answer" {
		t.Errorf("Content = %q, want answer", c.Message.Content)
	}
	if c.Message.Reasoning != "let me think" {
		t.Errorf("Reasoning = %q", c.Message.Reasoning)
	}
	if c.FinishReason != llm.FinishReasonStop {
		t.Errorf("FinishReason = %v, want Stop", c.FinishReason)
	}
	if c.FinishReason.String() != "stop" {
		t.Errorf("FinishReason.String() = %q", c.FinishReason.String())
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d", resp.Usage.PromptTokens)
	}
}

func TestConvertResponseToolUse(t *testing.T) {
	p := newProvider()
	respJSON := `{
		"id": "msg_01",
		"model": "MiniMax-M3",
		"stop_reason": "tool_use",
		"content": [
			{"type": "text", "text": "let me check"},
			{"type": "tool_use", "id": "toolu_abc", "name": "get_weather", "input": {"location": "Tokyo"}}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, err := p.ConvertResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].FinishReason != llm.FinishReasonToolUse {
		t.Errorf("FinishReason = %v, want ToolUse", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %d", len(resp.Choices[0].Message.ToolCalls))
	}
	tc := resp.Choices[0].Message.ToolCalls[0]
	if tc.ID != "toolu_abc" {
		t.Errorf("ID = %q", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("Name = %q", tc.Function.Name)
	}
	if !strings.Contains(tc.Function.Arguments, "Tokyo") {
		t.Errorf("Args = %q", tc.Function.Arguments)
	}
}

func TestConvertResponseVendorError(t *testing.T) {
	p := newProvider()
	respJSON := `{
		"type": "error",
		"error": {"type": "rate_limit", "message": "too many requests"}
	}`
	_, err := p.ConvertResponse([]byte(respJSON))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	llmErr, ok := err.(*llm.Error)
	if !ok {
		t.Fatalf("err is not *llm.Error: %T", err)
	}
	if llmErr.Kind != llm.ErrorKindVendor {
		t.Errorf("Kind = %v, want Vendor", llmErr.Kind)
	}
}

func TestConvertResponseMalformedJSON(t *testing.T) {
	p := newProvider()
	_, err := p.ConvertResponse([]byte("not json"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	llmErr, ok := err.(*llm.Error)
	if !ok {
		t.Fatalf("err is not *llm.Error: %T", err)
	}
	if llmErr.Kind != llm.ErrorKindClient {
		t.Errorf("Kind = %v, want Client", llmErr.Kind)
	}
}

func TestConvertResponseCapturesSignature(t *testing.T) {
	p := newProvider()
	respJSON := `{
		"id": "msg_01",
		"model": "MiniMax-M3",
		"stop_reason": "end_turn",
		"content": [
			{"type": "thinking", "thinking": "let me think", "signature": "sig-xyz"},
			{"type": "text", "text": "answer"}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, err := p.ConvertResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	msg := resp.Choices[0].Message
	if msg.Reasoning != "let me think" {
		t.Errorf("Reasoning = %q, want 'let me think'", msg.Reasoning)
	}
	if msg.ReasoningSig != "sig-xyz" {
		t.Errorf("ReasoningSig = %q, want 'sig-xyz'", msg.ReasoningSig)
	}
}

func TestConvertResponseRedactedThinking(t *testing.T) {
	p := newProvider()
	respJSON := `{
		"id": "msg_01",
		"model": "MiniMax-M3",
		"stop_reason": "end_turn",
		"content": [
			{"type": "redacted_thinking", "data": "encrypted-blob-base64"},
			{"type": "text", "text": "answer"}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, err := p.ConvertResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	msg := resp.Choices[0].Message
	if msg.Reasoning != "[Reasoning redacted]" {
		t.Errorf("Reasoning = %q, want '[Reasoning redacted]'", msg.Reasoning)
	}
	if msg.ReasoningSig != "encrypted-blob-base64" {
		t.Errorf("ReasoningSig = %q, want 'encrypted-blob-base64'", msg.ReasoningSig)
	}
}

func TestConvertResponseNoSigWhenNoThinking(t *testing.T) {
	p := newProvider()
	respJSON := `{
		"id": "msg_01",
		"model": "MiniMax-M3",
		"stop_reason": "end_turn",
		"content": [{"type": "text", "text": "hi"}],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, err := p.ConvertResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	msg := resp.Choices[0].Message
	if msg.ReasoningSig != "" {
		t.Errorf("ReasoningSig = %q, want empty when no thinking", msg.ReasoningSig)
	}
}

func TestRoundTripThinkingSignature(t *testing.T) {
	p := newProvider()

	respJSON := `{
		"id": "msg_01",
		"model": "MiniMax-M3",
		"stop_reason": "tool_use",
		"content": [
			{"type": "thinking", "thinking": "I need to call a tool", "signature": "sig-round-trip"},
			{"type": "tool_use", "id": "toolu_1", "name": "f", "input": {}}
		],
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	resp, err := p.ConvertResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}

	asstMsg := resp.Choices[0].Message
	if asstMsg.ReasoningSig != "sig-round-trip" {
		t.Fatalf("Response.ReasoningSig = %q, want 'sig-round-trip'", asstMsg.ReasoningSig)
	}

	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
			asstMsg,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2", len(msgs))
	}
	out := msgs[1].(map[string]any)
	if out["role"] != "assistant" {
		t.Errorf("role = %v, want assistant", out["role"])
	}

	content := out["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("blocks = %d, want 2 (thinking + tool_use)", len(content))
	}

	thinkingBlock := content[0].(map[string]any)
	if thinkingBlock["type"] != "thinking" {
		t.Errorf("block 0 type = %v, want thinking", thinkingBlock["type"])
	}
	if thinkingBlock["signature"] != "sig-round-trip" {
		t.Errorf("block 0 signature = %v, want sig-round-trip", thinkingBlock["signature"])
	}
	if thinkingBlock["thinking"] != "I need to call a tool" {
		t.Errorf("block 0 thinking = %v", thinkingBlock["thinking"])
	}
}

func TestRequestThinkingSignatureOnly(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{
				Role:         "assistant",
				Reasoning:    "thinking text",
				ReasoningSig: "sig-only",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	msgs := got["messages"].([]any)
	out := msgs[0].(map[string]any)

	content := out["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("blocks = %d, want 1", len(content))
	}
	block := content[0].(map[string]any)
	if block["type"] != "thinking" {
		t.Errorf("type = %v, want thinking", block["type"])
	}
	if block["signature"] != "sig-only" {
		t.Errorf("signature = %v, want sig-only", block["signature"])
	}
	if block["thinking"] != "thinking text" {
		t.Errorf("thinking = %v", block["thinking"])
	}
}
