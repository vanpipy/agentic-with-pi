package providers_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestConvertResponseMessageStart(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"message_start"}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("message_start should not be done")
	}
	if chunk == nil {
		t.Fatal("chunk nil")
	}
	if len(chunk.Choices) != 0 {
		t.Errorf("expected no choices, got %d", len(chunk.Choices))
	}
}

func TestConvertResponseContentBlockStop(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_stop"}`))
	if err != nil {
		t.Fatal(err)
	}
	if done || chunk == nil {
		t.Errorf("done=%v chunk=%v, want done=false chunk non-nil", done, chunk)
	}
}

func TestConvertResponsePing(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("ping should not be done")
	}
	if chunk == nil {
		t.Fatal("chunk nil")
	}
}

func TestConvertResponseContentBlockStartText(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("content_block_start text should not be done")
	}
	if chunk == nil || len(chunk.Choices) != 0 {
		t.Errorf("text start should produce empty chunk: %+v", chunk)
	}
}

func TestConvertResponseContentBlockStartThinking(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","signature":"sig123"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("thinking start should not be done")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("want 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.ReasoningSig != "sig123" {
		t.Errorf("ReasoningSig = %q, want sig123", chunk.Choices[0].Delta.ReasoningSig)
	}
	if chunk.Choices[0].Index != 0 {
		t.Errorf("Index = %d, want 0", chunk.Choices[0].Index)
	}
}

func TestConvertResponseContentBlockStartRedactedThinking(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"redacted-data"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("redacted_thinking start should not be done")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("want 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.ReasoningSig != "redacted-data" {
		t.Errorf("ReasoningSig = %q, want redacted-data", chunk.Choices[0].Delta.ReasoningSig)
	}
	if chunk.Choices[0].Index != 1 {
		t.Errorf("Index = %d, want 1", chunk.Choices[0].Index)
	}
}

func TestConvertResponseContentBlockStartToolUse(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_42","name":"bash","input":{"cmd":"ls"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("tool_use start should not be done")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("want 1 choice, got %d", len(chunk.Choices))
	}
	tc := chunk.Choices[0].Delta.ToolCalls
	if len(tc) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(tc))
	}
	if tc[0].ID != "toolu_42" {
		t.Errorf("ToolCall.ID = %q, want toolu_42", tc[0].ID)
	}
	if tc[0].Type != "function" {
		t.Errorf("ToolCall.Type = %q, want function", tc[0].Type)
	}
	if tc[0].Function.Name != "bash" {
		t.Errorf("ToolCall.Function.Name = %q, want bash", tc[0].Function.Name)
	}
	if chunk.Choices[0].Index != 2 {
		t.Errorf("Index = %d, want 2", chunk.Choices[0].Index)
	}
}

func TestConvertResponseContentBlockStartNilContentBlock(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_start","index":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if done || chunk == nil || len(chunk.Choices) != 0 {
		t.Errorf("nil content_block should produce empty chunk: %+v done=%v", chunk, done)
	}
}

func TestConvertResponseContentBlockStartUnknownType(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"unknown_block"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done || chunk == nil || len(chunk.Choices) != 0 {
		t.Errorf("unknown block should produce empty chunk: %+v done=%v", chunk, done)
	}
}

func TestConvertResponseContentBlockDeltaText(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("text_delta should not be done")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("want 1 choice, got %d", len(chunk.Choices))
	}
	if chunk.Choices[0].Delta.Content != "hello" {
		t.Errorf("Content = %q, want hello", chunk.Choices[0].Delta.Content)
	}
}

func TestConvertResponseContentBlockDeltaThinking(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reasoning content"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("thinking_delta should not be done")
	}
	if chunk.Choices[0].Delta.Reasoning != "reasoning content" {
		t.Errorf("Reasoning = %q, want reasoning content", chunk.Choices[0].Delta.Reasoning)
	}
}

func TestConvertResponseContentBlockDeltaInputJSON(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"cmd\":"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Error("input_json_delta should not be done")
	}
	if len(chunk.Choices) != 1 {
		t.Fatalf("want 1 choice, got %d", len(chunk.Choices))
	}
	tc := chunk.Choices[0].Delta.ToolCalls
	if len(tc) != 1 || tc[0].Function.Arguments != `{"cmd":` {
		t.Errorf("partial_json not propagated: %+v", tc)
	}
}

