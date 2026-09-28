package protocol_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

func TestBuildAnthropicRequest_MinimalUserMessage(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 1024,
		Stream:    true,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.ContentText{Text: "hello"},
			}},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["model"] != "claude-opus-4-7" {
		t.Errorf("model = %v, want claude-opus-4-7", got["model"])
	}
	if v, ok := got["max_tokens"].(float64); !ok || int(v) != 1024 {
		t.Errorf("max_tokens = %v, want 1024", got["max_tokens"])
	}
	if v, ok := got["stream"].(bool); !ok || !v {
		t.Errorf("stream = %v, want true", got["stream"])
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("messages = %v, want 1 entry", got["messages"])
	}
	m0 := msgs[0].(map[string]any)
	if m0["role"] != "user" {
		t.Errorf("role = %v, want user", m0["role"])
	}
	cs, ok := m0["content"].([]any)
	if !ok || len(cs) != 1 {
		t.Fatalf("content = %v, want 1 entry", m0["content"])
	}
	c0 := cs[0].(map[string]any)
	if c0["type"] != "text" {
		t.Errorf("content[0].type = %v, want text", c0["type"])
	}
	if c0["text"] != "hello" {
		t.Errorf("content[0].text = %v, want hello", c0["text"])
	}
	if _, present := got["system"]; present {
		t.Errorf("system present in minimal request, expected absent")
	}
	if _, present := got["tools"]; present {
		t.Errorf("tools present in minimal request, expected absent")
	}
}

func TestBuildAnthropicRequest_WithSystemBlocks(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 2048,
		System: []llm.ContentBlock{
			llm.ContentText{Text: "you are a researcher", CacheControl: llm.CacheEphemeral()},
		},
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.ContentText{Text: "hi"},
			}},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sys, ok := got["system"].([]any)
	if !ok || len(sys) != 1 {
		t.Fatalf("system = %v, want 1 entry", got["system"])
	}
	s0 := sys[0].(map[string]any)
	if s0["type"] != "text" {
		t.Errorf("system[0].type = %v, want text", s0["type"])
	}
	if s0["text"] != "you are a researcher" {
		t.Errorf("system[0].text = %v", s0["text"])
	}
	cc, ok := s0["cache_control"].(map[string]any)
	if !ok {
		t.Fatalf("system[0].cache_control absent or not object")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("cache_control.type = %v, want ephemeral", cc["type"])
	}
}

func TestBuildAnthropicRequest_WithToolsArray(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 2048,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.ContentText{Text: "do thing"}}},
		},
		Tools: []anthropic.AnthropicTool{
			{Name: "bash", Description: "shell tool", InputSchema: schema},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %v, want 1 entry", got["tools"])
	}
	t0 := tools[0].(map[string]any)
	if t0["name"] != "bash" {
		t.Errorf("tools[0].name = %v", t0["name"])
	}
	if t0["description"] != "shell tool" {
		t.Errorf("tools[0].description = %v", t0["description"])
	}
	schemaAny, ok := t0["input_schema"]
	if !ok {
		t.Fatalf("input_schema absent")
	}
	schemaJSON, _ := json.Marshal(schemaAny)
	if !strings.Contains(string(schemaJSON), `"q"`) {
		t.Errorf("input_schema missing q property: %s", schemaJSON)
	}
}

func TestBuildAnthropicRequest_TemperatureAndStopSequences(t *testing.T) {
	temp := 0.5
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 1024,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.ContentText{Text: "x"}}},
		},
		Temperature:   &temp,
		StopSequences: []string{"STOP"},
		Metadata:      map[string]string{"user_id": "u-42"},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := got["temperature"].(float64); !ok || v != 0.5 {
		t.Errorf("temperature = %v, want 0.5", got["temperature"])
	}
	stops, ok := got["stop_sequences"].([]any)
	if !ok || len(stops) != 1 || stops[0] != "STOP" {
		t.Errorf("stop_sequences = %v, want [STOP]", got["stop_sequences"])
	}
	md, ok := got["metadata"].(map[string]any)
	if !ok || md["user_id"] != "u-42" {
		t.Errorf("metadata = %v, want user_id=u-42", got["metadata"])
	}
}

