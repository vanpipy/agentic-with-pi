package agentcore_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
)

func newExecuteToolsAgent(t *testing.T) *agentcore.Agent {
	t.Helper()
	return agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
}

func TestTryExecuteToolCallEmptyArgsSkip(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(context.Context, string) (string, error) { return "ok", nil }})

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: "   "}}
	events, updated, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, []llm.ToolCall{tc}, nil)
	if !ok {
		t.Fatal("ok = false, want true (empty-args skip continues the chain)")
	}
	var sawErr bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "empty arguments") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected EventError mentioning 'empty arguments'; events=%+v", events)
	}
	if len(updated) != 1 {
		t.Fatalf("updated len = %d, want 1 (one tool skip message)", len(updated))
	}
	last := updated[len(updated)-1]
	if last.Role != "tool" || last.ToolCallID != "c1" || !strings.Contains(last.Content, "empty arguments") {
		t.Errorf("expected trailing tool message with skip content; got role=%q id=%q content=%q", last.Role, last.ToolCallID, last.Content)
	}
}

func TestTryExecuteToolCallUnknownToolError(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	ag.WithTool(agentcore.ToolFunc{N: "registered", Fn: func(context.Context, string) (string, error) { return "ok", nil }})

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ghost", Arguments: `{"intent":"x"}`}}
	calls := []llm.ToolCall{tc, {ID: "c2", Function: llm.FunctionCall{Name: "registered", Arguments: `{"intent":"y"}`}}}
	inputMsgs := []llm.Message{{Role: "user", Content: "u"}}
	events, updated, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, calls, inputMsgs)
	if ok {
		t.Fatal("ok = true, want false (unknown tool aborts the chain)")
	}
	var sawErr bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "is not registered") && strings.Contains(ev.ToolError, "registered") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Errorf("expected EventError mentioning 'is not registered' and the available 'registered' tool; events=%+v", events)
	}
	appended := updated[len(inputMsgs):]
	if len(appended) != len(calls) {
		t.Fatalf("appended len = %d, want %d (one skip for failing call, one skip for remaining)", len(appended), len(calls))
	}
	for i, m := range appended {
		if m.Role != "tool" {
			t.Errorf("appended[%d].Role = %q, want tool", i, m.Role)
		}
		if !strings.Contains(m.Content, "is not registered") {
			t.Errorf("appended[%d].Content missing 'is not registered': %q", i, m.Content)
		}
	}
}

func TestTryExecuteToolCallPreflightFailureRecords(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	ag.WithTool(agentcore.ToolFunc{
		N: "needs_path",
		P: map[string]any{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "the file path"},
			},
		},
		Fn: func(context.Context, string) (string, error) { return "", nil },
	})

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "needs_path", Arguments: `{}`}}
	calls := []llm.ToolCall{tc, {ID: "c2", Function: llm.FunctionCall{Name: "needs_path", Arguments: `{}`}}}
	events, updated, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, calls, nil)
	if !ok {
		t.Fatal("ok = false, want true (preflight failure continues the chain)")
	}
	if got := ag.PreflightFailureStreakForTest()["needs_path"]; got != 1 {
		t.Errorf("preflight streak[needs_path] = %d, want 1", got)
	}
	var sawPreflight bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "missing required field") && strings.Contains(ev.ToolError, "path") {
			sawPreflight = true
		}
	}
	if !sawPreflight {
		t.Errorf("expected preflight EventError mentioning 'missing required field' and 'path'; events=%+v", events)
	}
	if len(updated) != 2 {
		t.Fatalf("updated len = %d, want 2 (one preflight msg + one skipped-tail msg)", len(updated))
	}
	if !strings.Contains(updated[0].Content, "missing required field") {
		t.Errorf("updated[0].Content missing preflight text: %q", updated[0].Content)
	}
	if !strings.Contains(updated[1].Content, "prior tool needs_path failed preflight validation") {
		t.Errorf("updated[1].Content missing tail-skip text: %q", updated[1].Content)
	}
}

