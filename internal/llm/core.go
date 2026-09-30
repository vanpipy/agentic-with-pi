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

	// ConvertResponse decodes one protocol-level wire chunk into the
	// wire-shape StreamChunk plus a done flag and a parse error. It
	// is an internal helper used by Core to bridge the transport
	// protocol to the sealed llm.StreamEvent channel; callers do
	// not invoke it directly.
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

	NativeCompactMode(model string) string

	NativeCompactThreshold(model string) int

	NativeCompactCapabilities(model string) NativeCompactionCapabilities

	NativeCompact(ctx context.Context, model string, msgs []Message, summaryText, encryptedContent string) (NativeCompactionResult, error)

	// CompleteSplit divides a system prompt into a cacheable static prefix
	// and a dynamic suffix. When the target model supports prompt-cache, the
	// static prefix carries CacheEphemeral; otherwise the blocks are plain
	// ContentText. The split point is the first newline at or after the byte
	// midpoint. Empty prompt returns nil.
	CompleteSplit(systemPrompt string, model string) []ContentBlock
}

type core struct {
	provider Provider
	protocol protocol.Protocol
}

// Core is the consumer-facing LLM streaming boundary. The returned
// channel carries sealed llm.StreamEvent values that map 1:1 to the
// runtime behavior of the strategy layer (and any other sealed-event
// consumer). Channels are buffered; producers close on terminal
// event or context cancellation.
type Core interface {
	StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
	CompleteSplit(systemPrompt string, model string) []ContentBlock
}

func NewCore(provider Provider, proto protocol.Protocol) Core {
	return &core{
		provider: provider,
		protocol: proto,
	}
}

func (c *core) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
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

	events := make(chan StreamEvent, 32)
	go func() {
		defer close(events)
		c.streamOnce(ctx, rawChan, events)
	}()
	return events, nil
}

// CompleteSplit delegates to the wrapped Provider. It exists on Core so
// agent-core can call the splitter without holding a direct Provider
// reference (Core is the boundary it actually owns).
func (c *core) CompleteSplit(systemPrompt string, model string) []ContentBlock {
	return c.provider.CompleteSplit(systemPrompt, model)
}

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

// streamOnce runs one streaming attempt and projects every parsed
// StreamChunk into one or more sealed StreamEvent values on `events`.
// Transport-level errors arrive as EventErr; the channel closes when
// the producer signals done or ctx cancels.
func (c *core) streamOnce(ctx context.Context, rawChan <-chan protocol.StreamItem, events chan<- StreamEvent) {
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
			return
		case item, ok := <-rawChan:
			if !ok {
				return
			}
			if item.Err != nil {
				ifaceLogger().Info("llm: transport err", "err", item.Err)
				select {
				case events <- EventErr{Err: item.Err}:
				case <-ctx.Done():
				}
				return
			}
			chunk, done, err := c.provider.ConvertResponse(item.Data)
			if err != nil {
				ifaceLogger().Info("llm: parse err", "err", err)
				select {
				case events <- EventErr{Err: err}:
				case <-ctx.Done():
				}
				return
			}
			if chunk == nil && !done {
				continue
			}
			eventCount++
			if chunk != nil {
				if chunk.Usage != nil {
					lastUsage = chunk.Usage
					select {
					case events <- EventUsage{
						InputTokens:         chunk.Usage.PromptTokens,
						OutputTokens:        chunk.Usage.CompletionTokens,
						CacheReadTokens:     chunk.Usage.CacheReadTokens,
						CacheCreationTokens: chunk.Usage.CacheCreationTokens,
					}:
					case <-ctx.Done():
						return
					}
				}
				for _, choice := range chunk.Choices {
					totalContent += len(choice.Delta.Content)
					totalReasoning += len(choice.Delta.Reasoning)
					if choice.Delta.Reasoning != "" {
						select {
						case events <- EventThinkingDelta{Text: choice.Delta.Reasoning}:
						case <-ctx.Done():
							return
						}
					}
					if choice.Delta.ReasoningSig != "" {
						select {
						case events <- EventThinkingSignature{Signature: choice.Delta.ReasoningSig}:
						case <-ctx.Done():
							return
						}
					}
					if choice.Delta.Content != "" {
						select {
						case events <- EventTextDelta{Text: choice.Delta.Content}:
						case <-ctx.Done():
							return
						}
					}
					for _, tc := range choice.Delta.ToolCalls {
						acc, exists := accumulators[choice.Index]
						if !exists {
							acc = &toolCallAccum{}
							accumulators[choice.Index] = acc
						}
						if tc.ID != "" && tc.ID != acc.ID {
							if acc.ID != "" {
								select {
								case events <- EventToolEnd{ID: acc.ID}:
								case <-ctx.Done():
									return
								}
							}
							acc.ID = tc.ID
							acc.Type = tc.Type
							acc.Name = tc.Function.Name
							acc.SeenArgs = false
							select {
							case events <- EventToolStart{ID: acc.ID, Name: acc.Name}:
							case <-ctx.Done():
								return
							}
						}
						if tc.Type != "" {
							acc.Type = tc.Type
						}
						if tc.Function.Name != "" {
							acc.Name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							acc.SeenArgs = true
							select {
							case events <- EventToolDelta{ID: acc.ID, JSON: tc.Function.Arguments}:
							case <-ctx.Done():
								return
							}
						}
					}
					if choice.FinishReason != FinishReasonUnknown {
						select {
						case events <- EventFinish{Reason: choice.FinishReason}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
			if done {
				for _, acc := range accumulators {
					if acc.ID == "" {
						continue
					}
					if acc.SeenArgs {
						select {
						case events <- EventToolEnd{ID: acc.ID}:
						case <-ctx.Done():
							return
						}
					}
				}
				return
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
	ID        string
	Type      string
	Name      string
	SeenArgs  bool
	ArgsCount int
}
