package anthropic

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/vanpiyp/awp/internal/llm"
)

// ConvertAnthropicEvent parses one Anthropic SSE event payload into the
// legacy (*StreamChunk, done, error) shape the Provider interface expects.
// It is the per-event counterpart to ParseAnthropicSSE, which parses an
// entire SSE byte stream.
//
// The payload is the JSON body of a single event (no "event: ..." /
// "data: ..." framing). For "ping" and stop-only events it returns a
// zero-value chunk with done=false; for "message_stop" it returns
// (nil, true, nil).
func ConvertAnthropicEvent(payload []byte) (*llm.StreamChunk, bool, error) {
	var ev anthropicStreamEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		slog.Debug("anthropic: parse stream event failed", "bytes", len(payload), "err", err)
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
				PromptTokens:        ev.Usage.InputTokens,
				CompletionTokens:    ev.Usage.OutputTokens,
				TotalTokens:         ev.Usage.InputTokens + ev.Usage.OutputTokens,
				CacheReadTokens:     ev.Usage.CacheReadTokens,
				CacheCreationTokens: ev.Usage.CacheCreationTokens,
			}
		}
		return chunk, false, nil

	case "message_stop":
		return nil, true, nil

	case "error":
		return nil, false, &llm.Error{
			Kind:    llm.ErrorKindVendor,
			Message: fmt.Sprintf("anthropic stream error: %s", string(payload)),
		}
	}

	return &llm.StreamChunk{}, false, nil
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
		Type      string         `json:"type"`
		Text      string         `json:"text,omitzero"`
		Thinking  string         `json:"thinking,omitzero"`
		Signature string         `json:"signature,omitzero"`
		Data      string         `json:"data,omitzero"`
		ID        string         `json:"id,omitzero"`
		Name      string         `json:"name,omitzero"`
		Input     map[string]any `json:"input,omitzero"`
	} `json:"content_block,omitzero"`

	Delta *struct {
		Type        string `json:"type,omitzero"`
		Text        string `json:"text,omitzero"`
		Thinking    string `json:"thinking,omitzero"`
		PartialJSON string `json:"partial_json,omitzero"`
		StopReason  string `json:"stop_reason,omitzero"`
	} `json:"delta,omitzero"`

	Usage *struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage,omitzero"`
}
