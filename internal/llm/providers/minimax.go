package providers

import (
	"encoding/json"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

type MiniMaxProvider struct {
	APIKey string
}

func NewMiniMaxProvider(apiKey string) *MiniMaxProvider {
	return &MiniMaxProvider{
		APIKey: apiKey,
	}
}

func (p *MiniMaxProvider) Name() string {
	return "minimax"
}

func (p *MiniMaxProvider) BaseURL() string {
	return "https://api.minimaxi.com/anthropic"
}

func (p *MiniMaxProvider) Path() string {
	return "/v1/messages"
}

func (p *MiniMaxProvider) Headers() map[string]string {
	return map[string]string{
		"x-api-key":         p.APIKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}
}

func (p *MiniMaxProvider) Models() []llm.Model {
	return []llm.Model{
		{
			ID:                "MiniMax-M3",
			Name:              "MiniMax M3",
			Vendor:            p.Name(),
			MaxContextTokens:  10240000,
			MaxOutputTokens:   4096,
			SupportsTool:      true,
			SupportsVision:    true,
			SupportsStreaming: true,
			SupportsReasoning: true,
		},
	}
}

type anthropicMessageContent struct {
	Type string `json:"type"`

	Text string `json:"text,omitzero"`

	Thinking  string `json:"thinking,omitzero"`
	Signature string `json:"signature,omitzero"`

	ID    string         `json:"id,omitzero"`
	Name  string         `json:"name,omitzero"`
	Input map[string]any `json:"input,omitzero"`

	ToolUseID string `json:"tool_use_id,omitzero"`
	Content   string `json:"content,omitzero"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitzero"`
}

type anthropicRequest struct {
	Model         string               `json:"model"`
	MaxTokens     int                  `json:"max_tokens"`
	Messages      []anthropicMessage   `json:"messages"`
	System        string               `json:"system,omitzero"`
	Tools         []anthropicTool      `json:"tools,omitzero"`
	ToolChoice    *anthropicToolChoice `json:"tool_choice,omitzero"`
	StopSequences []string             `json:"stop_sequences,omitzero"`
	Temperature   *float32             `json:"temperature,omitzero"`
	Stream        bool                 `json:"stream,omitzero"`
}

func (p *MiniMaxProvider) ConvertRequest(req *llm.ChatRequest) ([]byte, error) {
	var systemParts []string
	var messages []anthropicMessage

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			systemParts = append(systemParts, msg.Content)
		case "user", "assistant":
			content := convertOutboundContent(msg)
			messages = append(messages, anthropicMessage{Role: msg.Role, Content: content})
		case "tool":
			messages = append(messages, anthropicMessage{
				Role: "user",
				Content: []anthropicMessageContent{{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				}},
			})
		}
	}
	if len(messages) == 0 {
		messages = []anthropicMessage{{Role: "user", Content: ""}}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	out := anthropicRequest{
		Model:         req.Model,
		MaxTokens:     maxTokens,
		Messages:      messages,
		StopSequences: req.StopSequences,
		Stream:        req.Stream,
	}
	if len(systemParts) > 0 {
		out.System = strings.Join(systemParts, "\n\n")
	}
	if req.Temperature > 0 {
		temp := req.Temperature
		out.Temperature = &temp
	}
	if req.ToolChoice != nil {
		out.ToolChoice = &anthropicToolChoice{
			Type: req.ToolChoice.Mode,
			Name: req.ToolChoice.Name,
		}
	}

	for _, tool := range req.Tools {
		out.Tools = append(out.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: toSchemaMap(tool.Function.Parameters),
		})
	}

	return json.Marshal(out)
}

