package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
)

type fakeProvider struct {
	models       []llm.Model
	convertReq   func(*llm.ChatRequest) ([]byte, error)
	convertResp  func([]byte) (*llm.ChatResponse, error)
	convertChunk func([]byte) (*llm.StreamChunk, bool, error)
	mu           sync.Mutex
}

func (f *fakeProvider) Name() string                                              { return "fake" }
func (f *fakeProvider) BaseURL() string                                           { return "https://fake" }
func (f *fakeProvider) Path() string                                              { return "/v1/chat" }
func (f *fakeProvider) Headers() map[string]string                                 { return nil }
func (f *fakeProvider) ConvertRequest(r *llm.ChatRequest) ([]byte, error) {
	if f.convertReq == nil {
		return []byte("{}"), nil
	}
	return f.convertReq(r)
}
func (f *fakeProvider) ConvertResponse(d []byte) (*llm.ChatResponse, error)       { return f.convertResp(d) }
func (f *fakeProvider) ConvertStreamChunk(d []byte) (*llm.StreamChunk, bool, error) { return f.convertChunk(d) }
func (f *fakeProvider) Models() []llm.Model                                       { return f.models }

type fakeProtocol struct {
	streamItems []protocol.StreamItem
	streamErr   error
	sendData    []byte
	sendErr     error
}

func (f *fakeProtocol) Send(ctx context.Context, req *protocol.Request) ([]byte, error) {
	return f.sendData, f.sendErr
}
func (f *fakeProtocol) Stream(ctx context.Context, req *protocol.Request) (<-chan protocol.StreamItem, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	ch := make(chan protocol.StreamItem, len(f.streamItems))
	for _, item := range f.streamItems {
		ch <- item
	}
	close(ch)
	return ch, nil
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestChatRejectsEmptyModel(t *testing.T) {
	core := llm.NewCore(&fakeProvider{}, &fakeProtocol{})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{})
	if err == nil {
		t.Error("expected error for empty model, got nil")
	}
}

func TestChatRejectsToolsForUnsupportedModel(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "no-tools", Vendor: "fake", SupportsTool: false}},
	}, &fakeProtocol{})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{
		Model:    "no-tools",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools:    []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "x"}}},
	})
	if err == nil {
		t.Error("expected error for tools + unsupported model, got nil")
	} else if !strings.Contains(err.Error(), "tool use") {
		t.Errorf("err = %q, want mentions tool use", err.Error())
	}
}

func TestChatAllowsToolsForSupportedModel(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake", SupportsTool: true}},
		convertReq: func(r *llm.ChatRequest) ([]byte, error) { return []byte("{}"), nil },
		convertResp: func(d []byte) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{Choices: []llm.Choice{{Message: llm.Message{Content: "ok"}}}}, nil
		},
	}, &fakeProtocol{sendData: []byte(`{}`)})

	resp, err := core.Chat(context.Background(), &llm.ChatRequest{
		Model:    "x",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools:    []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{Name: "f"}}},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Errorf("content = %q, want ok", resp.Choices[0].Message.Content)
	}
}