func TestBuildAnthropicRequest_ToolUseAndToolResultBlocks(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 4096,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.ContentText{Text: "find files"},
			}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				llm.ContentToolUse{ID: "tu-1", Name: "bash", Input: []byte(`{"cmd":"ls"}`)},
			}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.ContentToolResult{ToolUseID: "tu-1", Content: []llm.ContentBlock{
					llm.ContentText{Text: "out.txt"},
				}, IsError: false},
			}},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages len = %d, want 3", len(msgs))
	}

	asstMsg := msgs[1].(map[string]any)
	asstContent := asstMsg["content"].([]any)
	tuBlock := asstContent[0].(map[string]any)
	if tuBlock["type"] != "tool_use" {
		t.Fatalf("assistant[0].type = %v, want tool_use", tuBlock["type"])
	}
	if tuBlock["id"] != "tu-1" {
		t.Errorf("assistant[0].id = %v", tuBlock["id"])
	}
	if tuBlock["name"] != "bash" {
		t.Errorf("assistant[0].name = %v", tuBlock["name"])
	}
	input, ok := tuBlock["input"]
	if !ok {
		t.Fatalf("assistant[0].input absent")
	}
	inputJSON, _ := json.Marshal(input)
	if !strings.Contains(string(inputJSON), `"cmd":"ls"`) {
		t.Errorf("assistant[0].input JSON = %s", inputJSON)
	}

	toolMsg := msgs[2].(map[string]any)
	toolContent := toolMsg["content"].([]any)
	trBlock := toolContent[0].(map[string]any)
	if trBlock["type"] != "tool_result" {
		t.Fatalf("tool[0].type = %v, want tool_result", trBlock["type"])
	}
	if trBlock["tool_use_id"] != "tu-1" {
		t.Errorf("tool[0].tool_use_id = %v", trBlock["tool_use_id"])
	}
	if v, ok := trBlock["is_error"].(bool); ok && v {
		t.Errorf("tool[0].is_error = true, want false or absent")
	}
	trInner, ok := trBlock["content"].([]any)
	if !ok || len(trInner) != 1 {
		t.Errorf("tool[0].content = %v, want 1 entry", trBlock["content"])
	}
}

func TestBuildAnthropicRequest_ImageContent(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 1024,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.ContentImage{MediaType: "image/png", Data: "BASE64"},
			}},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgs := got["messages"].([]any)
	imgBlock := msgs[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if imgBlock["type"] != "image" {
		t.Fatalf("type = %v, want image", imgBlock["type"])
	}
	src, ok := imgBlock["source"].(map[string]any)
	if !ok {
		t.Fatalf("source absent")
	}
	if src["type"] != "base64" {
		t.Errorf("source.type = %v, want base64", src["type"])
	}
	if src["media_type"] != "image/png" {
		t.Errorf("source.media_type = %v, want image/png", src["media_type"])
	}
	if src["data"] != "BASE64" {
		t.Errorf("source.data = %v", src["data"])
	}
}

func TestBuildAnthropicRequest_MessageCacheControl(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 1024,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.ContentText{Text: "pinned", CacheControl: llm.CacheEphemeral1h()},
			}},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgs := got["messages"].([]any)
	block := msgs[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	cc, ok := block["cache_control"].(map[string]any)
	if !ok {
		t.Fatalf("cache_control absent on message block")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("type = %v, want ephemeral", cc["type"])
	}
	if cc["ttl"] != "1h" {
		t.Errorf("ttl = %v, want 1h", cc["ttl"])
	}
}

func TestBuildAnthropicRequest_MaxTokensZeroReturnsError(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model: "claude-opus-4-7",
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.ContentText{Text: "x"}}},
		},
	}
	_, err := anthropic.BuildAnthropicRequest(req)
	if err == nil {
		t.Fatal("expected error for max_tokens=0, got nil")
	}
}

func TestBuildAnthropicRequest_ThinkingBlockRoundtrips(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 4096,
		Messages: []anthropic.AnthropicMessage{
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				llm.ContentThinking{Text: "let me think", Signature: "sig-abc"},
				llm.ContentText{Text: "answer"},
			}},
		},
	}
	raw, err := anthropic.BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	msgs := got["messages"].([]any)
	blocks := msgs[0].(map[string]any)["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(blocks))
	}
	thinkBlock := blocks[0].(map[string]any)
	if thinkBlock["type"] != "thinking" {
		t.Errorf("type = %v, want thinking", thinkBlock["type"])
	}
	if thinkBlock["thinking"] != "let me think" {
		t.Errorf("thinking = %v", thinkBlock["thinking"])
	}
	if thinkBlock["signature"] != "sig-abc" {
		t.Errorf("signature = %v", thinkBlock["signature"])
	}
}

