package agentcore_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/streamtest"
)

func streamPlaceholderToolCall(id, name, args string) llm.StreamEvent {
	return streamtest.ToolStart(id, name)
}

func streamDeltaToolCall(id, args string) llm.StreamEvent {
	return streamtest.ToolDelta(id, args)
}

func TestStreamMerge_T1_SingleCallMergesMultipleDeltasIntoOneEntry(t *testing.T) {
	events := []llm.StreamEvent{
		streamPlaceholderToolCall("c1", "bash", ""),
		streamDeltaToolCall("c1", `{"com`),
		streamDeltaToolCall("c1", `mand`),
		streamDeltaToolCall("c1", `":"ls"}`),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1 (single tool call accumulated from start + 3 deltas)", len(calls))
	}
	if calls[0].ID != "c1" {
		t.Errorf("calls[0].ID = %q, want c1", calls[0].ID)
	}
	if calls[0].Function.Name != "bash" {
		t.Errorf("calls[0].Name = %q, want bash", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"command":"ls"}` {
		t.Errorf("calls[0].Arguments = %q, want %q", calls[0].Function.Arguments, `{"command":"ls"}`)
	}
}

func TestStreamMerge_T2_TwoParallelCallsKeepDistinctEntries(t *testing.T) {
	events := []llm.StreamEvent{
		streamPlaceholderToolCall("cA", "bash", ""),
		streamPlaceholderToolCall("cB", "read", ""),
		streamDeltaToolCall("cA", `{"com`),
		streamDeltaToolCall("cA", `mand":"ls"}`),
		streamDeltaToolCall("cB", `{"path`),
		streamDeltaToolCall("cB", `":"x"}`),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2 (parallel tool calls stay distinct)", len(calls))
	}
	a, b := calls[0], calls[1]
	if a.ID != "cA" || a.Function.Name != "bash" {
		t.Errorf("calls[0] = {%q,%q}, want {cA,bash}", a.ID, a.Function.Name)
	}
	if b.ID != "cB" || b.Function.Name != "read" {
		t.Errorf("calls[1] = {%q,%q}, want {cB,read}", b.ID, b.Function.Name)
	}
	if a.Function.Arguments != `{"command":"ls"}` {
		t.Errorf("calls[0].Arguments = %q, want %q", a.Function.Arguments, `{"command":"ls"}`)
	}
	if b.Function.Arguments != `{"path":"x"}` {
		t.Errorf("calls[1].Arguments = %q, want %q", b.Function.Arguments, `{"path":"x"}`)
	}
}

func TestStreamMerge_T3_ByteOrderPreservedAcrossDeltas(t *testing.T) {
	a := `{"command":"echo hi","note":"first part "`
	b := `,"trailing":"x"}`
	events := []llm.StreamEvent{
		streamPlaceholderToolCall("c1", "bash", ""),
		streamDeltaToolCall("c1", a),
		streamDeltaToolCall("c1", b),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1", len(calls))
	}
	want := a + b
	if calls[0].Function.Arguments != want {
		t.Errorf("byte-level concat mismatch:\n got %q\nwant %q", calls[0].Function.Arguments, want)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &parsed); err != nil {
		t.Fatalf("merged arguments not valid JSON: %v\nargs: %q", err, calls[0].Function.Arguments)
	}
	if parsed["command"] != "echo hi" {
		t.Errorf("command = %v, want 'echo hi'", parsed["command"])
	}
	if parsed["trailing"] != "x" {
		t.Errorf("trailing = %v, want 'x'", parsed["trailing"])
	}
	if parsed["note"] != "first part " {
		t.Errorf("note = %v, want 'first part '", parsed["note"])
	}
}

func newBashToolAgent(t *testing.T) *agentcore.Agent {
	t.Helper()
	ag := newTestAgent(&fakeCore{}, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: "bash",
		P: map[string]any{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
		},
		Fn: func(_ context.Context, _ string) (string, error) {
			return "ran", nil
		},
	})
	return ag
}

func TestStreamMerge_T4_TruncatedStreamPlaceholderSkippedByExecutor(t *testing.T) {
	events := []llm.StreamEvent{
		streamPlaceholderToolCall("c1", "bash", ""),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1 (truncated stream should still surface placeholder entry)", len(calls))
	}
	if calls[0].ID != "c1" {
		t.Errorf("calls[0].ID = %q, want c1", calls[0].ID)
	}
	if strings.TrimSpace(calls[0].Function.Arguments) != "" {
		t.Errorf("calls[0].Arguments = %q, want empty placeholder", calls[0].Function.Arguments)
	}

	ag := newBashToolAgent(t)
	msgs := []llm.Message{{Role: "user", Content: "explore"}, {Role: "assistant", ToolCalls: calls}}
	evs, msgs, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if !ok {
		t.Fatalf("executeTools returned ok=false, want true (defensive skip should not abort)")
	}

	skipMsg := ""
	for _, m := range msgs {
		if m.Role == "tool" && strings.Contains(m.Content, "empty arguments") {
			skipMsg = m.Content
			break
		}
	}
	if skipMsg == "" {
		t.Fatalf("expected tool result msg mentioning 'empty arguments', got msgs=%+v events=%+v", msgs, evs)
	}
	if !strings.Contains(skipMsg, "bash") {
		t.Errorf("skip msg should name the failing tool 'bash', got %q", skipMsg)
	}

	toolInvocationCount := 0
	for _, m := range msgs {
		if m.Role == "tool" && m.Content == "ran" {
			toolInvocationCount++
		}
	}
	if toolInvocationCount != 0 {
		t.Errorf("placeholder call must not invoke the real tool; got %d successful runs", toolInvocationCount)
	}

	sawSkipErrorEvent := false
	for _, ev := range evs {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "empty arguments") && ev.ToolName == "bash" {
			sawSkipErrorEvent = true
		}
	}
	if !sawSkipErrorEvent {
		t.Errorf("expected EventError event with ToolName=bash + 'empty arguments' in ToolError; events=%+v", evs)
	}
}

