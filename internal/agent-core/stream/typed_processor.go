package stream

import (
	"context"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

type TypedProcessor struct {
	contentBuf          strings.Builder
	reasoningBuf        strings.Builder
	toolCalls           []llm.ToolCall
	reasoningSig        string
	finishReason        llm.FinishReason
	inputTokens         int
	outputTokens        int
	cacheReadTokens     int
	cacheCreationTokens int
}

func NewTypedProcessor() *TypedProcessor {
	return &TypedProcessor{}
}

func (p *TypedProcessor) ProcessEvent(ctx context.Context, ev llm.StreamEvent) (continue_, stepDone bool, emits []EmitEvent) {
	continue_ = true
	switch e := ev.(type) {
	case llm.EventTextDelta:
		p.contentBuf.WriteString(e.Text)
		emits = append(emits, EmitEvent{Kind: EmitKindObserve, Content: e.Text})
	case llm.EventThinkingStart:
	case llm.EventThinkingDelta:
		p.reasoningBuf.WriteString(e.Text)
		emits = append(emits, EmitEvent{Kind: EmitKindObserve, Reasoning: e.Text})
	case llm.EventThinkingSignature:
		p.reasoningSig = e.Signature
	case llm.EventThinkingEnd:
	case llm.EventToolStart:
		p.toolCalls = append(p.toolCalls, llm.ToolCall{
			ID:       e.ID,
			Type:     "function",
			Function: llm.FunctionCall{Name: e.Name, Arguments: ""},
		})
		emits = append(emits, EmitEvent{Kind: EmitKindTool, ToolName: e.Name})
	case llm.EventToolDelta:
		for i := range p.toolCalls {
			if p.toolCalls[i].ID == e.ID {
				p.toolCalls[i].Function.Arguments += e.JSON
				break
			}
		}
	case llm.EventToolEnd:
	case llm.EventToolUseSignature:
		for i := range p.toolCalls {
			if p.toolCalls[i].ID == e.ID {
				break
			}
		}
	case llm.EventUsage:
		p.inputTokens = e.InputTokens
		p.outputTokens = e.OutputTokens
		p.cacheReadTokens = e.CacheReadTokens
		p.cacheCreationTokens = e.CacheCreationTokens
	case llm.EventFinish:
		p.finishReason = e.Reason
		stepDone = true
	case llm.EventErr:
		if ctx.Err() != nil {
			continue_ = false
			return
		}
		emits = append(emits, EmitEvent{Kind: EmitKindError, Content: e.Err.Error()})
		continue_ = false
	case llm.EventRetryRollback:
		p.contentBuf.Reset()
		p.reasoningBuf.Reset()
		p.toolCalls = nil
		p.reasoningSig = ""
	case llm.EventSessionID:
	case llm.EventCompaction:
	}
	return
}

func (p *TypedProcessor) Content() string {
	return p.contentBuf.String()
}

func (p *TypedProcessor) Reasoning() string {
	return p.reasoningBuf.String()
}

func (p *TypedProcessor) ReasoningSig() string {
	return p.reasoningSig
}

func (p *TypedProcessor) ToolCalls() []llm.ToolCall {
	return p.toolCalls
}

func (p *TypedProcessor) FinishReason() llm.FinishReason {
	return p.finishReason
}

func (p *TypedProcessor) Usage() *llm.Usage {
	if p.inputTokens == 0 && p.outputTokens == 0 && p.cacheReadTokens == 0 && p.cacheCreationTokens == 0 {
		return nil
	}
	return &llm.Usage{
		PromptTokens:     p.inputTokens,
		CompletionTokens: p.outputTokens,
		TotalTokens:      p.inputTokens + p.outputTokens + p.cacheReadTokens + p.cacheCreationTokens,
	}
}

func (p *TypedProcessor) HasUsage() bool {
	return p.inputTokens != 0 || p.outputTokens != 0 || p.cacheReadTokens != 0 || p.cacheCreationTokens != 0
}

func (p *TypedProcessor) InputTokens() int         { return p.inputTokens }
func (p *TypedProcessor) OutputTokens() int        { return p.outputTokens }
func (p *TypedProcessor) CacheReadTokens() int     { return p.cacheReadTokens }
func (p *TypedProcessor) CacheCreationTokens() int { return p.cacheCreationTokens }

func (p *TypedProcessor) Result() TypedProcessorResult {
	u := p.Usage()
	return TypedProcessorResult{
		Content:      p.contentBuf.String(),
		Reasoning:    p.reasoningBuf.String(),
		ReasoningSig: p.reasoningSig,
		ToolCalls:    p.toolCalls,
		FinishReason: p.finishReason,
		Usage:        u,
	}
}

type TypedProcessorResult struct {
	Content      string
	Reasoning    string
	ReasoningSig string
	ToolCalls    []llm.ToolCall
	FinishReason llm.FinishReason
	Usage        *llm.Usage
}