func TestMapToAnthropicRequest_MinimalChat(t *testing.T) {
	llmReq := llm.ChatRequest{
		Model: "claude-opus-4-7",
		Messages: []llm.Message{
			{Role: string(llm.RoleUser), Content: "hello"},
		},
		MaxTokens: 1024,
		Stream:    true,
	}
	got, err := anthropic.MapToAnthropicRequest(llmReq)
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	if got.Model != "claude-opus-4-7" {
		t.Errorf("model = %q", got.Model)
	}
	if got.MaxTokens != 1024 {
		t.Errorf("max_tokens = %d", got.MaxTokens)
	}
	if !got.Stream {
		t.Errorf("stream = false, want true")
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages len = %d", len(got.Messages))
	}
	m := got.Messages[0]
	if m.Role != llm.RoleUser {
		t.Errorf("role = %q", m.Role)
	}
	if len(m.Content) != 1 {
		t.Fatalf("content len = %d", len(m.Content))
	}
	tb, ok := m.Content[0].(llm.ContentText)
	if !ok {
		t.Fatalf("content[0] type = %T, want ContentText", m.Content[0])
	}
	if tb.Text != "hello" {
		t.Errorf("text = %q", tb.Text)
	}
}

func TestMapToAnthropicRequest_WithToolCalls(t *testing.T) {
	llmReq := llm.ChatRequest{
		Model: "claude-opus-4-7",
		Messages: []llm.Message{
			{Role: "assistant", Content: "thinking", ToolCalls: []llm.ToolCall{
				{ID: "tu-1", Type: "function", Function: llm.FunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`}},
			}},
		},
	}
	got, err := anthropic.MapToAnthropicRequest(llmReq)
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	m := got.Messages[0]
	if len(m.Content) != 2 {
		t.Fatalf("content len = %d, want 2 (text + tool_use)", len(m.Content))
	}
	tb, ok := m.Content[0].(llm.ContentText)
	if !ok || tb.Text != "thinking" {
		t.Errorf("[0] = %#v, want ContentText{thinking}", m.Content[0])
	}
	tu, ok := m.Content[1].(llm.ContentToolUse)
	if !ok {
		t.Fatalf("[1] type = %T, want ContentToolUse", m.Content[1])
	}
	if tu.ID != "tu-1" || tu.Name != "bash" {
		t.Errorf("tool_use = %+v", tu)
	}
	if string(tu.Input) != `{"cmd":"ls"}` {
		t.Errorf("input = %q", string(tu.Input))
	}
}

func TestMapToAnthropicRequest_WithToolsArray(t *testing.T) {
	llmReq := llm.ChatRequest{
		Model: "claude-opus-4-7",
		Tools: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{
				Name:        "bash",
				Description: "shell",
				Parameters:  map[string]any{"type": "object"},
			}},
		},
	}
	got, err := anthropic.MapToAnthropicRequest(llmReq)
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	if len(got.Tools) != 1 {
		t.Fatalf("tools len = %d", len(got.Tools))
	}
	if got.Tools[0].Name != "bash" || got.Tools[0].Description != "shell" {
		t.Errorf("tool = %+v", got.Tools[0])
	}
	if !strings.Contains(string(got.Tools[0].InputSchema), `"object"`) {
		t.Errorf("input_schema missing type: %s", got.Tools[0].InputSchema)
	}
}

func TestMapToAnthropicRequest_TemperaturePassthrough(t *testing.T) {
	llmReq := llm.ChatRequest{
		Model:       "claude-opus-4-7",
		Temperature: 0.7,
		Messages:    []llm.Message{{Role: "user", Content: "x"}},
	}
	got, err := anthropic.MapToAnthropicRequest(llmReq)
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	if got.Temperature == nil {
		t.Fatal("Temperature nil, want pointer")
	}
	if math.Abs(*got.Temperature-0.7) > 1e-6 {
		t.Errorf("temperature = %v, want ~0.7", *got.Temperature)
	}
}

func TestMapToAnthropicRequest_MaxTokensZeroGetsDefault(t *testing.T) {
	llmReq := llm.ChatRequest{
		Model:    "claude-opus-4-7",
		Messages: []llm.Message{{Role: "user", Content: "x"}},
	}
	got, err := anthropic.MapToAnthropicRequest(llmReq)
	if err != nil {
		t.Fatalf("MapToAnthropicRequest: %v", err)
	}
	if got.MaxTokens <= 0 {
		t.Errorf("max_tokens = %d, want default > 0", got.MaxTokens)
	}
}

func TestBuildAnthropicRequest_EmptyMessagesRejected(t *testing.T) {
	req := anthropic.AnthropicRequest{
		Model:     "claude-opus-4-7",
		MaxTokens: 1024,
	}
	_, err := anthropic.BuildAnthropicRequest(req)
	if err == nil {
		t.Fatal("expected error for empty messages, got nil")
	}
}

var _ = context.Background
