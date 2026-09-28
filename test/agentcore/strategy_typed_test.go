package agentcore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestTypedProcessor_TextDeltaAccumulates(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	events := []llm.StreamEvent{
		llm.EventTextDelta{Text: "Hello"},
		llm.EventTextDelta{Text: ", "},
		llm.EventTextDelta{Text: "world!"},
	}
	for _, ev := range events {
		continue_, _, emits := tp.ProcessEvent(ctx, ev)
		if !continue_ {
			t.Fatalf("expected continue=true, got false")
		}
		if len(emits) == 0 {
			t.Errorf("expected at least one emit for text delta")
		}
	}
	if got := tp.Content(); got != "Hello, world!" {
		t.Errorf("expected Content=%q, got %q", "Hello, world!", got)
	}
}

func TestTypedProcessor_ThinkingDeltaAccumulates(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	events := []llm.StreamEvent{
		llm.EventThinkingStart{},
		llm.EventThinkingDelta{Text: "step 1: "},
		llm.EventThinkingDelta{Text: "step 2"},
		llm.EventThinkingEnd{},
	}
	for _, ev := range events {
		continue_, _, _ := tp.ProcessEvent(ctx, ev)
		if !continue_ {
			t.Fatalf("expected continue=true")
		}
	}
	if got := tp.Reasoning(); got != "step 1: step 2" {
		t.Errorf("expected Reasoning=%q, got %q", "step 1: step 2", got)
	}
}

func TestTypedProcessor_ThinkingSignatureCaptures(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	tp.ProcessEvent(ctx, llm.EventThinkingSignature{Signature: "sig-abc-123"})
	if got := tp.ReasoningSig(); got != "sig-abc-123" {
		t.Errorf("expected ReasoningSig=%q, got %q", "sig-abc-123", got)
	}
	tp.ProcessEvent(ctx, llm.EventThinkingSignature{Signature: "sig-def-456"})
	if got := tp.ReasoningSig(); got != "sig-def-456" {
		t.Errorf("expected last signature to win, got %q", got)
	}
}