func TestConvertResponseContentBlockDeltaUnknownType(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"unknown_delta"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if done || chunk == nil || len(chunk.Choices) != 1 {
		t.Errorf("unknown delta type should still produce empty choice: %+v done=%v", chunk, done)
	}
	if chunk.Choices[0].Delta.Content != "" || chunk.Choices[0].Delta.Reasoning != "" {
		t.Errorf("delta fields should be empty: %+v", chunk.Choices[0].Delta)
	}
}

func TestConvertResponseContentBlockDeltaNilDelta(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"content_block_delta","index":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if done || chunk == nil || len(chunk.Choices) != 0 {
		t.Errorf("nil delta should produce empty chunk: %+v done=%v", chunk, done)
	}
}

func TestConvertResponseMessageStop(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"message_stop"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Error("message_stop should be done")
	}
	if chunk != nil {
		t.Errorf("chunk = %+v, want nil on message_stop", chunk)
	}
}

func TestConvertResponseErrorEvent(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"error","error":{"message":"rate limit"}}`))
	if err == nil {
		t.Fatal("expected error from error event")
	}
	if done {
		t.Error("error event should not be done")
	}
	if chunk != nil {
		t.Errorf("chunk = %+v, want nil on error event", chunk)
	}
	if !strings.Contains(err.Error(), "anthropic stream error") {
		t.Errorf("err = %q, want it to mention 'anthropic stream error'", err)
	}
	var llmErr *llm.Error
	if !errors.As(err, &llmErr) {
		t.Errorf("err is not *llm.Error: %v", err)
	} else if llmErr.Kind != llm.ErrorKindVendor {
		t.Errorf("Kind = %v, want Vendor", llmErr.Kind)
	}
}

func TestConvertResponseInvalidJSON(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`not json at all`))
	if err == nil {
		t.Fatal("expected error from invalid JSON")
	}
	if done || chunk != nil {
		t.Errorf("invalid JSON should return chunk=nil done=false: chunk=%+v done=%v", chunk, done)
	}
	if !strings.Contains(err.Error(), "failed to parse stream event") {
		t.Errorf("err = %q, want parse error message", err)
	}
	var llmErr *llm.Error
	if !errors.As(err, &llmErr) {
		t.Errorf("err is not *llm.Error: %v", err)
	} else if llmErr.Kind != llm.ErrorKindClient {
		t.Errorf("Kind = %v, want Client", llmErr.Kind)
	}
}

func TestConvertResponseUnknownType(t *testing.T) {
	p := newProvider()
	chunk, done, err := p.ConvertResponse([]byte(`{"type":"some_unknown_type"}`))
	if err != nil {
		t.Fatal(err)
	}
	if done || chunk == nil || len(chunk.Choices) != 0 {
		t.Errorf("unknown type should produce empty chunk: %+v done=%v", chunk, done)
	}
}

func TestParseAnthropicStopReasonAll(t *testing.T) {
	p := newProvider()
	cases := []struct {
		raw  string
		want llm.FinishReason
	}{
		{"end_turn", llm.FinishReasonStop},
		{"stop_sequence", llm.FinishReasonStop},
		{"max_tokens", llm.FinishReasonLength},
		{"tool_use", llm.FinishReasonToolUse},
		{"content_filter", llm.FinishReasonContentFilter},
		{"error", llm.FinishReasonError},
		{"weird_thing", llm.FinishReasonUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			chunk, _, err := p.ConvertResponse([]byte(`{"type":"message_delta","delta":{"stop_reason":"` + tc.raw + `"}}`))
			if err != nil {
				t.Fatal(err)
			}
			if len(chunk.Choices) != 1 || chunk.Choices[0].FinishReason != tc.want {
				t.Errorf("stop_reason=%q -> FinishReason=%v, want %v", tc.raw, chunk.Choices, tc.want)
			}
		})
	}
}

func TestConvertRequestWithToolCallsAndReasoning(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{
				Role:         "assistant",
				Content:      "answer",
				Reasoning:    "thought process",
				ReasoningSig: "sig-abc",
				ToolCalls: []llm.ToolCall{{
					ID:       "call_1",
					Type:     "function",
					Function: llm.FunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`},
				}},
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
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	content := msgs[0].(map[string]any)["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("want 3 blocks (thinking + text + tool_use), got %d: %+v", len(content), content)
	}
	thinking := content[0].(map[string]any)
	if thinking["type"] != "thinking" || thinking["signature"] != "sig-abc" {
		t.Errorf("thinking block wrong: %+v", thinking)
	}
	if thinking["thinking"] != "thought process" {
		t.Errorf("thinking content wrong: %+v", thinking)
	}
	textBlock := content[1].(map[string]any)
	if textBlock["type"] != "text" || textBlock["text"] != "answer" {
		t.Errorf("text block wrong: %+v", textBlock)
	}
	toolBlock := content[2].(map[string]any)
	if toolBlock["type"] != "tool_use" || toolBlock["id"] != "call_1" || toolBlock["name"] != "bash" {
		t.Errorf("tool_use block wrong: %+v", toolBlock)
	}
}