func TestTryExecuteToolCallDedupHitReturnsCached(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	var calls int
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) {
		calls++
		return "v", nil
	}})
	ag.PreflightFailureStreakForTest()["echo"] = 3

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "echo", Arguments: `{"intent":"x","v":1}`}}
	calls1, ok := stream.AgentExecuteToolsForTest(ag, []llm.ToolCall{tc}, nil)
	if !ok {
		t.Fatal("prime cache: executeTools ok = false")
	}
	if calls != 1 {
		t.Fatalf("prime cache: tool calls = %d, want 1", calls)
	}

	events, updated, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, []llm.ToolCall{tc}, calls1)
	if !ok {
		t.Fatal("ok = false, want true (cache hit continues the chain)")
	}
	if calls != 1 {
		t.Errorf("after cache hit: tool calls = %d, want 1 (cache hit must not re-invoke)", calls)
	}
	if got := ag.PreflightFailureStreakForTest()["echo"]; got != 0 {
		t.Errorf("after cache hit: streak[echo] = %d, want 0 (cache hit clears streak)", got)
	}
	var sawCacheObserve bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindObserve && ev.FromCache && ev.ToolResult == "v" {
			sawCacheObserve = true
		}
	}
	if !sawCacheObserve {
		t.Errorf("expected EventObserve with FromCache=true and ToolResult='v'; events=%+v", events)
	}
	if len(updated) <= len(calls1) {
		t.Fatalf("updated len = %d, want > %d (cache hit must append tool message)", len(updated), len(calls1))
	}
	last := updated[len(updated)-1]
	if last.Role != "tool" || last.ToolCallID != "c1" || last.Content != "v" {
		t.Errorf("expected trailing tool message with cached content 'v'; got role=%q id=%q content=%q", last.Role, last.ToolCallID, last.Content)
	}
}

func TestTryExecuteToolCallInvokeSuccess(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) { return "done", nil }})

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "echo", Arguments: `{"intent":"hi"}`}}
	events, updated, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, []llm.ToolCall{tc}, nil)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	var sawTool, sawObserve bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindTool && ev.ToolName == "echo" {
			sawTool = true
		}
		if ev.Kind == stream.EmitKindObserve && ev.ToolResult == "done" && !ev.FromCache {
			sawObserve = true
		}
	}
	if !sawTool {
		t.Errorf("expected EventTool for 'echo'; events=%+v", events)
	}
	if !sawObserve {
		t.Errorf("expected EventObserve with ToolResult='done' and FromCache=false; events=%+v", events)
	}
	if len(updated) != 1 || updated[0].Content != "done" || updated[0].ToolCallID != "c1" {
		t.Fatalf("expected one tool message with content 'done'; got %+v", updated)
	}
	if got := ag.PreflightFailureStreakForTest()["echo"]; got != 0 {
		t.Errorf("after success: streak[echo] = %d, want 0 (cleared)", got)
	}
}

func TestTryExecuteToolCallInvokeErrorMapping(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	ag.WithTool(agentcore.ToolFunc{N: "boom", Fn: func(context.Context, string) (string, error) { return "", errors.New("kaboom") }})

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "boom", Arguments: `{"intent":"x"}`}}
	events, updated, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, []llm.ToolCall{tc}, nil)
	if !ok {
		t.Fatal("ok = false, want true (tool error continues the chain)")
	}
	var sawObserveErr, sawExtraErr bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindObserve && ev.ToolError == "kaboom" {
			sawObserveErr = true
		}
		if ev.Kind == stream.EmitKindError && ev.ToolError == "kaboom" && !ev.FromCache {
			sawExtraErr = true
		}
	}
	if !sawObserveErr {
		t.Errorf("expected EventObserve with ToolError='kaboom'; events=%+v", events)
	}
	if !sawExtraErr {
		t.Errorf("expected trailing EventError with ToolError='kaboom'; events=%+v", events)
	}
	if len(updated) != 1 {
		t.Fatalf("updated len = %d, want 1 (one failure tool message)", len(updated))
	}
	last := updated[len(updated)-1]
	if !strings.Contains(last.Content, "Tool boom failed: kaboom") {
		t.Errorf("expected trailing message 'Tool boom failed: kaboom'; got %q", last.Content)
	}
}

func TestTryExecuteToolCallInvokeErrorInvalidToolCategory(t *testing.T) {
	ag := newExecuteToolsAgent(t)
	ag.WithTool(agentcore.ToolFunc{N: "invalid", Fn: func(context.Context, string) (string, error) { return "", errors.New("bad shape") }})

	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "invalid", Arguments: `{"intent":"x"}`}}
	events, _, ok := stream.AgentTryExecuteToolCallForTest(ag, context.Background(), tc, 0, []llm.ToolCall{tc}, nil)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	var sawInvalid bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindInvalid && ev.ToolError == "bad shape" {
			sawInvalid = true
		}
	}
	if !sawInvalid {
		t.Errorf("expected EventInvalid with ToolError='bad shape' when tool name is 'invalid'; events=%+v", events)
	}
}
