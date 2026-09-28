package agentcore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
)

// =====================================================================
// Strategy interface + Step result
// =====================================================================

type Strategy interface {
	Name() string
	Step(ctx context.Context, msgs []llm.Message, emit func(context.Context, Event) bool) (Step, error)
	ShouldAbort(msgs []llm.Message, lastFailedToolError string) error
}

type StepKind int

const (
	StepContinue StepKind = iota
	StepFinal
)

type Step struct {
	Kind         StepKind
	Content      string
	Reasoning    string
	ReasoningSig string
	ToolCalls    []llm.ToolCall
	Usage        *llm.Usage
	FinishReason string
}

type turnResult struct {
	finishReason llm.FinishReason
	content      string
	reasoning    string
	reasoningSig string
	toolCalls    []llm.ToolCall
	aborted      error
	usage        *llm.Usage
}

func (r turnResult) toAssistantMessage() llm.Message {
	return llm.Message{Role: "assistant", Content: r.content, Reasoning: r.reasoning, ReasoningSig: r.reasoningSig, ToolCalls: r.toolCalls}
}

// =====================================================================
// ReActStrategy implementation
// =====================================================================

type ReActStrategy struct {
	core                   llm.Core
	model                  llm.Model
	toolDefsGetter         func() []llm.ToolDef
	maxToolsPerTurn        int
	repeatedToolErrorLimit int
}

func NewReActStrategy(core llm.Core, model llm.Model, toolDefsGetter func() []llm.ToolDef) *ReActStrategy {
	return &ReActStrategy{
		core:                   core,
		model:                  model,
		toolDefsGetter:         toolDefsGetter,
		maxToolsPerTurn:        6,
		repeatedToolErrorLimit: 3,
	}
}

func (r *ReActStrategy) Name() string { return "react" }

func (r *ReActStrategy) Step(ctx context.Context, msgs []llm.Message, emit func(context.Context, Event) bool) (Step, error) {
	if !emit(ctx, Event{Category: EventThoughtStart}) {
		return Step{}, ctx.Err()
	}
	req := &llm.ChatRequest{Model: r.model.ID, Messages: msgs}
	if r.toolDefsGetter != nil {
		req.Tools = r.toolDefsGetter()
	}
	raw, err := r.core.StreamChat(ctx, req)
	if err != nil {
		if isCtxErr(err) {
			return Step{}, err
		}
		emit(ctx, Event{Category: EventError, ToolError: err.Error()})
		emit(ctx, Event{Category: EventThoughtEnd})
		return Step{}, nil
	}
	var result turnResult
	var contentBuf, reasoningBuf strings.Builder
	contentBuf.Grow(2048)
	reasoningBuf.Grow(2048)
	for ev := range raw {
		if ctx.Err() != nil {
			return Step{}, ctx.Err()
		}
		if !r.processStreamEvent(ctx, ev, &result, &contentBuf, &reasoningBuf, emit) {
			return Step{}, ctx.Err()
		}
	}
	result.content = contentBuf.String()
	result.reasoning = reasoningBuf.String()
	if !emit(ctx, Event{
		Category:  EventThoughtEnd,
		Content:   result.content,
		Reasoning: result.reasoning,
		ToolCalls: result.toolCalls,
		Usage:     result.usage,
	}) {
		return Step{}, ctx.Err()
	}
	switch {
	case result.finishReason == llm.FinishReasonLength && len(result.toolCalls) > 0:
		emit(ctx, Event{Category: EventError, ToolError: "response truncated mid tool call, refusing"})
		return Step{}, nil
	case result.finishReason == llm.FinishReasonLength && result.content == "":
		emit(ctx, Event{Category: EventError, ToolError: "response truncated with no content"})
		return Step{}, nil
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) > 0 && allToolCallsEmpty(result.toolCalls):
		emit(ctx, Event{Category: EventError, ToolError: "empty tool calls, refusing"})
		return Step{}, nil
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) > 0:
		return Step{
			Kind:         StepContinue,
			Content:      result.content,
			Reasoning:    result.reasoning,
			ReasoningSig: result.reasoningSig,
			ToolCalls:    result.toolCalls,
			Usage:        result.usage,
			FinishReason: result.finishReason.String(),
		}, nil
	case result.finishReason == llm.FinishReasonToolUse && len(result.toolCalls) == 0:
		emit(ctx, Event{Category: EventError, ToolError: "tool_use finish reason with no tool calls"})
		return Step{}, nil
	case (result.finishReason == llm.FinishReasonStop || (result.finishReason == llm.FinishReasonLength && result.content != "")) && len(result.toolCalls) == 0:
		emit(ctx, Event{Category: EventFinalAnswer, Content: result.content, Usage: result.usage})
		return Step{
			Kind:         StepFinal,
			Content:      result.content,
			Usage:        result.usage,
			FinishReason: result.finishReason.String(),
		}, nil
	}
	return Step{}, fmt.Errorf("unreachable: finishReason=%v toolCalls=%d", result.finishReason, len(result.toolCalls))
}

