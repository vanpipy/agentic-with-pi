package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/vanpiyp/awp/internal/llm/protocol"
)

type Provider interface {
	Name() string

	BaseURL() string

	Path() string

	Headers() map[string]string

	ConvertRequest(req *ChatRequest) ([]byte, error)

	ConvertResponse(data []byte) (*ChatResponse, error)

	ConvertStreamChunk(data []byte) (*StreamChunk, bool, error)

	Models() []Model
}

type core struct {
	provider Provider
	protocol protocol.Protocol
}

type Core interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
}

func NewCore(provider Provider, proto protocol.Protocol) Core {
	return &core{
		provider: provider,
		protocol: proto,
	}
}

func (c *core) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if err := c.validateRequest(req); err != nil {
		return nil, err
	}

	body, err := c.provider.ConvertRequest(req)
	if err != nil {
		return nil, err
	}

	protoReq := &protocol.Request{
		URL:     c.provider.BaseURL() + c.provider.Path(),
		Method:  "POST",
		Headers: c.provider.Headers(),
		Body:    body,
	}

	data, err := c.protocol.Send(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	resp, err := c.provider.ConvertResponse(data)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *core) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	if err := c.validateRequest(req); err != nil {
		return nil, err
	}

	streamReq := *req
	streamReq.Stream = true

	body, err := c.provider.ConvertRequest(&streamReq)
	if err != nil {
		return nil, err
	}

	protoReq := &protocol.Request{
		URL:     c.provider.BaseURL() + c.provider.Path(),
		Method:  "POST",
		Headers: c.provider.Headers(),
		Body:    body,
	}

	rawChan, err := c.protocol.Stream(ctx, protoReq)
	if err != nil {
		return nil, err
	}

	events := make(chan StreamEvent, 32)
	go func() {
		defer close(events)
		accumulators := make(map[int]*toolCallAccum)
		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-rawChan:
				if !ok {
					return
				}
				if item.Err != nil {
					select {
					case events <- StreamEvent{Err: item.Err}:
					case <-ctx.Done():
					}
					return
				}
				chunk, done, err := c.provider.ConvertStreamChunk(item.Data)
				if err != nil {
					select {
					case events <- StreamEvent{Err: err}:
					case <-ctx.Done():
					}
					return
				}

				if chunk != nil {
					for _, choice := range chunk.Choices {
						for _, tc := range choice.Delta.ToolCalls {
							acc, exists := accumulators[choice.Index]
							if !exists {
								acc = &toolCallAccum{}
								accumulators[choice.Index] = acc
							}
							if tc.ID != "" {
								acc.ID = tc.ID
							}
							if tc.Type != "" {
								acc.Type = tc.Type
							}
							if tc.Function.Name != "" {
								acc.Name = tc.Function.Name
							}
							acc.Args.WriteString(tc.Function.Arguments)
						}
					}
					for _, choice := range chunk.Choices {
						if choice.Delta.Content == "" && choice.Delta.Reasoning == "" {
							continue
						}
						select {
						case events <- StreamEvent{Chunk: &StreamChunk{
							Choices: []StreamChoice{{
								Index:    choice.Index,
								Delta:    Message{Content: choice.Delta.Content, Reasoning: choice.Delta.Reasoning},
								FinishReason: choice.FinishReason,
							}},
						}}:
						case <-ctx.Done():
							return
						}
					}
				}

				if done {
					assembled := assembleToolCalls(accumulators)
					if len(assembled) > 0 {
						final := &StreamChunk{
							Choices: []StreamChoice{{
								Index: 0,
								Delta: Message{ToolCalls: assembled},
							}},
						}
						select {
						case events <- StreamEvent{Chunk: final}:
						case <-ctx.Done():
							return
						}
					}
					return
				}
			}
		}
	}()

	return events, nil
}

func (c *core) validateRequest(req *ChatRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(req.Tools) > 0 {
		for _, m := range c.provider.Models() {
			if m.ID == req.Model {
				if !m.SupportsTool {
					return fmt.Errorf("model %q does not support tool use", req.Model)
				}
				break
			}
		}
	}
	return nil
}

type toolCallAccum struct {
	ID   string
	Type string
	Name string
	Args strings.Builder
}

func assembleToolCalls(m map[int]*toolCallAccum) []ToolCall {
	if len(m) == 0 {
		return nil
	}
	maxIdx := -1
	for idx := range m {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	out := make([]ToolCall, 0, len(m))
	for i := 0; i <= maxIdx; i++ {
		acc, ok := m[i]
		if !ok {
			continue
		}
		tcType := acc.Type
		if tcType == "" {
			tcType = "function"
		}
		out = append(out, ToolCall{
			ID:   acc.ID,
			Type: tcType,
			Function: FunctionCall{
				Name:      acc.Name,
				Arguments: acc.Args.String(),
			},
		})
	}
	return out
}