func TestTypedProcessor_ToolStartDeltaEnd(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	events := []llm.StreamEvent{
		llm.EventToolStart{ID: "tc-1", Name: "search"},
		llm.EventToolDelta{ID: "tc-1", JSON: `{"query":`},
		llm.EventToolDelta{ID: "tc-1", JSON: `"foo"}`},
		llm.EventToolEnd{ID: "tc-1"},
	}
	for _, ev := range events {
		continue_, _, _ := tp.ProcessEvent(ctx, ev)
		if !continue_ {
			t.Fatalf("expected continue=true")
		}
	}
	calls := tp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].ID != "tc-1" {
		t.Errorf("expected ID=tc-1, got %q", calls[0].ID)
	}
	if calls[0].Function.Name != "search" {
		t.Errorf("expected Name=search, got %q", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"query":"foo"}` {
		t.Errorf("expected Arguments=%q, got %q", `{"query":"foo"}`, calls[0].Function.Arguments)
	}
}

func TestTypedProcessor_TwoParallelToolCalls(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	events := []llm.StreamEvent{
		llm.EventToolStart{ID: "tc-A", Name: "search"},
		llm.EventToolStart{ID: "tc-B", Name: "read"},
		llm.EventToolDelta{ID: "tc-A", JSON: `{"q":"a"}`},
		llm.EventToolDelta{ID: "tc-B", JSON: `{"p":"b"}`},
		llm.EventToolEnd{ID: "tc-A"},
		llm.EventToolEnd{ID: "tc-B"},
	}
	for _, ev := range events {
		tp.ProcessEvent(ctx, ev)
	}
	calls := tp.ToolCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].ID != "tc-A" || calls[0].Function.Name != "search" || calls[0].Function.Arguments != `{"q":"a"}` {
		t.Errorf("first call mismatch: %+v", calls[0])
	}
	if calls[1].ID != "tc-B" || calls[1].Function.Name != "read" || calls[1].Function.Arguments != `{"p":"b"}` {
		t.Errorf("second call mismatch: %+v", calls[1])
	}
}

func TestTypedProcessor_UsageAggregates(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	tp.ProcessEvent(ctx, llm.EventUsage{
		InputTokens:         100,
		OutputTokens:        50,
		CacheReadTokens:     25,
		CacheCreationTokens: 10,
	})
	if tp.InputTokens() != 100 {
		t.Errorf("expected InputTokens=100, got %d", tp.InputTokens())
	}
	if tp.OutputTokens() != 50 {
		t.Errorf("expected OutputTokens=50, got %d", tp.OutputTokens())
	}
	if tp.CacheReadTokens() != 25 {
		t.Errorf("expected CacheReadTokens=25, got %d", tp.CacheReadTokens())
	}
	if tp.CacheCreationTokens() != 10 {
		t.Errorf("expected CacheCreationTokens=10, got %d", tp.CacheCreationTokens())
	}
	if !tp.HasUsage() {
		t.Errorf("expected HasUsage=true")
	}
	usage := tp.Usage()
	if usage == nil {
		t.Fatalf("expected non-nil Usage()")
	}
	if usage.PromptTokens != 100 {
		t.Errorf("expected PromptTokens=100, got %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("expected CompletionTokens=50, got %d", usage.CompletionTokens)
	}
}

func TestTypedProcessor_FinishSetsReason(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	continue_, stepDone, emits := tp.ProcessEvent(ctx, llm.EventFinish{Reason: llm.FinishReasonToolUse})
	if !continue_ {
		t.Errorf("expected continue=true on finish")
	}
	if !stepDone {
		t.Errorf("expected stepDone=true on finish")
	}
	if len(emits) != 0 {
		t.Errorf("expected no emits on finish, got %d", len(emits))
	}
	if got := tp.FinishReason(); got != llm.FinishReasonToolUse {
		t.Errorf("expected FinishReason=ToolUse, got %v", got)
	}
}

func TestTypedProcessor_FinishStop(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	tp.ProcessEvent(ctx, llm.EventFinish{Reason: llm.FinishReasonStop})
	if got := tp.FinishReason(); got != llm.FinishReasonStop {
		t.Errorf("expected FinishReason=Stop, got %v", got)
	}
}

func TestTypedProcessor_ErrStops(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	continue_, _, emits := tp.ProcessEvent(ctx, llm.EventErr{Err: errors.New("boom")})
	if continue_ {
		t.Errorf("expected continue=false on err")
	}
	if len(emits) != 1 {
		t.Fatalf("expected 1 emit on err, got %d", len(emits))
	}
	if emits[0].Kind != stream.EmitKindError {
		t.Errorf("expected EmitKindError, got %v", emits[0].Kind)
	}
	if emits[0].Content != "boom" {
		t.Errorf("expected Content=boom, got %q", emits[0].Content)
	}
}

func TestTypedProcessor_ErrCtxNoStop(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	continue_, _, emits := tp.ProcessEvent(ctx, llm.EventErr{Err: errors.New("ignored")})
	if continue_ {
		t.Errorf("expected continue=false when ctx canceled")
	}
	if len(emits) != 0 {
		t.Errorf("expected no emits when ctx canceled, got %d", len(emits))
	}
}

func TestTypedProcessor_RetryRollbackResets(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	tp.ProcessEvent(ctx, llm.EventTextDelta{Text: "first"})
	tp.ProcessEvent(ctx, llm.EventToolStart{ID: "x", Name: "tool"})
	tp.ProcessEvent(ctx, llm.EventThinkingDelta{Text: "reasoning"})

	continue_, _, _ := tp.ProcessEvent(ctx, llm.EventRetryRollback{Attempt: 1, Max: 3})
	if !continue_ {
		t.Errorf("expected continue=true on rollback")
	}
	if got := tp.Content(); got != "" {
		t.Errorf("expected Content to be cleared, got %q", got)
	}
	if got := tp.Reasoning(); got != "" {
		t.Errorf("expected Reasoning to be cleared, got %q", got)
	}
	if got := tp.ToolCalls(); len(got) != 0 {
		t.Errorf("expected toolCalls to be cleared, got %d", len(got))
	}
}

func TestTypedProcessor_SessionIDIgnored(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	continue_, stepDone, emits := tp.ProcessEvent(ctx, llm.EventSessionID{ID: "sess-xyz"})
	if !continue_ {
		t.Errorf("expected continue=true on session ID")
	}
	if stepDone {
		t.Errorf("expected stepDone=false on session ID")
	}
	if len(emits) != 0 {
		t.Errorf("expected no emits on session ID, got %d", len(emits))
	}
}

func TestTypedProcessor_CompactionEmitsNil(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	continue_, stepDone, emits := tp.ProcessEvent(ctx, llm.EventCompaction{Trigger: "auto", PreTokens: 8000})
	if !continue_ {
		t.Errorf("expected continue=true on compaction")
	}
	if stepDone {
		t.Errorf("expected stepDone=false on compaction")
	}
	if len(emits) != 0 {
		t.Errorf("expected no emits on compaction, got %d", len(emits))
	}
}

func TestTypedProcessor_ToolDeltaUnknownIDIgnored(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	tp.ProcessEvent(ctx, llm.EventToolStart{ID: "tc-1", Name: "search"})
	tp.ProcessEvent(ctx, llm.EventToolDelta{ID: "tc-unknown", JSON: `{"lost":true}`})
	calls := tp.ToolCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Function.Arguments != "" {
		t.Errorf("expected args unchanged, got %q", calls[0].Function.Arguments)
	}
}

func TestTypedProcessor_TextDeltaEmitsObserve(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	_, _, emits := tp.ProcessEvent(ctx, llm.EventTextDelta{Text: "hi"})
	if len(emits) != 1 {
		t.Fatalf("expected 1 emit, got %d", len(emits))
	}
	if emits[0].Kind != stream.EmitKindObserve {
		t.Errorf("expected EmitKindObserve, got %v", emits[0].Kind)
	}
	if emits[0].Content != "hi" {
		t.Errorf("expected Content=hi, got %q", emits[0].Content)
	}
}

func TestTypedProcessor_ToolStartEmitsTool(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	_, _, emits := tp.ProcessEvent(ctx, llm.EventToolStart{ID: "tc-1", Name: "search"})
	if len(emits) != 1 {
		t.Fatalf("expected 1 emit, got %d", len(emits))
	}
	if emits[0].Kind != stream.EmitKindTool {
		t.Errorf("expected EmitKindTool, got %v", emits[0].Kind)
	}
	if emits[0].ToolName != "search" {
		t.Errorf("expected ToolName=search, got %q", emits[0].ToolName)
	}
}

func TestTypedProcessor_ToolEndEmitsNothing(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	_, _, emits := tp.ProcessEvent(ctx, llm.EventToolEnd{ID: "tc-1"})
	if len(emits) != 0 {
		t.Errorf("expected 0 emits on tool end, got %d", len(emits))
	}
}

func TestTypedProcessor_UsagePointerStable(t *testing.T) {
	tp := stream.NewTypedProcessor()
	ctx := context.Background()
	tp.ProcessEvent(ctx, llm.EventUsage{InputTokens: 1, OutputTokens: 2})
	if !tp.HasUsage() {
		t.Fatalf("expected HasUsage=true after first usage")
	}
	tp.ProcessEvent(ctx, llm.EventUsage{InputTokens: 3, OutputTokens: 4})
	if tp.InputTokens() != 3 || tp.OutputTokens() != 4 {
		t.Errorf("expected second usage to reflect latest event, got in=%d out=%d", tp.InputTokens(), tp.OutputTokens())
	}
}
