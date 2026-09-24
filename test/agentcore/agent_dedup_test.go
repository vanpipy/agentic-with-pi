package agentcore_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestToolResultCachePutGet(t *testing.T) {
	c := agentcore.NewToolResultCache(5)
	c.Put("a", "result-a")
	c.Put("b", "result-b")

	if v, ok := c.Get("a"); !ok || v != "result-a" {
		t.Errorf("a: got (%q, %v), want (result-a, true)", v, ok)
	}
	if v, ok := c.Get("b"); !ok || v != "result-b" {
		t.Errorf("b: got (%q, %v), want (result-b, true)", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Errorf("missing should be miss")
	}
}

func TestToolResultCacheFIFOEviction(t *testing.T) {
	c := agentcore.NewToolResultCache(3)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	if got := c.Len(); got != 3 {
		t.Errorf("len = %d, want 3", got)
	}

	c.Put("d", "4")

	if got := c.Len(); got != 3 {
		t.Errorf("after eviction: len = %d, want 3", got)
	}
	if _, ok := c.Get("a"); ok {
		t.Errorf("a should be evicted (oldest)")
	}
	if v, ok := c.Get("b"); !ok || v != "2" {
		t.Errorf("b: got (%q, %v), want (2, true)", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != "3" {
		t.Errorf("c: got (%q, %v), want (3, true)", v, ok)
	}
	if v, ok := c.Get("d"); !ok || v != "4" {
		t.Errorf("d: got (%q, %v), want (4, true)", v, ok)
	}
}

func TestToolResultCachePutExistingUpdatesInPlace(t *testing.T) {
	c := agentcore.NewToolResultCache(3)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	c.Put("a", "1-updated")

	if got := c.Len(); got != 3 {
		t.Errorf("len = %d, want 3 (no growth on duplicate)", got)
	}
	if v, ok := c.Get("a"); !ok || v != "1-updated" {
		t.Errorf("a: got (%q, %v), want (1-updated, true)", v, ok)
	}
}

func TestToolResultCacheEmptySigNoOp(t *testing.T) {
	c := agentcore.NewToolResultCache(3)
	c.Put("", "should-not-store")

	if got := c.Len(); got != 0 {
		t.Errorf("len = %d, want 0", got)
	}
}

func newCountingAgent(t *testing.T, core *fakeCore, toolName string, counter *atomic.Int32, result string) *agentcore.Agent {
	t.Helper()
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: toolName,
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
			counter.Add(1)
			return result, nil
		},
	})
	return ag
}

func TestAgentDedupIdenticalCallsAcrossTurns(t *testing.T) {
	var invocations atomic.Int32
	ag := newCountingAgent(t, &fakeCore{}, "mytool", &invocations, "cached-content")

	calls := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`},
	}}

	msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
	msgs, ok := agentcore.AgentExecuteToolsForTest(ag, calls, msgs)
	if !ok {
		t.Fatal("first executeTools failed")
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("after first: invocations = %d, want 1", got)
	}

	calls2 := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`},
	}}
	msgs2 := []llm.Message{{Role: "assistant", ToolCalls: calls2}}
	msgs2, ok = agentcore.AgentExecuteToolsForTest(ag, calls2, msgs2)
	if !ok {
		t.Fatal("second executeTools failed")
	}
	if got := invocations.Load(); got != 1 {
		t.Errorf("after second: invocations = %d, want 1 (dedup hit, no re-invoke)", got)
	}

	if len(msgs2) != 2 {
		t.Fatalf("msgs2 len = %d, want 2", len(msgs2))
	}
	if msgs2[1].Role != "tool" {
		t.Fatalf("msgs2[1].Role = %q, want tool", msgs2[1].Role)
	}
	if msgs2[1].Content != "cached-content" {
		t.Errorf("msgs2[1].Content = %q, want cached-content", msgs2[1].Content)
	}
	if msgs2[1].ToolCallID != "c2" {
		t.Errorf("msgs2[1].ToolCallID = %q, want c2 (current call's ID)", msgs2[1].ToolCallID)
	}
}

