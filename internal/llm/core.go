package llm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vanpiyp/awp/internal/llm/protocol"
)

type Provider interface {
	Name() string

	BaseURL() string

	Path() string

	Headers() map[string]string

	ConvertRequest(req *ChatRequest) ([]byte, error)

	ConvertResponse(data []byte) (*StreamChunk, bool, error)

	// RecoverRequest inspects `err` from a failed first attempt and,
	// when the error is recoverable (provider-specific 400 with a
	// known field-rejection signature), mutates `req` so the next
	// attempt emits a compatible wire shape and returns true. When
	// the error is not recoverable, it leaves `req` untouched and
	// returns false. Called by Core.StreamChat at most once per
	// turn, only before any chunk has been forwarded. Providers with
	// no recovery surface (e.g. MiniMax) implement this as a no-op.
	RecoverRequest(req *ChatRequest, err error) bool

	Models() []Model

	SupportsCacheControl(model string) bool

	ContextWindow(model string) int

	MaxOutputTokens(model string) int

	AvailableReasoningEfforts(model string) []string

	AvailableServiceTiers(model string) []string

	BetaHeaders(model string) []string

	ModelCapabilities(model string) ModelCapabilities

	SupportsNativeCompact(model string) bool

	CompleteSplit(systemPrompt string) ([]ContentBlock, error)
}

type core struct {
	provider Provider
	protocol protocol.Protocol
}

type Core interface {
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan LegacyStreamEvent, error)
}

func NewCore(provider Provider, proto protocol.Protocol) Core {
	return &core{
		provider: provider,
		protocol: proto,
	}
}

func (c *core) StreamChat(ctx context.Context, req *ChatRequest) (<-chan LegacyStreamEvent, error) {
	if err := c.validateRequest(req); err != nil {
		return nil, err
	}
	ifaceLogger().Info("llm: request", "model", req.Model, "messages", len(req.Messages), "tools", len(req.Tools))
	for i, m := range req.Messages {
		ifaceLogger().Info("llm: request msg", "i", i, "role", m.Role, "content", m.Content, "reasoning_len", len(m.Reasoning), "tool_calls", len(m.ToolCalls))
	}
	for i, t := range req.Tools {
		ifaceLogger().Info("llm: request tool", "i", i, "name", t.Function.Name)
	}
	slog.Debug("llm: stream start", "model", req.Model, "messages", len(req.Messages), "tools", len(req.Tools))

	rawChan, _, err := c.startStream(ctx, req)
	if err != nil {
		if !c.provider.RecoverRequest(req, err) {
			return nil, err
		}
		slog.Warn("llm: provider self-heal, retrying without rejected fields", "model", req.Model, "err", err)
		var retryErr error
		rawChan, _, retryErr = c.startStream(ctx, req)
		if retryErr != nil {
			return nil, retryErr
		}
	}

	events := make(chan LegacyStreamEvent, 32)
	go func() {
		defer close(events)
		c.streamOnce(ctx, rawChan, events)
	}()
	return events, nil
}

// startStream marshals the request and opens a transport-level
// stream. Returns the raw chunk channel and the built protocol
// request (so callers can correlate retries) plus any synchronous
// transport error from the first attempt. Does NOT surface
// chunk-level errors — those arrive via the returned channel.
func (c *core) startStream(ctx context.Context, req *ChatRequest) (<-chan protocol.StreamItem, *protocol.Request, error) {
	streamReq := *req
	streamReq.Stream = true

	body, err := c.provider.ConvertRequest(&streamReq)
	if err != nil {
		return nil, nil, err
	}

	protoReq := &protocol.Request{
		URL:     c.provider.BaseURL() + c.provider.Path(),
		Method:  "POST",
		Headers: c.headersFor(req),
		Body:    body,
	}

	rawChan, err := c.protocol.Stream(ctx, protoReq)
	if err != nil {
		return nil, protoReq, err
	}
	return rawChan, protoReq, nil
}

// streamOnce runs one streaming attempt and forwards every
// transport/parse error to the consumer. Self-heal happens at the
// synchronous transport level (StreamChat intercepts protocol.Stream
// errors before returning the channel); chunk-level errors after the
// response has begun are surfaced verbatim.
func (c *core) streamOnce(ctx context.Context, rawChan <-chan protocol.StreamItem, events chan LegacyStreamEvent) error {

	accumulators := make(map[int]*toolCallAccum)
	var totalContent, totalReasoning int
	var lastUsage *Usage
	var eventCount int
	defer func() {
		if lastUsage != nil {
			ifaceLogger().Info("llm: stream done", "events", eventCount, "content_chars", totalContent, "reasoning_chars", totalReasoning, "prompt_tokens", lastUsage.PromptTokens, "completion_tokens", lastUsage.CompletionTokens)
		} else {
			ifaceLogger().Info("llm: stream done", "events", eventCount, "content_chars", totalContent, "reasoning_chars", totalReasoning)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case item, ok := <-rawChan:
			if !ok {
				return nil
			}
			if item.Err != nil {
				ifaceLogger().Info("llm: transport err", "err", item.Err)
				select {
				case events <- LegacyStreamEvent{Err: item.Err}:
				case <-ctx.Done():
				}
				return nil
			}
			chunk, done, err := c.provider.ConvertResponse(item.Data)
			if err != nil {
				ifaceLogger().Info("llm: parse err", "err", err)
				select {
				case events <- LegacyStreamEvent{Err: err}:
				case <-ctx.Done():
				}
				return nil
			}

			if chunk != nil {
				eventCount++
				for _, choice := range chunk.Choices {
					totalContent += len(choice.Delta.Content)
					totalReasoning += len(choice.Delta.Reasoning)
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
				if chunk.Usage != nil {
					lastUsage = chunk.Usage
					select {
					case events <- LegacyStreamEvent{Chunk: &StreamChunk{Usage: chunk.Usage}}:
					case <-ctx.Done():
						return nil
					}
				}
				for _, choice := range chunk.Choices {
					if choice.Delta.Content == "" && choice.Delta.Reasoning == "" && choice.FinishReason == 0 {
						continue
					}
					select {
					case events <- LegacyStreamEvent{Chunk: &StreamChunk{
						Choices: []StreamChoice{{
							Index:        choice.Index,
							Delta:        Message{Content: choice.Delta.Content, Reasoning: choice.Delta.Reasoning},
							FinishReason: choice.FinishReason,
						}},
					}}:
					case <-ctx.Done():
						return nil
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
					case events <- LegacyStreamEvent{Chunk: final}:
					case <-ctx.Done():
						return nil
					}
				}
				return nil
			}
		}
	}
}

func (c *core) validateRequest(req *ChatRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}
	for i, msg := range req.Messages {
		switch msg.Role {
		case "system", "user", "assistant", "tool":
		default:
			return fmt.Errorf("messages[%d]: invalid role %q", i, msg.Role)
		}
	}
	if req.ToolChoice != nil {
		if err := req.ToolChoice.Validate(); err != nil {
			return err
		}
	}
	if len(req.Tools) > 0 {
		if err := validateTools(req.Tools); err != nil {
			return err
		}
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

func (c *core) headersFor(req *ChatRequest) map[string]string {
	h := c.provider.Headers()
	betas := c.provider.BetaHeaders(req.Model)
	if len(betas) == 0 {
		return h
	}
	merged := make(map[string]string, len(h)+1)
	for k, v := range h {
		merged[k] = v
	}
	merged["anthropic-beta"] = strings.Join(betas, ",")
	return merged
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
