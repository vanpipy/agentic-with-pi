package agentcore

import (
	"context"
	"fmt"
	"strings"

	"github.com/vanpiyp/awp/internal/agent-core/cache"
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

// =====================================================================
// ReActStrategy implementation
// =====================================================================

type ReActStrategy struct {
	core                   llm.Core
	model                  llm.Model
	toolDefsGetter         func() []llm.ToolDef
	supportsCacheControl   func(model string) bool
	maxToolsPerTurn        int
	repeatedToolErrorLimit int
}

func NewReActStrategy(core llm.Core, model llm.Model, toolDefsGetter func() []llm.ToolDef, supportsCacheControl func(model string) bool) *ReActStrategy {
	if supportsCacheControl == nil {
		supportsCacheControl = func(string) bool { return false }
	}
	return &ReActStrategy{
		core:                   core,
		model:                  model,
		toolDefsGetter:         toolDefsGetter,
		supportsCacheControl:   supportsCacheControl,
		maxToolsPerTurn:        6,
		repeatedToolErrorLimit: 3,
	}
}

func (r *ReActStrategy) Name() string { return "react" }

func (r *ReActStrategy) Step(ctx context.Context, msgs []llm.Message, emit func(context.Context, Event) bool) (Step, error) {
	if !emit(ctx, Event{Category: EventThoughtStart}) {
		return Step{}, ctx.Err()
	}
	msgs = cache.InjectCacheControl(msgs, r.supportsCacheControl, r.model.ID)
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
	tp := stream.NewTypedProcessor()
	streamDone := false
	for ev := range raw {
		if ctx.Err() != nil {
			return Step{}, ctx.Err()
		}
		if !r.processStreamEventTyped(ctx, ev, tp, emit) {
			streamDone = true
			break
		}
	}
	if streamDone {
		emit(ctx, Event{Category: EventThoughtEnd})
		return Step{Kind: StepContinue}, nil
	}
	result := tp.Result()
	if !emit(ctx, Event{
		Category:  EventThoughtEnd,
		Content:   result.Content,
		Reasoning: result.Reasoning,
		ToolCalls: result.ToolCalls,
		Usage:     result.Usage,
	}) {
		return Step{}, ctx.Err()
	}
	switch {
	case result.FinishReason == llm.FinishReasonLength && len(result.ToolCalls) > 0:
		emit(ctx, Event{Category: EventError, ToolError: "response truncated mid tool call, refusing"})
		return Step{}, nil
	case result.FinishReason == llm.FinishReasonLength && result.Content == "":
		emit(ctx, Event{Category: EventError, ToolError: "response truncated with no content"})
		return Step{}, nil
	case result.FinishReason == llm.FinishReasonToolUse && len(result.ToolCalls) > 0 && allToolCallsEmpty(result.ToolCalls):
		emit(ctx, Event{Category: EventError, ToolError: "empty tool calls, refusing"})
		return Step{}, nil
	case result.FinishReason == llm.FinishReasonToolUse && len(result.ToolCalls) > 0:
		return Step{
			Kind:         StepContinue,
			Content:      result.Content,
			Reasoning:    result.Reasoning,
			ReasoningSig: result.ReasoningSig,
			ToolCalls:    result.ToolCalls,
			Usage:        result.Usage,
			FinishReason: result.FinishReason.String(),
		}, nil
	case result.FinishReason == llm.FinishReasonToolUse && len(result.ToolCalls) == 0:
		emit(ctx, Event{Category: EventError, ToolError: "tool_use finish reason with no tool calls"})
		return Step{}, nil
	case (result.FinishReason == llm.FinishReasonStop || (result.FinishReason == llm.FinishReasonLength && result.Content != "")) && len(result.ToolCalls) == 0:
		emit(ctx, Event{Category: EventFinalAnswer, Content: result.Content, Usage: result.Usage})
		return Step{
			Kind:         StepFinal,
			Content:      result.Content,
			Usage:        result.Usage,
			FinishReason: result.FinishReason.String(),
		}, nil
	}
	return Step{}, fmt.Errorf("unreachable: finishReason=%v toolCalls=%d", result.FinishReason, len(result.ToolCalls))
}

func (r *ReActStrategy) processStreamEventTyped(ctx context.Context, ev llm.StreamEvent, tp *stream.TypedProcessor, emit func(context.Context, Event) bool) bool {
	continue_, _, emits := tp.ProcessEvent(ctx, ev)
	for _, e := range emits {
		if !emitTypedEmit(ctx, e, emit) {
			return false
		}
	}
	return continue_
}

func emitTypedEmit(ctx context.Context, e stream.EmitEvent, emit func(context.Context, Event) bool) bool {
	switch e.Kind {
	case stream.EmitKindObserve:
		var ev Event
		if e.Reasoning != "" {
			ev = Event{Category: EventThoughtChunk, Reasoning: e.Reasoning}
		} else {
			ev = Event{Category: EventThoughtChunk, Content: e.Content}
		}
		return emit(ctx, ev)
	case stream.EmitKindTool:
		return emit(ctx, Event{Category: EventTool, ToolName: e.ToolName, ToolArgs: e.ToolArgs, ToolIntent: e.ToolIntent})
	case stream.EmitKindError:
		return emit(ctx, Event{Category: EventError, ToolError: e.Content})
	default:
		return true
	}
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