func TestAgentDedupIdenticalCallsInSameTurn(t *testing.T) {
	var invocations atomic.Int32
	ag := newCountingAgent(t, &fakeCore{}, "mytool", &invocations, "cached-content")

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`}},
		{ID: "c2", Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`}},
		{ID: "c3", Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`}},
	}
	msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
	msgs, ok := agentcore.AgentExecuteToolsForTest(ag, calls, msgs)
	if !ok {
		t.Fatal("executeTools failed")
	}

	if got := invocations.Load(); got != 1 {
		t.Errorf("invocations = %d, want 1 (first call real, rest cached)", got)
	}

	if len(msgs) != 4 {
		t.Fatalf("msgs len = %d, want 4 (1 assistant + 3 tool)", len(msgs))
	}
	for i := 1; i < 4; i++ {
		if msgs[i].Role != "tool" {
			t.Errorf("msgs[%d].Role = %q, want tool", i, msgs[i].Role)
		}
		if msgs[i].Content != "cached-content" {
			t.Errorf("msgs[%d].Content = %q, want cached-content", i, msgs[i].Content)
		}
		expectedID := fmt.Sprintf("c%d", i)
		if msgs[i].ToolCallID != expectedID {
			t.Errorf("msgs[%d].ToolCallID = %q, want %q (current call's ID)", i, msgs[i].ToolCallID, expectedID)
		}
	}
}

func TestAgentDedupDifferentArgsGetFreshExecution(t *testing.T) {
	var invocations atomic.Int32
	ag := newCountingAgent(t, &fakeCore{}, "mytool", &invocations, "result")

	calls := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`},
	}}
	msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
	_, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)

	calls2 := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":2}`},
	}}
	msgs2 := []llm.Message{{Role: "assistant", ToolCalls: calls2}}
	_, _ = agentcore.AgentExecuteToolsForTest(ag, calls2, msgs2)

	if got := invocations.Load(); got != 2 {
		t.Errorf("invocations = %d, want 2 (different args = no dedup)", got)
	}
}

func TestAgentDedupFIFOEviction(t *testing.T) {
	var invocations atomic.Int32
	ag := newCountingAgent(t, &fakeCore{}, "mytool", &invocations, "result")
	ag.WithToolCacheSize(20)

	for i := 0; i < 21; i++ {
		calls := []llm.ToolCall{{
			ID:       fmt.Sprintf("c%d", i),
			Function: llm.FunctionCall{Name: "mytool", Arguments: fmt.Sprintf(`{"x":%d}`, i)},
		}}
		msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
		_, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)
	}

	if got := invocations.Load(); got != 21 {
		t.Fatalf("after 21 distinct calls: invocations = %d, want 21", got)
	}

	calls := []llm.ToolCall{{
		ID:       "c0-again",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":0}`},
	}}
	msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
	_, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)

	if got := invocations.Load(); got != 22 {
		t.Errorf("after re-running call[0]: invocations = %d, want 22 (oldest evicted, fresh execution)", got)
	}
}