func TestStreamMerge_BackwardCompat_TestHelperPlaceholdersThenDelta(t *testing.T) {
	events := []llm.StreamEvent{
		streamtest.ToolStart("c1", "read"),
		streamtest.ToolDelta("c1", `{"path":"AGENTS.md"}`),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1 (test helper '{}' placeholder + same-id delta should yield 1 entry)", len(calls))
	}
	if calls[0].Function.Arguments != `{"path":"AGENTS.md"}` {
		t.Errorf("calls[0].Arguments = %q, want %q (sentinel replacement, not append)", calls[0].Function.Arguments, `{"path":"AGENTS.md"}`)
	}
	if calls[0].Function.Name != "read" {
		t.Errorf("calls[0].Name = %q, want read", calls[0].Function.Name)
	}
}

func TestStreamMerge_DeltaWithoutIDAppendsToLastPlaceholder(t *testing.T) {
	events := []llm.StreamEvent{
		streamPlaceholderToolCall("c1", "bash", ""),
		streamtest.ToolDelta("", `{"command":"ls"}`),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %d, want 1 (anonymous delta should attach to last placeholder)", len(calls))
	}
	if calls[0].Function.Arguments != `{"command":"ls"}` {
		t.Errorf("calls[0].Arguments = %q, want %q", calls[0].Function.Arguments, `{"command":"ls"}`)
	}
}

func TestStreamMerge_NewIDAppendsAndKeepsPriorIntact(t *testing.T) {
	events := []llm.StreamEvent{
		streamPlaceholderToolCall("cA", "bash", ""),
		streamDeltaToolCall("cA", `{"command":"ls"}`),
		streamPlaceholderToolCall("cB", "read", ""),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(events)
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %d, want 2 (second call with new ID must append, not merge into first)", len(calls))
	}
	if calls[0].Function.Arguments != `{"command":"ls"}` {
		t.Errorf("calls[0].Arguments = %q, want %q", calls[0].Function.Arguments, `{"command":"ls"}`)
	}
	if calls[1].ID != "cB" {
		t.Errorf("calls[1].ID = %q, want cB", calls[1].ID)
	}
	if calls[1].Function.Arguments != "" {
		t.Errorf("calls[1].Arguments = %q, want empty (placeholder only)", calls[1].Function.Arguments)
	}
}

func TestStreamMerge_EmptyArgsEntryIsPassedToExecutorAsSkip(t *testing.T) {
	raw := []llm.ToolCall{{
		ID:       "c1",
		Type:     "function",
		Function: llm.FunctionCall{Name: "bash", Arguments: ""},
	}}
	ag := newBashToolAgent(t)
	msgs := []llm.Message{{Role: "user", Content: "x"}, {Role: "assistant", ToolCalls: raw}}
	_, msgs, ok := stream.AgentExecuteToolsWithChanForTest(ag, raw, msgs)
	if !ok {
		t.Fatal("executeTools returned ok=false, want true (skip must not abort the chain)")
	}
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3 (user + assistant + 1 tool skip msg)", len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != "tool" {
		t.Fatalf("last msg role = %q, want tool", last.Role)
	}
	if !strings.Contains(last.Content, "empty arguments") {
		t.Errorf("skip msg = %q, want contains 'empty arguments'", last.Content)
	}
	if last.ToolCallID != "c1" {
		t.Errorf("skip msg ToolCallID = %q, want c1 (alignment with assistant.tool_calls)", last.ToolCallID)
	}
}

func TestStreamMerge_ExecutesMergedCallEndToEnd(t *testing.T) {
	callsObserved := []string{}
	ag := newTestAgent(&fakeCore{}, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: "bash",
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			callsObserved = append(callsObserved, argsJSON)
			var parsed struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &parsed); err != nil {
				return "", errors.New("invalid args: " + err.Error())
			}
			return "ok:" + parsed.Command, nil
		},
	})
	chunks := [][]llm.StreamEvent{
		{
			streamPlaceholderToolCall("c1", "bash", ""),
			streamDeltaToolCall("c1", `{"command`),
			streamDeltaToolCall("c1", `":"date`),
			streamDeltaToolCall("c1", `"}`),
			streamtest.Finish(llm.FinishReasonToolUse),
			streamtest.NoOp(),
		},
		{
			streamtest.Text("done at " + "2026-09-25"),
			streamtest.Finish(llm.FinishReasonStop),
			streamtest.NoOp(),
		},
	}
	core := &fakeCore{streamEventsList: chunks}
	ag2 := newTestAgent(core, "test-model")
	ag2.WithTool(agentcore.ToolFunc{
		N: "bash",
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			callsObserved = append(callsObserved, argsJSON)
			return "ok", nil
		},
	})

	result, err := runAgentLastError(t, ag2, "what's the date?")
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	if result == "" {
		t.Fatalf("expected final answer, got empty")
	}
	if len(callsObserved) != 1 {
		t.Errorf("tool invoked %d times, want 1 (merge must collapse placeholders+deltas to a single tool call)", len(callsObserved))
	}
	if len(callsObserved) > 0 && callsObserved[0] != `{"command":"date"}` {
		t.Errorf("tool args = %q, want %q", callsObserved[0], `{"command":"date"}`)
	}
}