func TestConvertRequestAssistantOnlyReasoning(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{{
			Role:         "assistant",
			Reasoning:    "r",
			ReasoningSig: "sig",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	content := got["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["type"] != "thinking" {
		t.Errorf("expected single thinking block: %+v", content)
	}
}

func TestConvertRequestAssistantOnlyToolCalls(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:       "c1",
				Type:     "function",
				Function: llm.FunctionCall{Name: "bash"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	content := got["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["type"] != "tool_use" {
		t.Errorf("expected single tool_use block: %+v", content)
	}
}

func TestConvertRequestPlainAssistantContent(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "reply"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	asst := got["messages"].([]any)[1].(map[string]any)
	content := asst["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("assistant content = %v, want single text block", asst["content"])
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "reply" {
		t.Errorf("assistant text block = %v, want {type:text, text:reply}", block)
	}
}

func TestConvertRequestNoMessagesDefaultUserEmpty(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("want 1 default message, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "user" {
		t.Errorf("default message role = %v, want user", msgs[0])
	}
}

func TestConvertRequestIgnoresUnknownRoles(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model: "MiniMax-M3",
		Messages: []llm.Message{
			{Role: "function", Content: "should be dropped"},
			{Role: "user", Content: "hi"},
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
	if len(msgs) != 1 || msgs[0].(map[string]any)["role"] != "user" {
		t.Errorf("unknown role should be ignored: %+v", msgs)
	}
	// The dropped function role should not leak into the user message
	// (it would have produced two blocks {type:text, text:"hi"} plus a
	// leak block). Verify content is exactly one text block.
	content := msgs[0].(map[string]any)["content"].([]any)
	if len(content) != 1 {
		t.Errorf("user content = %v, want single text block (no leak from dropped role)", content)
	}
}

func TestConvertRequestNonReasoningModelOmitsThinking(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:    "no-such-model",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["thinking"]; ok {
		t.Errorf("thinking should be omitted for non-reasoning model: %+v", got)
	}
}

func TestConvertRequestThinkingBudgetFloor(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:     "MiniMax-M3",
		Messages:  []llm.Message{{Role: "user", Content: "hi"}},
		MaxTokens: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	thinking := got["thinking"].(map[string]any)
	if budget, _ := thinking["budget_tokens"].(float64); budget != 1024 {
		t.Errorf("budget_tokens = %v, want 1024 floor", budget)
	}
}

func TestConvertRequestIncludesTools(t *testing.T) {
	p := newProvider()
	body, err := p.ConvertRequest(&llm.ChatRequest{
		Model:    "MiniMax-M3",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools: []llm.ToolDef{{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        "bash",
				Description: "run bash",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	tools := got["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	t0 := tools[0].(map[string]any)
	if t0["name"] != "bash" || t0["description"] != "run bash" {
		t.Errorf("tool fields wrong: %+v", t0)
	}
	schema := t0["input_schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Errorf("input_schema = %+v", schema)
	}
}