func TestAgentDedupEventObserveHasFromCacheFlag(t *testing.T) {
	var invocations atomic.Int32
	ag := newCountingAgent(t, &fakeCore{}, "mytool", &invocations, "cached-content")

	calls := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`},
	}}
	events1, _, _ := agentcore.AgentExecuteToolsWithChanForTest(ag, calls, []llm.Message{{Role: "assistant", ToolCalls: calls}})

	var sawObserve bool
	for _, ev := range events1 {
		if ev.Category == agentcore.EventObserve && ev.ToolName == "mytool" {
			sawObserve = true
			if ev.FromCache {
				t.Errorf("first call: EventObserve.FromCache = true, want false (real invocation)")
			}
		}
	}
	if !sawObserve {
		t.Fatalf("first call: expected EventObserve, events=%+v", events1)
	}

	calls2 := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "mytool", Arguments: `{"x":1}`},
	}}
	events2, _, _ := agentcore.AgentExecuteToolsWithChanForTest(ag, calls2, []llm.Message{{Role: "assistant", ToolCalls: calls2}})

	var cachedObserve int
	for _, ev := range events2 {
		if ev.Category == agentcore.EventObserve && ev.ToolName == "mytool" {
			if !ev.FromCache {
				t.Errorf("second call: EventObserve.FromCache = false, want true (dedup hit)")
			}
			if ev.ToolResult != "cached-content" {
				t.Errorf("second call: EventObserve.ToolResult = %q, want cached-content", ev.ToolResult)
			}
			cachedObserve++
		}
	}
	if cachedObserve != 1 {
		t.Errorf("second call: got %d EventObserve, want 1", cachedObserve)
	}
	if got := invocations.Load(); got != 1 {
		t.Errorf("invocations = %d, want 1 (dedup)", got)
	}
}

func TestAgentDedupDoesNotCacheErrors(t *testing.T) {
	var invocations atomic.Int32
	core := &fakeCore{}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: "flaky",
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
			invocations.Add(1)
			return "", errors.New("transient")
		},
	})

	calls := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "flaky", Arguments: `{"x":1}`},
	}}
	msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
	_, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)

	calls2 := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "flaky", Arguments: `{"x":1}`},
	}}
	msgs2 := []llm.Message{{Role: "assistant", ToolCalls: calls2}}
	_, _ = agentcore.AgentExecuteToolsForTest(ag, calls2, msgs2)

	if got := invocations.Load(); got != 2 {
		t.Errorf("invocations = %d, want 2 (errors should not be cached)", got)
	}
}

func TestAgentDedupKeyIsOrderInsensitive(t *testing.T) {
	c := agentcore.NewToolResultCache(5)
	c.Put(agentcore.ToolCallDedupKeyForTest("mytool", `{"a":1,"b":2}`), "result")

	v, ok := c.Get(agentcore.ToolCallDedupKeyForTest("mytool", `{"b":2,"a":1}`))
	if !ok {
		t.Errorf("expected hit: same key with reordered args should be the same dedup key")
	}
	if v != "result" {
		t.Errorf("got %q, want result", v)
	}
}

func TestAgentDedupKeyDiffersOnNames(t *testing.T) {
	c := agentcore.NewToolResultCache(5)
	c.Put(agentcore.ToolCallDedupKeyForTest("tool_a", `{"x":1}`), "a-result")

	if _, ok := c.Get(agentcore.ToolCallDedupKeyForTest("tool_b", `{"x":1}`)); ok {
		t.Errorf("different tool names should produce different dedup keys")
	}
}

func TestAgentDedupArgsWithUnicodeNormalize(t *testing.T) {
	c := agentcore.NewToolResultCache(5)
	key := agentcore.ToolCallDedupKeyForTest("bash", `{"command":"echo \\u4e2d\\u6587"}`)
	c.Put(key, "result")

	v, ok := c.Get(key)
	if !ok || v != "result" {
		t.Errorf("unicode args should not break native map: got (%q, %v)", v, ok)
	}
}

func TestAgentDedupSkippedWhenPreflightFails(t *testing.T) {
	var invocations atomic.Int32
	core := &fakeCore{}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: "needsintent",
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
			if !strings.Contains(argsJSON, "intent") {
				return "", fmt.Errorf("missing intent")
			}
			invocations.Add(1)
			return "ok", nil
		},
	})

	calls := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "needsintent", Arguments: `{"x":1}`},
	}}
	msgs := []llm.Message{{Role: "assistant", ToolCalls: calls}}
	_, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)

	if got := invocations.Load(); got != 0 {
		t.Errorf("invocations = %d, want 0 (preflight should fail before tool runs)", got)
	}
}