func convertOutboundContent(msg llm.Message) any {
	hasReasoning := msg.ReasoningSig != ""
	if len(msg.ToolCalls) == 0 && !hasReasoning {
		return msg.Content
	}
	var blocks []anthropicMessageContent
	if hasReasoning {
		blocks = append(blocks, anthropicMessageContent{
			Type:      "thinking",
			Thinking:  msg.Reasoning,
			Signature: msg.ReasoningSig,
		})
	}
	if msg.Content != "" {
		blocks = append(blocks, anthropicMessageContent{Type: "text", Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		blocks = append(blocks, anthropicMessageContent{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: toSchemaMap(tc.Function.Arguments),
		})
	}
	return blocks
}

func toSchemaMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

type anthropicResponseContent struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking,omitzero"`
	Signature string `json:"signature,omitzero"`
	Text      string `json:"text,omitzero"`
	Data      string `json:"data,omitzero"`

	ID    string         `json:"id,omitzero"`
	Name  string         `json:"name,omitzero"`
	Input map[string]any `json:"input,omitzero"`
}

type anthropicResponse struct {
	ID         string                     `json:"id"`
	Model      string                     `json:"model"`
	StopReason string                     `json:"stop_reason"`
	Content    []anthropicResponseContent `json:"content"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitzero"`
}

func (p *MiniMaxProvider) ConvertResponse(data []byte) (*llm.ChatResponse, error) {
	var raw anthropicResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &llm.Error{
			Kind:    llm.ErrorKindClient,
			Message: "failed to parse response",
			Cause:   err,
		}
	}
	if raw.Error != nil {
		return nil, &llm.Error{
			Kind:    llm.ErrorKindVendor,
			Message: raw.Error.Message,
		}
	}

	var contentText, reasoning strings.Builder
	var reasoningSig string
	var toolCalls []llm.ToolCall
	for _, block := range raw.Content {
		switch block.Type {
		case "thinking":
			reasoning.WriteString(block.Thinking)
			if block.Signature != "" {
				reasoningSig = block.Signature
			}
		case "redacted_thinking":
			reasoning.WriteString("[Reasoning redacted]")
			if block.Data != "" {
				reasoningSig = block.Data
			}
		case "text":
			contentText.WriteString(block.Text)
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	return &llm.ChatResponse{
		ID:      raw.ID,
		Model:   raw.Model,
		Object:  "message",
		Choices: []llm.Choice{{
			Index: 0,
			Message: llm.Message{
				Role:         "assistant",
				Content:      contentText.String(),
				Reasoning:    reasoning.String(),
				ReasoningSig: reasoningSig,
				ToolCalls:    toolCalls,
			},
			FinishReason: parseAnthropicStopReason(raw.StopReason),
		}},
		Usage: llm.Usage{
			PromptTokens:     raw.Usage.InputTokens,
			CompletionTokens: raw.Usage.OutputTokens,
			TotalTokens:      raw.Usage.InputTokens + raw.Usage.OutputTokens,
		},
	}, nil
}

func parseAnthropicStopReason(s string) llm.FinishReason {
	switch s {
	case "end_turn", "stop_sequence":
		return llm.FinishReasonStop
	case "max_tokens":
		return llm.FinishReasonLength
	case "tool_use":
		return llm.FinishReasonToolUse
	case "content_filter":
		return llm.FinishReasonContentFilter
	case "error":
		return llm.FinishReasonError
	default:
		return llm.FinishReasonUnknown
	}
}

type anthropicStreamEvent struct {
	Type string `json:"type"`

	Index        int `json:"index,omitzero"`
	ContentBlock *struct {
		Type string `json:"type"`
		Text string `json:"text,omitzero"`
		Thinking  string `json:"thinking,omitzero"`
		Signature string `json:"signature,omitzero"`
		Data      string `json:"data,omitzero"`
		ID    string         `json:"id,omitzero"`
		Name  string         `json:"name,omitzero"`
		Input map[string]any `json:"input,omitzero"`
	} `json:"content_block,omitzero"`

	Delta *struct {
		Type         string `json:"type,omitzero"`
		Text         string `json:"text,omitzero"`
		Thinking     string `json:"thinking,omitzero"`
		PartialJSON  string `json:"partial_json,omitzero"`
		StopReason   string `json:"stop_reason,omitzero"`
	} `json:"delta,omitzero"`

	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitzero"`
}

func (p *MiniMaxProvider) ConvertStreamChunk(data []byte) (*llm.StreamChunk, bool, error) {
	var ev anthropicStreamEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return nil, false, &llm.Error{
			Kind:    llm.ErrorKindClient,
			Message: "failed to parse stream event",
			Cause:   err,
		}
	}

	switch ev.Type {
	case "message_start", "content_block_stop", "ping":
		return &llm.StreamChunk{}, false, nil

	case "content_block_start":
		if ev.ContentBlock == nil {
			return &llm.StreamChunk{}, false, nil
		}
		switch ev.ContentBlock.Type {
		case "text":
			return &llm.StreamChunk{}, false, nil
		case "thinking":
			return &llm.StreamChunk{
				Choices: []llm.StreamChoice{{
					Index: ev.Index,
					Delta: llm.Message{ReasoningSig: ev.ContentBlock.Signature},
				}},
			}, false, nil
		case "redacted_thinking":
			return &llm.StreamChunk{
				Choices: []llm.StreamChoice{{
					Index: ev.Index,
					Delta: llm.Message{ReasoningSig: ev.ContentBlock.Data},
				}},
			}, false, nil
		case "tool_use":
			return &llm.StreamChunk{
				Choices: []llm.StreamChoice{{
					Index: ev.Index,
					Delta: llm.Message{
						ToolCalls: []llm.ToolCall{{
							ID:   ev.ContentBlock.ID,
							Type: "function",
							Function: llm.FunctionCall{
								Name: ev.ContentBlock.Name,
							},
						}},
					},
				}},
			}, false, nil
		}
		return &llm.StreamChunk{}, false, nil

	case "content_block_delta":
		if ev.Delta == nil {
			return &llm.StreamChunk{}, false, nil
		}
		chunk := &llm.StreamChunk{
			Choices: []llm.StreamChoice{{Index: ev.Index}},
		}
		switch ev.Delta.Type {
		case "text_delta":
			chunk.Choices[0].Delta.Content = ev.Delta.Text
		case "thinking_delta":
			chunk.Choices[0].Delta.Reasoning = ev.Delta.Thinking
		case "input_json_delta":
			chunk.Choices[0].Delta.ToolCalls = []llm.ToolCall{{
				Function: llm.FunctionCall{Arguments: ev.Delta.PartialJSON},
			}}
		}
		return chunk, false, nil

	case "message_delta":
		chunk := &llm.StreamChunk{}
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			chunk.Choices = append(chunk.Choices, llm.StreamChoice{
				FinishReason: parseAnthropicStopReason(ev.Delta.StopReason),
			})
		}
		if ev.Usage != nil {
			chunk.Usage = &llm.Usage{
				PromptTokens:     ev.Usage.InputTokens,
				CompletionTokens: ev.Usage.OutputTokens,
				TotalTokens:      ev.Usage.InputTokens + ev.Usage.OutputTokens,
			}
		}
		return chunk, false, nil

	case "message_stop":
		return nil, true, nil

	case "error":
		return nil, false, &llm.Error{
			Kind:    llm.ErrorKindVendor,
			Message: "anthropic stream error: " + string(data),
		}
	}

	return &llm.StreamChunk{}, false, nil
}
