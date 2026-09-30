package streamtest

import (
	"context"

	"github.com/vanpiyp/awp/internal/llm"
)

func projectChunk(out chan<- llm.StreamEvent, c llm.StreamChunk) {
	if c.Usage != nil {
		out <- llm.EventUsage{
			InputTokens:         c.Usage.PromptTokens,
			OutputTokens:        c.Usage.CompletionTokens,
			CacheReadTokens:     c.Usage.CacheReadTokens,
			CacheCreationTokens: c.Usage.CacheCreationTokens,
		}
	}
	for _, choice := range c.Choices {
		if choice.Delta.Reasoning != "" {
			out <- llm.EventThinkingDelta{Text: choice.Delta.Reasoning}
		}
		if choice.Delta.ReasoningSig != "" {
			out <- llm.EventThinkingSignature{Signature: choice.Delta.ReasoningSig}
		}
		if choice.Delta.Content != "" {
			out <- llm.EventTextDelta{Text: choice.Delta.Content}
		}
		seenIDs := make(map[string]struct{})
		for _, tc := range choice.Delta.ToolCalls {
			if tc.ID != "" {
				if _, ok := seenIDs[tc.ID]; !ok {
					out <- llm.EventToolStart{ID: tc.ID, Name: tc.Function.Name}
					seenIDs[tc.ID] = struct{}{}
				}
			}
			if tc.Function.Arguments != "" {
				id := tc.ID
				if id == "" {
					id = "_anon"
				}
				out <- llm.EventToolDelta{ID: id, JSON: tc.Function.Arguments}
			}
		}
		if choice.FinishReason != llm.FinishReasonUnknown {
			out <- llm.EventFinish{Reason: choice.FinishReason}
		}
	}
}

func projectChunkCtx(ctx context.Context, out chan<- llm.StreamEvent, c llm.StreamChunk) bool {
	send := func(ev llm.StreamEvent) bool {
		select {
		case out <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if c.Usage != nil {
		if !send(llm.EventUsage{
			InputTokens:         c.Usage.PromptTokens,
			OutputTokens:        c.Usage.CompletionTokens,
			CacheReadTokens:     c.Usage.CacheReadTokens,
			CacheCreationTokens: c.Usage.CacheCreationTokens,
		}) {
			return false
		}
	}
	for _, choice := range c.Choices {
		if choice.Delta.Reasoning != "" {
			if !send(llm.EventThinkingDelta{Text: choice.Delta.Reasoning}) {
				return false
			}
		}
		if choice.Delta.ReasoningSig != "" {
			if !send(llm.EventThinkingSignature{Signature: choice.Delta.ReasoningSig}) {
				return false
			}
		}
		if choice.Delta.Content != "" {
			if !send(llm.EventTextDelta{Text: choice.Delta.Content}) {
				return false
			}
		}
		seenIDs := make(map[string]struct{})
		for _, tc := range choice.Delta.ToolCalls {
			if tc.ID != "" {
				if _, ok := seenIDs[tc.ID]; !ok {
					if !send(llm.EventToolStart{ID: tc.ID, Name: tc.Function.Name}) {
						return false
					}
					seenIDs[tc.ID] = struct{}{}
				}
			}
			if tc.Function.Arguments != "" {
				id := tc.ID
				if id == "" {
					id = "_anon"
				}
				if !send(llm.EventToolDelta{ID: id, JSON: tc.Function.Arguments}) {
					return false
				}
			}
		}
		if choice.FinishReason != llm.FinishReasonUnknown {
			if !send(llm.EventFinish{Reason: choice.FinishReason}) {
				return false
			}
		}
	}
	return true
}