func TestChatSurfacesProtocolError(t *testing.T) {
	want := errors.New("network down")
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake"}},
		convertReq: func(r *llm.ChatRequest) ([]byte, error) { return []byte("{}"), nil },
	}, &fakeProtocol{sendErr: want})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{Model: "x", Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if err != want {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestStreamChatAssemblesToolCalls(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake", SupportsTool: true}},
		convertChunk: func(d []byte) (*llm.StreamChunk, bool, error) {
			raw := json.RawMessage(d)
			_ = raw
			var ev struct {
				Type        string `json:"type"`
				Index       int    `json:"index"`
				ContentBlock *struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
				Delta *struct {
					Type        string `json:"type"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(d, &ev); err != nil {
				return nil, false, err
			}
			switch ev.Type {
			case "message_start", "content_block_stop", "message_delta", "ping":
				return &llm.StreamChunk{}, false, nil
			case "content_block_start":
				if ev.ContentBlock == nil {
					return &llm.StreamChunk{}, false, nil
				}
				if ev.ContentBlock.Type != "tool_use" {
					return &llm.StreamChunk{}, false, nil
				}
				return &llm.StreamChunk{
					Choices: []llm.StreamChoice{{
						Index: ev.Index,
						Delta: llm.Message{ToolCalls: []llm.ToolCall{{
							ID:   ev.ContentBlock.ID,
							Type: "function",
							Function: llm.FunctionCall{Name: ev.ContentBlock.Name},
						}}},
					}},
				}, false, nil
			case "content_block_delta":
				if ev.Delta == nil {
					return &llm.StreamChunk{}, false, nil
				}
				chunk := &llm.StreamChunk{Choices: []llm.StreamChoice{{Index: ev.Index}}}
				switch ev.Delta.Type {
				case "text_delta":
					chunk.Choices[0].Delta.Content = "text"
				case "thinking_delta":
					chunk.Choices[0].Delta.Reasoning = "think"
				case "input_json_delta":
					chunk.Choices[0].Delta.ToolCalls = []llm.ToolCall{{
						Function: llm.FunctionCall{Arguments: ev.Delta.PartialJSON},
					}}
				}
				return chunk, false, nil
			case "message_stop":
				return nil, true, nil
			}
			return &llm.StreamChunk{}, false, nil
		},
	}, &fakeProtocol{
		streamItems: []protocol.StreamItem{
			{Data: mustEncode(t, map[string]any{
				"type": "content_block_start", "index": 0,
				"content_block": map[string]any{
					"type": "tool_use", "id": "abc", "name": "get_weather",
				},
			})},
			{Data: mustEncode(t, map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `{"loc`},
			})},
			{Data: mustEncode(t, map[string]any{
				"type": "content_block_delta", "index": 0,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": `ation":"Tokyo"}`},
			})},
			{Data: mustEncode(t, map[string]any{"type": "message_stop"})},
		},
	})

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{Model: "x", Messages: []llm.Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var assembled []llm.ToolCall
	for ev := range events {
		if ev.Err != nil {
			t.Fatal(ev.Err)
		}
		for _, c := range ev.Chunk.Choices {
			if len(c.Delta.ToolCalls) > 0 {
				assembled = append(assembled, c.Delta.ToolCalls...)
			}
		}
	}
	if len(assembled) != 1 {
		t.Fatalf("assembled = %d, want 1", len(assembled))
	}
	tc := assembled[0]
	if tc.ID != "abc" {
		t.Errorf("ID = %q, want abc", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("Name = %q, want get_weather", tc.Function.Name)
	}
	want := `{"location":"Tokyo"}`
	if tc.Function.Arguments != want {
		t.Errorf("Arguments = %q, want %q", tc.Function.Arguments, want)
	}
}

func TestChatRejectsEmptyMessages(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake"}},
	}, &fakeProtocol{})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{Model: "x"})
	if err == nil {
		t.Fatal("expected error for empty messages")
	}
	if !strings.Contains(err.Error(), "at least one message") {
		t.Errorf("err = %q", err.Error())
	}
}

func TestChatRejectsInvalidRole(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake"}},
	}, &fakeProtocol{})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{
		Model:    "x",
		Messages: []llm.Message{{Role: "bogus", Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if !strings.Contains(err.Error(), "role") {
		t.Errorf("err = %q", err.Error())
	}
}

func TestChatRejectsBadToolDefinition(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake", SupportsTool: true}},
	}, &fakeProtocol{})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{
		Model:    "x",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
		Tools:    []llm.ToolDef{{Type: "function", Function: llm.FunctionDef{}}}, // missing Name
	})
	if err == nil {
		t.Fatal("expected error for tool without name")
	}
}

func TestChatRejectsInvalidToolChoice(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake"}},
	}, &fakeProtocol{})
	_, err := core.Chat(context.Background(), &llm.ChatRequest{
		Model:      "x",
		Messages:   []llm.Message{{Role: "user", Content: "hi"}},
		ToolChoice: &llm.ToolChoice{Mode: "bogus"},
	})
	if err == nil {
		t.Fatal("expected error for invalid tool_choice mode")
	}
}

func TestStreamChatPropagatesUsage(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake"}},
		convertChunk: func(d []byte) (*llm.StreamChunk, bool, error) {
			s := string(d)
			if strings.Contains(s, "message_delta") {
				return &llm.StreamChunk{Usage: &llm.Usage{
					PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
				}}, false, nil
			}
			if s == `{"type":"message_stop"}` {
				return nil, true, nil
			}
			return &llm.StreamChunk{}, false, nil
		},
	}, &fakeProtocol{streamItems: []protocol.StreamItem{
		{Data: []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":50}}`)},
		{Data: []byte(`{"type":"message_stop"}`)},
	}})

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "x",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawUsage bool
	for ev := range events {
		if ev.Err != nil {
			t.Fatal(ev.Err)
		}
		if ev.Chunk != nil && ev.Chunk.Usage != nil {
			sawUsage = true
			if ev.Chunk.Usage.CompletionTokens != 50 {
				t.Errorf("CompletionTokens = %d, want 50", ev.Chunk.Usage.CompletionTokens)
			}
		}
	}
	if !sawUsage {
		t.Error("expected stream event with Usage")
	}
}

func TestStreamChatPropagatesFinishReason(t *testing.T) {
	core := llm.NewCore(&fakeProvider{
		models: []llm.Model{{ID: "x", Vendor: "fake"}},
		convertChunk: func(d []byte) (*llm.StreamChunk, bool, error) {
			s := string(d)
			if strings.Contains(s, "message_delta") {
				return &llm.StreamChunk{Choices: []llm.StreamChoice{{
					FinishReason: llm.FinishReasonToolUse,
				}}}, false, nil
			}
			if s == `{"type":"message_stop"}` {
				return nil, true, nil
			}
			return &llm.StreamChunk{}, false, nil
		},
	}, &fakeProtocol{streamItems: []protocol.StreamItem{
		{Data: []byte(`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`)},
		{Data: []byte(`{"type":"message_stop"}`)},
	}})

	events, err := core.StreamChat(context.Background(), &llm.ChatRequest{
		Model:    "x",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sawFinish bool
	for ev := range events {
		if ev.Err != nil {
			t.Fatal(ev.Err)
		}
		for _, c := range ev.Chunk.Choices {
			if c.FinishReason == llm.FinishReasonToolUse {
				sawFinish = true
			}
		}
	}
	if !sawFinish {
		t.Error("expected stream event with FinishReasonToolUse")
	}
}
