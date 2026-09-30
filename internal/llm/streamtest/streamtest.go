// Package streamtest provides constructors and channel builders for
// sealed llm.StreamEvent values used in tests. It centralizes the
// helper surface so test files can express stream behavior in the
// same shape Core produces at runtime.
package streamtest

import (
	"context"
	"errors"

	"github.com/vanpiyp/awp/internal/llm"
)

// Chunks projects a slice of wire-shape llm.StreamChunk values into a
// sealed-event channel, mirroring what Core.streamOnce emits at runtime.
// Tests that previously constructed legacy wire-shape chunks can now
// express the same wire-level scenario as raw StreamChunks and bridge to
// sealed events through this helper. The channel is closed when all
// chunks have been projected.
func Chunks(chunks []llm.StreamChunk) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, len(chunks)*3+1)
	go func() {
		defer close(ch)
		for _, c := range chunks {
			projectChunk(ch, c)
		}
	}()
	return ch
}

// ChunksCtx is Chunks gated on ctx cancellation, for tests that want to
// exercise cancellation paths via a cancellable goroutine.
func ChunksCtx(ctx context.Context, chunks []llm.StreamChunk) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, len(chunks)*3+1)
	go func() {
		defer close(ch)
		for _, c := range chunks {
			if err := ctx.Err(); err != nil {
				return
			}
			if !projectChunkCtx(ctx, ch, c) {
				return
			}
		}
	}()
	return ch
}

// Text returns an EventTextDelta.
func Text(s string) llm.StreamEvent { return llm.EventTextDelta{Text: s} }

// Reasoning returns an EventThinkingDelta.
func Reasoning(s string) llm.StreamEvent { return llm.EventThinkingDelta{Text: s} }

// ReasoningSignature returns an EventThinkingSignature.
func ReasoningSignature(sig string) llm.StreamEvent {
	return llm.EventThinkingSignature{Signature: sig}
}

// ReasoningEnd returns an EventThinkingEnd.
func ReasoningEnd() llm.StreamEvent { return llm.EventThinkingEnd{} }

// ToolStart returns an EventToolStart with the given id and name.
func ToolStart(id, name string) llm.StreamEvent { return llm.EventToolStart{ID: id, Name: name} }

// ToolDelta returns an EventToolDelta with the given id and JSON fragment.
func ToolDelta(id, json string) llm.StreamEvent { return llm.EventToolDelta{ID: id, JSON: json} }

// ToolEnd returns an EventToolEnd with the given id.
func ToolEnd(id string) llm.StreamEvent { return llm.EventToolEnd{ID: id} }

// Finish returns an EventFinish with the given reason.
func Finish(r llm.FinishReason) llm.StreamEvent { return llm.EventFinish{Reason: r} }

// Usage returns an EventUsage with the given prompt/completion token counts.
func Usage(prompt, completion int) llm.StreamEvent {
	return llm.EventUsage{InputTokens: prompt, OutputTokens: completion}
}

// UsageFull returns an EventUsage including cache token counts.
func UsageFull(prompt, completion, cacheRead, cacheCreation int) llm.StreamEvent {
	return llm.EventUsage{
		InputTokens:         prompt,
		OutputTokens:        completion,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
	}
}

// Err returns an EventErr wrapping the given error.
func Err(err error) llm.StreamEvent { return llm.EventErr{Err: err} }

// ErrStr returns an EventErr wrapping a new error from the given string.
func ErrStr(msg string) llm.StreamEvent { return llm.EventErr{Err: errors.New(msg)} }

// SessionID returns an EventSessionID with the given id.
func SessionID(id string) llm.StreamEvent { return llm.EventSessionID{ID: id} }

// Rollback returns an EventRetryRollback.
func Rollback(attempt, max int) llm.StreamEvent {
	return llm.EventRetryRollback{Attempt: attempt, Max: max}
}

// Channel returns a closed channel that emits the given events in order.
// Use this to drive sealed-event consumers (e.g. a fake Core) in tests.
func Channel(events ...llm.StreamEvent) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent, len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

// ChannelContext returns Channel gated on ctx.Done, so test bodies that
// want to exercise cancellation can wire a cancellable context.
func ChannelContext(ctx context.Context, events ...llm.StreamEvent) <-chan llm.StreamEvent {
	ch := make(chan llm.StreamEvent)
	go func() {
		defer close(ch)
		for _, e := range events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

// TextResponse is shorthand for Channel(Text(s), Finish(reason)).
func TextResponse(s string, reason llm.FinishReason) <-chan llm.StreamEvent {
	return Channel(Text(s), Finish(reason))
}

// NoOp returns an empty EventFinish with FinishReasonUnknown (no-op for the
// typed processor). Use it in tests to fill a slot where the legacy chunk
// was just a message_stop signal with no delta.
func NoOp() llm.StreamEvent { return llm.EventCompaction{} }

// ToolStartDelta returns a slice containing an EventToolStart followed by an
// EventToolDelta for the same tool call. Tests that previously constructed a
// single chunk with both ID+Name and Arguments can use this to express the
// same scenario as two sequential sealed events.
func ToolStartDelta(id, name, args string) []llm.StreamEvent {
	return []llm.StreamEvent{
		llm.EventToolStart{ID: id, Name: name},
		llm.EventToolDelta{ID: id, JSON: args},
	}
}

// ToolDeltaOnly returns a slice with just an EventToolDelta carrying args.
// Use it for continuation deltas where ID+Name were emitted in an earlier event.
func ToolDeltaOnly(id, args string) []llm.StreamEvent {
	return []llm.StreamEvent{llm.EventToolDelta{ID: id, JSON: args}}
}

// TextChunk returns a wire-shape StreamChunk carrying a single Content delta.
// Useful for tests that store []llm.StreamChunk slices and want a TextDelta
// without spelling out the Choices nesting.
func TextChunk(s string) llm.StreamChunk {
	return llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{Content: s},
	}}}
}

// FinishChunk returns a wire-shape StreamChunk carrying only a FinishReason.
func FinishChunk(reason llm.FinishReason) llm.StreamChunk {
	return llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: reason,
	}}}
}

// FinishChunkWithUsage returns a wire-shape StreamChunk carrying a FinishReason
// and a Usage token block. Anthropic-compatible providers commonly emit both
// at message_stop.
func FinishChunkWithUsage(reason llm.FinishReason, prompt, completion int) llm.StreamChunk {
	return llm.StreamChunk{
		Choices: []llm.StreamChoice{{FinishReason: reason}},
		Usage:   &llm.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion},
	}
}

// NoOpChunk returns an empty StreamChunk, useful as a separator in
// []llm.StreamChunk slices where the legacy code used a message_stop signal.
func NoOpChunk() llm.StreamChunk { return llm.StreamChunk{} }