func (r *ReActStrategy) processStreamEvent(ctx context.Context, ev llm.StreamEvent, result *turnResult, contentBuf, reasoningBuf *strings.Builder, emit func(context.Context, Event) bool) bool {
	if ev.Rollback {
		contentBuf.Reset()
		reasoningBuf.Reset()
		result.content = ""
		result.reasoning = ""
		result.reasoningSig = ""
		result.toolCalls = nil
		slog.Debug("agent: stream rollback — discarded partial output")
		return true
	}
	if ev.Err != nil {
		if isCtxErr(ev.Err) {
			return false
		}
		emit(ctx, Event{Category: EventError, ToolError: ev.Err.Error()})
		emit(ctx, Event{Category: EventThoughtEnd})
		return false
	}
	if ev.Chunk == nil {
		return true
	}
	for _, c := range ev.Chunk.Choices {
		if c.Delta.Reasoning != "" {
			reasoningBuf.WriteString(c.Delta.Reasoning)
			if !emit(ctx, Event{Category: EventThoughtChunk, Reasoning: c.Delta.Reasoning}) {
				return false
			}
		}
		if c.Delta.ReasoningSig != "" {
			result.reasoningSig = c.Delta.ReasoningSig
		}
		if c.Delta.Content != "" {
			contentBuf.WriteString(c.Delta.Content)
			if !emit(ctx, Event{Category: EventThoughtChunk, Content: c.Delta.Content}) {
				return false
			}
		}
		if c.FinishReason != llm.FinishReasonUnknown {
			result.finishReason = c.FinishReason
		}
		if len(c.Delta.ToolCalls) > 0 {
			for _, tc := range c.Delta.ToolCalls {
				if tc.ID != "" {
					idx := -1
					for i := range result.toolCalls {
						if result.toolCalls[i].ID == tc.ID {
							idx = i
							break
						}
					}
					if idx >= 0 {
						mergeToolCallDelta(&result.toolCalls[idx], tc)
					} else {
						result.toolCalls = append(result.toolCalls, tc)
					}
				} else if n := len(result.toolCalls); n > 0 {
					mergeToolCallDelta(&result.toolCalls[n-1], tc)
				} else {
					result.toolCalls = append(result.toolCalls, tc)
				}
			}
		}
	}
	if ev.Chunk.Usage != nil {
		result.usage = ev.Chunk.Usage
	}
	return true
}

func (r *ReActStrategy) ShouldAbort(msgs []llm.Message, lastFailedToolError string) error {
	const maxConsecutiveRepeats = 3
	recentCalls := []string{}
	for _, m := range msgs {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			sig := toolCallSignature(m.ToolCalls)
			recentCalls = append(recentCalls, sig)
			if len(recentCalls) > maxConsecutiveRepeats {
				recentCalls = recentCalls[1:]
			}
			if len(recentCalls) >= maxConsecutiveRepeats && allEqual(recentCalls) {
				return fmt.Errorf("tool calls repeated %d times, aborting", maxConsecutiveRepeats+1)
			}
		}
	}
	if lastFailedToolError == "" {
		return nil
	}
	normalised := stream.NormalizeToolError(lastFailedToolError)
	consecutiveTurns := 0
	i := len(msgs) - 1
	for i >= 0 {
		m := msgs[i]
		if !(m.Role == "tool" && stream.NormalizeToolError(m.Content) == normalised) {
			break
		}
		consecutiveTurns++
		i--
		for i >= 0 && msgs[i].Role == "tool" {
			i--
		}
		for i >= 0 && msgs[i].Role != "assistant" {
			i--
		}
		i--
	}
	if consecutiveTurns >= r.repeatedToolErrorLimit {
		return fmt.Errorf("aborting: tool failed %d times in a row with the same error: %s. Stop and report to the user instead of retrying.", consecutiveTurns, lastFailedToolError)
	}
	return nil
}

// =====================================================================
// helpers
// =====================================================================

func mergeToolCallDelta(existing *llm.ToolCall, delta llm.ToolCall) {
	if delta.Function.Arguments != "" {
		if isPlaceholderToolArgs(existing.Function.Arguments) {
			existing.Function.Arguments = delta.Function.Arguments
		} else {
			existing.Function.Arguments += delta.Function.Arguments
		}
	}
	if delta.Function.Name != "" && existing.Function.Name == "" {
		existing.Function.Name = delta.Function.Name
	}
}

func isPlaceholderToolArgs(s string) bool {
	trimmed := strings.TrimSpace(s)
	return trimmed == "" || trimmed == "{}"
}

func allToolCallsEmpty(calls []llm.ToolCall) bool {
	if len(calls) == 0 {
		return true
	}
	for _, c := range calls {
		trimmed := strings.TrimSpace(c.Function.Arguments)
		if trimmed != "" {
			return false
		}
	}
	return true
}

func toolCallSignature(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	parts := make([]string, len(calls))
	for i, c := range calls {
		parts[i] = c.Function.Name + ":" + c.Function.Arguments
	}
	return strings.Join(parts, "|")
}

func allEqual(sigs []string) bool {
	if len(sigs) == 0 {
		return false
	}
	first := sigs[0]
	for _, s := range sigs[1:] {
		if s != first {
			return false
		}
	}
	return true
}
