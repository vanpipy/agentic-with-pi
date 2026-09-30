package agentcore_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/streamtest"
)

func TestWriteMessageErrorWhenLogWriterNil(t *testing.T) {
	ag := &agentcore.Agent{}
	err := ag.WriteMessage(json_rpc.MessageEvent{ID: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "log writer not initialized") {
		t.Errorf("err = %q, want mentions 'log writer not initialized'", err.Error())
	}
}

func TestWriteCustomErrorWhenLogWriterNil(t *testing.T) {
	ag := &agentcore.Agent{}
	if err := ag.WriteCustom("p", "type", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteCustomMessageErrorWhenLogWriterNil(t *testing.T) {
	ag := &agentcore.Agent{}
	if err := ag.WriteCustomMessage("p", "type", "content", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteMessagePropagatesToBuffer(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	if err := a.WriteMessage(json_rpc.MessageEvent{ID: "msg-x", Timestamp: "2026-09-25T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	flushAgentLog(a)
	if !strings.Contains(buf.String(), `"id":"msg-x"`) {
		t.Errorf("buf should contain msg-x: %q", buf.String())
	}
}

func TestWriteCustomPropagatesToBuffer(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	if err := a.WriteCustom("p", "custom-type", json.RawMessage(`{"k":"v"}`)); err != nil {
		t.Fatal(err)
	}
	flushAgentLog(a)
	if !strings.Contains(buf.String(), `"custom-type"`) {
		t.Errorf("buf should contain custom-type: %q", buf.String())
	}
}

func TestWriteCustomMessagePropagatesToBuffer(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	if err := a.WriteCustomMessage("p", "custom-msg-type", "content-x", json.RawMessage(`{"k":"v"}`)); err != nil {
		t.Fatal(err)
	}
	flushAgentLog(a)
	if !strings.Contains(buf.String(), `"custom-msg-type"`) {
		t.Errorf("buf should contain custom-msg-type: %q", buf.String())
	}
}

func TestWriteAlignedEventUserMessageWritesAlignedMessage(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "hi"})

	flushAgentLog(a)

	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
	if !strings.Contains(buf.String(), `"role":"user"`) {
		t.Errorf("buf should contain user role: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"stopReason":"end_turn"`) {
		t.Errorf("buf should contain end_turn stopReason: %q", buf.String())
	}
}

func TestWriteAlignedEventThoughtChunkEmitsNoLineButPreservesBuf(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "u"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "think"})

	flushAgentLog(a)
	if buf.Len() == 0 {
		t.Fatal("expected at least the user message line")
	}
}

func TestWriteAlignedEventErrorWhenNoBufUsesCurrentParentID(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "hi"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventError, ToolError: "boom-without-buf"})

	flushAgentLog(a)
	if buf.Len() == 0 {
		t.Fatal("expected some log output")
	}
}

func TestWriteAlignedEventToolEmitsAssistantAndToolResult(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "u"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventTool, ToolName: "bash", ToolArgs: `{"command":"date","intent":"x"}`, ToolIntent: "x"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventObserve, ToolName: "bash", ToolResult: "out", ToolIntent: "x"})

	flushAgentLog(a)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines (user + assistant + toolResult), got %d:\n%s", len(lines), buf.String())
	}
}

func TestWriteAlignedEventWithObserveToolErrorEmitsToolResult(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventTool, ToolName: "bash", ToolArgs: `{"intent":"x"}`})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventObserve, ToolName: "bash", ToolError: "exit 1", ToolIntent: "x"})

	flushAgentLog(a)
	if !strings.Contains(buf.String(), `"error":"exit 1"`) {
		t.Errorf("buf should contain error=exit 1: %q", buf.String())
	}
}

func TestWriteAlignedEventFinalAnswerSetsRoleOnFinal(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "answer"})

	flushAgentLog(a)
	if !strings.Contains(buf.String(), `"role":"assistant"`) {
		t.Errorf("buf should contain assistant role: %q", buf.String())
	}
}

func TestWriteAlignedEventErrorAfterUserMessageAnchorsToUser(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "hi"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventError, ToolError: "boom"})

	flushAgentLog(a)
	if !strings.Contains(buf.String(), `"customType":"tool_error"`) {
		t.Errorf("buf should contain tool_error customType: %q", buf.String())
	}
}

func TestWriteAlignedEventEmptyCategoryIsNoOp(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventCategory(99999)})

	flushAgentLog(a)
	if buf.Len() != 0 {
		t.Errorf("empty category should not write, got %q", buf.String())
	}
}

func TestWriteAlignedEventThoughtEndPopulatesUsage(t *testing.T) {
	a, buf := newTestAgentWithBuf()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "hi"})
	usage := &llm.Usage{PromptTokens: 11, CompletionTokens: 22, TotalTokens: 33}
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtEnd, Usage: usage})

	flushAgentLog(a)
	if buf.Len() == 0 {
		t.Fatal("expected log output")
	}
}

func TestFlushLogForTestWithBufWrites(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogWriterForTest(&buf)
	ag.SetLogFileOpenedForTest(true)
	if err := ag.WriteMessage(json_rpc.MessageEvent{ID: "x"}); err != nil {
		t.Fatal(err)
	}
	ag.FlushLogForTest()
	if buf.Len() == 0 {
		t.Fatal("expected flush to write content")
	}
}

func TestFlushLogForTestNoopOnNilLogBuf(t *testing.T) {
	ag := &agentcore.Agent{LogWriter: nil}
	ag.FlushLogForTest()
}

func TestDefaultSessionLogPathHonoursAwpSessionLogPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", filepath.Join(tmp, "explicit.jsonl"))

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)

	for range ag.RunStream(context.Background(), "hi") {
	}

	if _, err := os.Stat(filepath.Join(tmp, "explicit.jsonl")); err != nil {
		t.Errorf("explicit log path not created: %v", err)
	}
}

func TestDefaultSessionLogPathFallsBackWhenEnvEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)

	for range ag.RunStream(context.Background(), "hi") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.jsonl"))
	if len(files) == 0 {
		t.Errorf("expected fallback path to write a session file under %s", tmp)
	}
}

func TestOpenLogLockedSuppressesViaAwpNoSessionLog(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "1")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)

	for range ag.RunStream(context.Background(), "hi") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.jsonl"))
	if len(files) != 0 {
		t.Errorf("AWP_NO_SESSION_LOG should suppress file creation, got %d", len(files))
	}
}

func TestOpenLogLockedHandlesMkdirFailure(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(parent, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWP_HOME", parent)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)

	for ev := range ag.RunStream(context.Background(), "hi") {
		_ = ev
	}
}

func TestOpenLogLockedUsesExistingLogWriterWhenSet(t *testing.T) {
	var buf bytes.Buffer
	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core).WithLogWriter(&buf)
	ag.WithSessionID("explicit-id")

	for range ag.RunStream(context.Background(), "hi") {
	}

	if buf.Len() == 0 {
		t.Fatal("WithLogWriter buffer should contain log content")
	}
}

func TestWriteTurnStartForTestNoopWithoutLogWriter(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteTurnStartForTest("msg-1")
}

func TestWriteTurnResponseForTestNoopWithoutLogWriter(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteTurnResponseForTest("m", "v", "stop", 1, nil, 0, 0, 0)
}

func TestWriteToolDedupHitForTestNoopWithoutLogWriter(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteToolDedupHitForTest("bash", "x", 1)
}

func TestWriteCompactionV3ForTestNoopWithoutLogWriter(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteCompactionV3ForTest("manual_command", "", "reactive", 1, 1, 1, "m", 1)
}

func TestWriteErrorV3ForTestNoopWithoutLogWriter(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteErrorV3ForTest("aft_worker", "crash", "boom", 1, false)
}

func TestWriteTurnStartLockedWithRealSession(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})

	for range ag.RunStream(context.Background(), "x") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	if len(files) != 1 {
		t.Fatalf("expected 1 session log file, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	sawTurnStart := false
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["kind"] == "turn_start" {
			sawTurnStart = true
			if int(entry["context_window"].(float64)) != 128000 {
				t.Errorf("default context_window = %v, want 128000", entry["context_window"])
			}
		}
	}
	if !sawTurnStart {
		t.Errorf("expected turn_start entry in:\n%s", string(data))
	}
}

func TestWriteTurnResponseLockedWithTTFTEmitted(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", Vendor: "anthropic"})
	for range ag.RunStream(context.Background(), "x") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	if len(files) != 1 {
		t.Skipf("expected v3 file at %s", tmp)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"kind":"turn_response"`) {
		t.Errorf("expected turn_response entry in v3 file:\n%s", string(data))
	}
}

func TestLogEventForTestNoWriterIsNoop(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LogEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "noop"})
}

func TestLogEventForTestSkipsWhenWriterNil(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LogEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})
}

func TestLogEventForTestInOrderWithLegacyHandoff(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "p1"})
	ag.WriteLegacyEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "p2"})
	ag.WriteLegacyEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "p3"})

	flushAgentLog(ag)
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), out)
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d unmarshal: %v", i, err)
		}
		if entry["category"] != "user_message" {
			t.Errorf("line %d category = %v, want user_message", i, entry["category"])
		}
	}
}

func TestWriteLegacyEventForTestAdvancesSequenceAfterMany(t *testing.T) {
	ag, buf := newV3TestAgent()
	for i := 0; i < 5; i++ {
		ag.WriteLegacyEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: fmt.Sprintf("a-%d", i)})
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d:\n%s", len(lines), out)
	}
}

func TestReActStrategyProcessStreamEventCapturesReasoningSig(t *testing.T) {
	chunks := []llm.StreamEvent{
		streamtest.Text("hello"),
		streamtest.ReasoningSignature("sig-abc"),
		streamtest.Finish(llm.FinishReasonStop),
		streamtest.NoOp(),
	}
	calls := agentcore.AccumulateStreamToolCallsForTest(chunks)
	if len(calls) != 0 {
		t.Errorf("calls = %d, want 0 (no tool calls)", len(calls))
	}
}

func TestReActStrategyProcessStreamEventCtxErrFromStream(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.Text("ok"),
		llm.EventErr{Err: context.DeadlineExceeded},
	})
	if len(calls) != 0 {
		t.Errorf("calls = %d, want 0 (ctx err short-circuits)", len(calls))
	}
}

func TestMergeToolCallDeltaReplacesPlaceholderArgs(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.ToolStartDelta("c1", "ls", "{}")[0],
		streamtest.ToolStartDelta("c1", "ls", "{}")[1],
		streamtest.ToolStartDelta("c1", "", `{"path":"."}`)[0],
		streamtest.ToolStartDelta("c1", "", `{"path":"."}`)[1],
		streamtest.Finish(llm.FinishReasonToolUse),
		streamtest.NoOp(),
	})
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Arguments != `{"path":"."}` {
		t.Errorf("args = %q, want %q (placeholder replaced)", calls[0].Function.Arguments, `{"path":"."}`)
	}
}

func TestMergeToolCallDeltaAppendsToExistingArgs(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.ToolStartDelta("c1", "ls", `{"path":`)[0],
		streamtest.ToolStartDelta("c1", "ls", `{"path":`)[1],
		streamtest.ToolStartDelta("c1", "", `".","deep":true}`)[0],
		streamtest.ToolStartDelta("c1", "", `".","deep":true}`)[1],
		streamtest.Finish(llm.FinishReasonToolUse),
		streamtest.NoOp(),
	})
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	want := `{"path":".","deep":true}`
	if calls[0].Function.Arguments != want {
		t.Errorf("args = %q, want %q (appended)", calls[0].Function.Arguments, want)
	}
}

func TestMergeToolCallDeltaFillsName(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.ToolStartDelta("c1", "", `{}`)[0],
		streamtest.ToolStartDelta("c1", "", `{}`)[1],
		streamtest.ToolStartDelta("c1", "name-added", "")[0],
		streamtest.ToolStartDelta("c1", "name-added", "")[1],
		streamtest.Finish(llm.FinishReasonToolUse),
		streamtest.NoOp(),
	})
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "name-added" {
		t.Errorf("name = %q, want name-added", calls[0].Function.Name)
	}
}

func TestAgentResetForRunWithNilStreakInitializesMap(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.ResetForRun()
	streak := ag.PreflightFailureStreakForTest()
	if streak == nil {
		t.Fatal("ResetForRun should initialize the streak map even when nil")
	}
	if len(streak) != 0 {
		t.Errorf("streak len = %d, want 0", len(streak))
	}
}

func TestAgentResetForRunClearsExistingStreak(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ag.PreflightFailureStreakForTest()["alpha"] = 5
	ag.PreflightFailureStreakForTest()["beta"] = 2

	ag.ResetForRun()

	if got := ag.PreflightFailureStreakForTest()["alpha"]; got != 0 {
		t.Errorf("alpha after reset = %d, want 0", got)
	}
	if got := ag.PreflightFailureStreakForTest()["beta"]; got != 0 {
		t.Errorf("beta after reset = %d, want 0", got)
	}
	if len(ag.PreflightFailureStreakForTest()) != 0 {
		t.Errorf("streak len = %d, want 0", len(ag.PreflightFailureStreakForTest()))
	}
}

func TestAgentResetForRunClearsAllKeys(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	streak := ag.PreflightFailureStreakForTest()
	streak["x"] = 1
	streak["y"] = 2
	streak["z"] = 3
	ag.ResetForRun()
	if len(ag.PreflightFailureStreakForTest()) != 0 {
		t.Errorf("streak len after reset = %d, want 0", len(ag.PreflightFailureStreakForTest()))
	}
}

func TestAgentCheckPreflightStreakEmptyMapNoError(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ag.WithModel(llm.Model{ID: "m"})
	if err := ag.ShouldAbort(nil, ""); err != nil {
		t.Errorf("ShouldAbort(empty streak) = %v, want nil", err)
	}
}

func TestAgentCheckPreflightStreakBelowThresholdNoError(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ag.WithModel(llm.Model{ID: "m"})
	streak := ag.PreflightFailureStreakForTest()
	streak["alpha"] = 1
	if err := ag.ShouldAbort(nil, ""); err != nil {
		t.Errorf("ShouldAbort = %v, want nil (below threshold)", err)
	}
}

func TestAllEqualSingleElement(t *testing.T) {
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "x", Arguments: "{}"}}
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
	}
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	if err := strat.ShouldAbort(msgs, ""); err != nil {
		t.Errorf("ShouldAbort = %v, want nil (1 assistant tool call)", err)
	}
}

func TestAgentExecuteToolsEmitsAllBranchesInOneTurn(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "good", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "bad", Fn: func(context.Context, string) (string, error) { return "", errors.New("nope") }})

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "good", Arguments: `{"intent":"good"}`}},
		{ID: "c2", Function: llm.FunctionCall{Name: "bad", Arguments: `{"intent":"bad"}`}},
		{ID: "c3", Function: llm.FunctionCall{Name: "missing", Arguments: `{}`}},
	}

	msgs := []llm.Message{{Role: "user", Content: "x"}}
	_, _, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if ok {
		t.Error("ok = true, want false (missing tool aborts)")
	}
}

func TestPreflightValidateEmptyArgsReturnsEmpty(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{
		N: "needs_x",
		P: map[string]any{
			"type":     "object",
			"required": []string{"x"},
		},
		Fn: func(context.Context, string) (string, error) { return "", nil },
	})
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "needs_x", Arguments: ""}}
	calls := []llm.ToolCall{tc}
	_, _, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})
	if !ok {
		t.Error("ok = false, want true (empty args short-circuits preflight)")
	}
}

func TestPreflightValidateMissingFieldWithPropertyDescription(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
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
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "needs_path", Arguments: `{}`}},
	}
	events, _, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})
	if !ok {
		t.Error("ok = false, want true")
	}
	var sawDesc bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "the file path") {
			sawDesc = true
		}
	}
	if !sawDesc {
		t.Errorf("expected preflight error to include property description; events=%+v", events)
	}
}

func TestPreflightValidateMissingFieldNoDescriptionFallsBackToName(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{
		N: "needs_x",
		P: map[string]any{
			"type":     "object",
			"required": []string{"x"},
			"properties": map[string]any{
				"x": map[string]any{"type": "string"},
			},
		},
		Fn: func(context.Context, string) (string, error) { return "", nil },
	})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "needs_x", Arguments: `{}`}},
	}
	events, _, _ := stream.AgentExecuteToolsWithChanForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})
	var sawFieldName bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "missing required field") {
			sawFieldName = true
		}
	}
	if !sawFieldName {
		t.Errorf("expected preflight error mentioning 'missing required field'; events=%+v", events)
	}
}

func TestPreflightValidateNoRequiredFieldsReturnsEmpty(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{
		N: "no_required",
		P: map[string]any{
			"type": "object",
		},
		Fn: func(context.Context, string) (string, error) { return "ok", nil },
	})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "no_required", Arguments: `{}`}},
	}
	events, _, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})
	if !ok {
		t.Error("ok = false, want true")
	}
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError {
			t.Errorf("unexpected error event: %+v", ev)
		}
	}
}

func TestPreflightValidateSchemaNilReturnsEmpty(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{
		N:  "no_schema",
		P:  nil,
		Fn: func(context.Context, string) (string, error) { return "ok", nil },
	})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "no_schema", Arguments: `{}`}},
	}
	_, _, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})
	if !ok {
		t.Error("ok = false, want true")
	}
}

func TestApplyOversizedGuardAcceptsLargeOutputWhenRequested(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	big := strings.Repeat("x", stream.OversizedResultThreshold+10)
	ag.WithTool(agentcore.ToolFunc{N: "big_tool", Fn: func(context.Context, string) (string, error) { return big, nil }})

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "big_tool", Arguments: `{"intent":"big","accept_large_output":true}`}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	updated, ok := stream.AgentExecuteToolsForTest(ag, calls, msgs)
	if !ok {
		t.Fatal("executeTools failed")
	}
	for _, m := range updated {
		if strings.Contains(m.Content, "OUTPUT WITHHELD") {
			t.Errorf("expected full output to pass through; got withheld: %q", m.Content)
		}
	}
}

func TestApplyOversizedGuardReplacesOutputWhenTooLarge(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	big := strings.Repeat("x", stream.OversizedResultThreshold+10)
	ag.WithTool(agentcore.ToolFunc{N: "big_tool", Fn: func(context.Context, string) (string, error) { return big, nil }})

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "big_tool", Arguments: `{"intent":"big"}`}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	updated, ok := stream.AgentExecuteToolsForTest(ag, calls, msgs)
	if !ok {
		t.Fatal("executeTools failed")
	}
	var sawWithheld bool
	for _, m := range updated {
		if strings.Contains(m.Content, "OUTPUT WITHHELD") {
			sawWithheld = true
		}
	}
	if !sawWithheld {
		t.Errorf("expected output to be withheld for >%d bytes; updated=%+v", stream.OversizedResultThreshold, updated)
	}
}

func TestAgentFindToolReturnsToolAt(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "a", Fn: func(context.Context, string) (string, error) { return "1", nil }})
	ag.WithTool(agentcore.ToolFunc{N: "b", Fn: func(context.Context, string) (string, error) { return "2", nil }})

	t1, ok1 := agentcore.AgentFindToolForTest(ag, "a")
	if !ok1 {
		t.Fatal("find a failed")
	}
	if got, _ := t1.Invoke(context.Background(), "{}"); got != "1" {
		t.Errorf("a invoke = %q, want 1", got)
	}

	t2, ok2 := agentcore.AgentFindToolForTest(ag, "b")
	if !ok2 {
		t.Fatal("find b failed")
	}
	if got, _ := t2.Invoke(context.Background(), "{}"); got != "2" {
		t.Errorf("b invoke = %q, want 2", got)
	}

	if _, ok := agentcore.AgentFindToolForTest(ag, "missing"); ok {
		t.Error("missing should return ok=false")
	}
}

func TestAgentLockUnlockLogNoop(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LockLogForTest()
	ag.UnlockLogForTest()
}

func TestAgentLockUnlockLog(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LockLogForTest()
	ag.UnlockLogForTest()
}

func TestAgentCurrentParentIDForTestAfterWriteMessage(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LogWriter = nil
	ag.LogEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "x"})
	if got := ag.CurrentParentIDForTest(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAgentCurrentParentIDForTestAfterWriteMessageUpdates(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LogWriter = nil
	if got := ag.CurrentParentIDForTest(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAgentSetLogFileOpenedRoundTrip(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.SetLogFileOpenedForTest(true)
	ag.SetLogFileOpenedForTest(false)
}

func TestAgentSetLogFileOpenedNoOpOnNilAgent(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.SetLogFileOpenedForTest(true)
}

func TestReActStrategyShouldAbortConsecutiveErrorsSkipRole(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"a"}`}}
	tc2 := llm.ToolCall{ID: "c2", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"b"}`}}
	errMsg := "Tool ls failed: kapow"
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc2}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc2}},
		{Role: "tool", Content: errMsg},
	}
	err := strat.ShouldAbort(msgs, errMsg)
	if err == nil {
		t.Fatal("expected non-nil abort, want consecutive tool errors")
	}
}

func TestAgentExecuteToolsCacheHitMissPathCovers(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	calls_count := 0
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) {
		calls_count++
		return "v", nil
	}})

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "echo", Arguments: `{"intent":"x","v":1}`}},
	}
	updated1, _ := stream.AgentExecuteToolsForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})
	if calls_count != 1 {
		t.Fatalf("first call count = %d, want 1", calls_count)
	}

	_, _, _ = stream.AgentExecuteToolsWithChanForTest(ag, calls, updated1)
	if calls_count != 1 {
		t.Errorf("second call count = %d, want 1 (cache hit)", calls_count)
	}
}

func TestAgentExecuteToolsCacheHitClearsPreflightStreak(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.PreflightFailureStreakForTest()["alpha"] = 5

	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "v", nil }})

	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "alpha", Arguments: `{"intent":"x"}`}},
	}
	updated1, _ := stream.AgentExecuteToolsForTest(ag, calls, []llm.Message{{Role: "user", Content: "x"}})

	if ag.PreflightFailureStreakForTest()["alpha"] != 0 {
		t.Errorf("streak after success = %d, want 0 (cleared)", ag.PreflightFailureStreakForTest()["alpha"])
	}

	_, _, _ = stream.AgentExecuteToolsWithChanForTest(ag, calls, updated1)
	if ag.PreflightFailureStreakForTest()["alpha"] != 0 {
		t.Errorf("streak after cache hit = %d, want 0 (still cleared)", ag.PreflightFailureStreakForTest()["alpha"])
	}
}

func TestAgentRunStreamResumedEmptyHistoryPrependsSystem(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("hi"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "u", nil)
	for ev := range ch {
		_ = ev
	}
	msgs := core.requests[0].Messages
	if msgs[0].Role != "system" {
		t.Errorf("first msg role = %q, want system", msgs[0].Role)
	}
}

func TestAgentRunStreamResumedSeedHistoryAppendsUser(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("hi"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	history := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "earlier"},
		{Role: "assistant", Content: "earlier-reply"},
	}
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "next", history)
	for ev := range ch {
		_ = ev
	}
	msgs := core.requests[0].Messages
	if msgs[len(msgs)-1].Content != "next" {
		t.Errorf("last msg = %q, want next", msgs[len(msgs)-1].Content)
	}
}

func TestAgentLoopWithMsgsStepFinalExitsEarly(t *testing.T) {
	chunksList := [][]llm.StreamEvent{
		{
			streamtest.Text("final answer"),
			streamtest.Finish(llm.FinishReasonStop),
			streamtest.NoOp(),
		},
	}
	core := &fakeCore{streamEventsList: chunksList}
	ag := newTestAgent(core, "m")

	var finalContent string
	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			finalContent = ev.Content
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Fatal("expected final answer")
	}
	if finalContent != "final answer" {
		t.Errorf("final = %q, want final answer", finalContent)
	}
}

func TestAgentLoopWithMsgsToolInvocationClearsCacheAndStreak(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{
			toolUseStartChunk("c1", "echo"),
			toolUseIDDeltaChunk("c1", "echo", `{"intent":"x"}`),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("done"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) { return "v", nil }})
	ag.PreflightFailureStreakForTest()["echo"] = 5

	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Error("expected final answer after tool use")
	}
	if ag.PreflightFailureStreakForTest()["echo"] != 0 {
		t.Errorf("streak after success = %d, want 0", ag.PreflightFailureStreakForTest()["echo"])
	}
}

func TestAgentLoopWithMsgsWritesV3TurnStartAndResponseOnFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	for range ag.RunStream(context.Background(), "x") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	if len(files) != 1 {
		t.Fatalf("expected 1 v3 session log file, got %d", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	var sawV3 bool
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["kind"] == "turn_start" || entry["kind"] == "turn_response" {
			sawV3 = true
		}
	}
	if !sawV3 {
		t.Errorf("expected v3 turn_start/turn_response entries; got:\n%s", content)
	}
}

func TestAgentLoopWithMsgsRecoversFromPreflightAbort(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ag.PreflightFailureStreakForTest()["alpha"] = 2

	var sawSomething bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventError || ev.Category == agentcore.EventFinalAnswer {
			sawSomething = true
		}
	}
	if !sawSomething {
		t.Error("expected any event (below threshold should NOT abort)")
	}
}

func TestAgentLoopWithMsgsFinalAfterToolTriggersCleanup(t *testing.T) {
	chunksList := [][]llm.StreamEvent{
		{
			streamtest.ToolStart("c1", "echo"),
			streamtest.ToolDelta("c1", `{"intent":"x"}`),
			streamtest.Finish(llm.FinishReasonToolUse),
			streamtest.NoOp(),
		},
		{
			streamtest.Text("done"),
			streamtest.Finish(llm.FinishReasonStop),
			streamtest.NoOp(),
		},
	}
	core := &fakeCore{streamEventsList: chunksList}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) { return "v", nil }})

	var finalContent string
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			finalContent = ev.Content
		}
	}
	if finalContent != "done" {
		t.Errorf("final = %q, want done", finalContent)
	}
}

func TestAgentLoopWithMsgsEmptyLoopEmitsSafetyNet(t *testing.T) {
	chunks := [][]llm.StreamEvent{}
	for i := 0; i < 3; i++ {
		chunks = append(chunks, []llm.StreamEvent{
			streamtest.ToolStart(fmt.Sprintf("c%d", i+1), "noop"),
			streamtest.ToolDelta(fmt.Sprintf("c%d", i+1), fmt.Sprintf(`{"intent":"x%d"}`, i+1)),
			streamtest.Finish(llm.FinishReasonToolUse),
			streamtest.NoOp(),
		})
	}
	core := &fakeCore{streamEventsList: chunks}
	ag := newTestAgent(core, "m").WithSafetyNet(2)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(context.Context, string) (string, error) { return "ok", nil }})

	var sawSafety bool
	for ev := range ag.RunStream(context.Background(), "loop forever") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "safety net reached") {
			sawSafety = true
		}
	}
	if !sawSafety {
		t.Error("expected safety net error")
	}
}

func TestReActStrategyStepStreamErrIsCtxStillReturnsErr(t *testing.T) {
	strat := agentcore.NewReActStrategy(&ctxErrCore{}, llm.Model{ID: "m"}, nil, nil)
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, func(context.Context, agentcore.Event) bool { return true })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if step.Kind != agentcore.StepContinue {
		t.Errorf("step.Kind = %v, want StepContinue", step.Kind)
	}
}

func TestReActStrategyStreamEventChunkNilEmitsNoChunkContent(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{streamChunks: []llm.StreamChunk{
		{},
		textDeltaChunk("hi"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}, llm.Model{ID: "m"}, nil, nil)
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, func(context.Context, agentcore.Event) bool { return true })
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Content != "hi" {
		t.Errorf("content = %q, want hi", step.Content)
	}
}

func TestReActStrategyShouldAbortWithNormalizedErrorStreak(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "x", Arguments: `{}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: "Tool x failed: boom"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: "Tool x failed: boom"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: "Tool x failed: boom"},
	}
	if err := strat.ShouldAbort(msgs, "Tool x failed: boom"); err == nil {
		t.Error("ShouldAbort = nil, want non-nil after normalized-streak hits limit")
	}
}

func TestReActStrategyShouldAbortWithNonNormalizedErrorsDoesNotHitLimit(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc1 := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "x", Arguments: `{"v":1}`}}
	tc2 := llm.ToolCall{ID: "c2", Function: llm.FunctionCall{Name: "x", Arguments: `{"v":2}`}}
	tc3 := llm.ToolCall{ID: "c3", Function: llm.FunctionCall{Name: "x", Arguments: `{"v":3}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc1}},
		{Role: "tool", Content: "Tool x failed: different error each time"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc2}},
		{Role: "tool", Content: "Tool x failed: yet another error message entirely"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc3}},
		{Role: "tool", Content: "Tool x failed: yet another error entirely"},
	}
	if err := strat.ShouldAbort(msgs, "Tool x failed: yet another error entirely"); err != nil {
		t.Errorf("ShouldAbort = %v, want nil (errors differ too much)", err)
	}
}

func TestAgentRunStreamEmitsErrorWhenModelNotSet(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})

	var sawModelErr bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "Model not set") {
			sawModelErr = true
		}
	}
	if !sawModelErr {
		t.Error("expected Model-not-set error")
	}
}

func TestReActStrategyStepEmitReturnsFalseInLoop(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("a"),
		textDeltaChunk("b"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	var sawThoughtStart, sawThoughtChunk bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventThoughtStart {
			sawThoughtStart = true
		}
		if ev.Category == agentcore.EventThoughtChunk {
			sawThoughtChunk = true
			return false
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Errorf("err = %v, want nil (ctx.Err() returned but ctx not canceled)", err)
	}
	if step.Content != "" {
		t.Errorf("step.Content = %q, want empty (Step{} returned on emit=false)", step.Content)
	}
	if !sawThoughtStart || !sawThoughtChunk {
		t.Errorf("sawThoughtStart=%v sawThoughtChunk=%v", sawThoughtStart, sawThoughtChunk)
	}
}

func TestAgentWithModelUpdatesStrategyModelPointer(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ag.WithModel(llm.Model{ID: "first"})
	ag.WithModel(llm.Model{ID: "second"})
	if ag.Model.ID != "second" {
		t.Errorf("Model.ID = %q, want second", ag.Model.ID)
	}
}

func TestReActStrategyStepStreamErrEmitsErrorThenThoughtEnd(t *testing.T) {
	ch := make(chan llm.StreamEvent, 3)
	ch <- llm.EventErr{Err: errors.New("upstream-llm-err-12345")}
	close(ch)
	strat := agentcore.NewReActStrategy(&streamErrCore{ch: ch}, llm.Model{ID: "m"}, nil, nil)

	var sawError, sawThoughtEnd bool
	var sawErrMsg string
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventError {
			sawError = true
			sawErrMsg = ev.ToolError
		}
		if ev.Category == agentcore.EventThoughtEnd {
			sawThoughtEnd = true
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepContinue || step.Content != "" {
		t.Errorf("step = %+v, want empty StepContinue", step)
	}
	if !sawError || !sawThoughtEnd {
		t.Errorf("sawError=%v sawThoughtEnd=%v, want both true", sawError, sawThoughtEnd)
	}
	if sawErrMsg == "" || !strings.Contains(sawErrMsg, "upstream-llm-err-12345") {
		t.Errorf("err msg = %q, want contains upstream-llm-err-12345", sawErrMsg)
	}
}

type streamErrCore struct{ ch chan llm.StreamEvent }

func (s *streamErrCore) StreamChat(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return s.ch, nil
}

func TestAgentExecuteToolsErrorOnEmptyArgsStopsCurrentTurn(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "ok", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "ok", Arguments: ""}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	_, _, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if !ok {
		t.Error("ok = false, want true (empty args skips but doesn't abort turn)")
	}
}

func TestAgentExecuteToolsWritesDedupHitLogEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")

	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{
			toolUseStartChunk("c1", "echo"),
			toolUseIDDeltaChunk("c1", "echo", `{"intent":"x"}`),
			toolUseStartChunk("c2", "echo"),
			toolUseIDDeltaChunk("c2", "echo", `{"intent":"x"}`),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("done"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) { return "v", nil }})

	for range ag.RunStream(context.Background(), "x") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	if len(files) == 0 {
		t.Fatal("no v3 file produced")
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"tool_dedup_hit"`) {
		t.Errorf("expected tool_dedup_hit v3 entry; got %s", string(data))
	}
}

func TestReActStrategyShouldAbortConsecutiveNormalizedErrors(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{}`}}
	errMsg := "Tool ls failed: boom-of-doom"
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
	}
	err := strat.ShouldAbort(msgs, errMsg)
	if err == nil {
		t.Fatal("ShouldAbort = nil, want non-nil after 3 consecutive identical errors")
	}
	if !strings.Contains(err.Error(), "aborting") {
		t.Errorf("err = %q, want mentions aborting", err.Error())
	}
}

func TestAgentWithSessionIDForEmptyString(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithSessionID("")
	if got := ag.SessionIDForTest(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAgentWithSessionIDForCustomString(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithSessionID("xyz-123")
	if got := ag.SessionIDForTest(); got != "xyz-123" {
		t.Errorf("got %q, want xyz-123", got)
	}
}

func TestAgentSetSystemPromptsRoundTrip(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m"})
	ag.SetSystemPrompts("custom system prompt")
	for range ag.RunStream(context.Background(), "x") {
	}
	if len(core.requests) == 0 {
		t.Fatal("no requests recorded")
	}
	if got := core.requests[0].Messages[0].Content; got != "custom system prompt" {
		t.Errorf("system = %q, want custom system prompt", got)
	}
}

func TestAgentExecuteToolsReturnsSkippedToolResultsForUnknownTool(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "exists", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "exists", Arguments: `{}`}},
		{ID: "c2", Function: llm.FunctionCall{Name: "ghost1", Arguments: `{}`}},
		{ID: "c3", Function: llm.FunctionCall{Name: "ghost2", Arguments: `{}`}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	_, updated, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if ok {
		t.Error("ok = true, want false")
	}
	if len(updated) < 4 {
		t.Errorf("updated len = %d, want at least 4 (user + tool result + 2 skipped fills)", len(updated))
	}
}

func TestAgentLoopWithMsgsEmitsObservedTokensAfterTurn(t *testing.T) {
	chunks := [][]llm.StreamEvent{
		{
			streamtest.Text("ok"),
			streamtest.Finish(llm.FinishReasonStop),
			streamtest.NoOp(),
		},
	}
	core := &fakeCore{streamEventsList: chunks}
	ag := newTestAgent(core, "m")

	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			break
		}
	}
}

func TestAgentWriteTurnResponseEndedWithoutUsageRecordsZeroes(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("hi"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag.SetCoreForTest(core)

	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			break
		}
	}
}

func TestReActStrategyEmitFalseAfterErrorRetainsStepZero(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	emit := func(_ context.Context, _ agentcore.Event) bool { return true }
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Usage != nil {
		t.Errorf("step.Usage = %v, want nil", step.Usage)
	}
}

func TestReActStrategyStepEmitsFinalAnswerWithUsage(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		streamtest.FinishChunkWithUsage(llm.FinishReasonStop, 5, 5),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	var sawUsageInEmit bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventFinalAnswer && ev.Usage != nil {
			sawUsageInEmit = true
		}
		return true
	}
	step, _ := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if !sawUsageInEmit {
		t.Error("expected usage propagated to final answer event")
	}
	if step.Usage == nil {
		t.Error("step.Usage nil, want non-nil")
	}
}

func TestAgentLoopWithMsgsContinuesAfterToolWithDifferentArgs(t *testing.T) {
	chunksList := [][]llm.StreamEvent{
		{
			streamtest.ToolStart("c1", "echo"),
			streamtest.ToolDelta("c1", `{"intent":"first"}`),
			streamtest.Finish(llm.FinishReasonToolUse),
			streamtest.NoOp(),
		},
		{
			streamtest.ToolStart("c2", "echo"),
			streamtest.ToolDelta("c2", `{"intent":"second"}`),
			streamtest.Finish(llm.FinishReasonToolUse),
			streamtest.NoOp(),
		},
		{
			streamtest.Text("done"),
			streamtest.Finish(llm.FinishReasonStop),
			streamtest.NoOp(),
		},
	}
	core := &fakeCore{streamEventsList: chunksList}
	ag := newTestAgent(core, "m").WithSafetyNet(10)
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) { return "v", nil }})

	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Error("expected final answer")
	}
}

func TestAgentRunStreamResumedWithSnapshotClosesSinkChannel(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	_, sinkCh := ag.RunStreamResumedWithSnapshot(context.Background(), "x", nil)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-sinkCh:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("sinkCh not closed within deadline")
		}
	}
}

func TestAgentRunStreamEmitsAllEventTypes(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")

	categories := map[agentcore.EventCategory]bool{}
	for ev := range ag.RunStream(context.Background(), "x") {
		categories[ev.Category] = true
	}
	if !categories[agentcore.EventThoughtStart] {
		t.Error("missing EventThoughtStart")
	}
	if !categories[agentcore.EventThoughtChunk] {
		t.Error("missing EventThoughtChunk")
	}
	if !categories[agentcore.EventThoughtEnd] {
		t.Error("missing EventThoughtEnd")
	}
	if !categories[agentcore.EventFinalAnswer] {
		t.Error("missing EventFinalAnswer")
	}
}

func TestAgentOpenLogLockedHandlesDuplicateExistingBuf(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.WithLogWriter(&bytes.Buffer{})
	ag.SetLogFileOpenedForTest(true)
	for range ag.RunStream(context.Background(), "x") {
	}
}

func TestAgentRunStreamResumedWithNilHistoryUsesBaseMessages(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "user", nil)
	for range ch {
	}
	if len(core.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(core.requests))
	}
	msgs := core.requests[0].Messages
	if msgs[0].Role != "system" || msgs[len(msgs)-1].Content != "user" {
		t.Errorf("messages malformed: %+v", msgs)
	}
}

func TestAgentOpenLogLockedSkipsHeaderWhenAlreadyBuffered(t *testing.T) {
	ag := &agentcore.Agent{LogWriter: nil}
	core := &fakeCoreForRecord{}
	ag.SetCoreForTest(core)
	ag.WithSessionID("skip-header-id")

	var buf bytes.Buffer
	ag.WithLogWriter(&buf)
	ag.SetLogFileOpenedForTest(true)

	for range ag.RunStream(context.Background(), "x") {
	}

	if !strings.Contains(buf.String(), `"kind":"session"`) {
		t.Errorf("expected session header; got %q", buf.String())
	}
}

func TestWriteAlignedEventThoughtEndUsageAppliesToBuf(t *testing.T) {
	ag, _ := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "think-1"})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "think-2"})
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category: agentcore.EventThoughtEnd,
		Usage:    &llm.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15},
	})
}

func TestWriteAlignedEventToolSetsCurrentToolCallID(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category:   agentcore.EventTool,
		ToolName:   "ls",
		ToolArgs:   `{"intent":"x","path":"."}`,
		ToolIntent: "x",
	})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventObserve, ToolName: "ls", ToolResult: "out"})
	flushAgentLog(ag)
	if !strings.Contains(buf.String(), `"role":"toolResult"`) {
		t.Errorf("expected toolResult message; got %q", buf.String())
	}
}

func TestWriteAlignedEventFinalAnswerEmitsMessageAndClearsBuf(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Content: "x"})
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "answer",
	})
	flushAgentLog(ag)
	out := buf.String()
	if !strings.Contains(out, `"role":"assistant"`) || !strings.Contains(out, `"text":"answer"`) {
		t.Errorf("expected assistant message with text=answer; got %q", out)
	}
}

func TestWriteAlignedEventErrorWithBufEmitsMessageAndCustom(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Content: "x"})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventError, ToolError: "boom"})
	flushAgentLog(ag)
	out := buf.String()
	if !strings.Contains(out, `"customType":"tool_error"`) {
		t.Errorf("expected tool_error custom; got %q", out)
	}
}

func TestWriteAlignedEventErrorNoBufUsesCurrentParentID(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "u1"})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventError, ToolError: "boom"})
	flushAgentLog(ag)
	if !strings.Contains(buf.String(), `"customType":"tool_error"`) {
		t.Errorf("expected tool_error custom; got %q", buf.String())
	}
}

func TestWriteAlignedEventToolSkipsWhenNoBuf(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category:   agentcore.EventTool,
		ToolName:   "ls",
		ToolArgs:   `{"intent":"x"}`,
		ToolIntent: "x",
	})
	flushAgentLog(ag)
	if buf.Len() != 0 {
		t.Errorf("expected empty buffer; got %q", buf.String())
	}
}

func TestWriteAlignedEventThoughtChunkNoBufReturnsNil(t *testing.T) {
	ag, _ := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category:  agentcore.EventThoughtChunk,
		Reasoning: "no-buf",
	})
}

func TestReActStrategyStepEmitFalseAfterThoughtStartReturnsCtxErr(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("x"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventThoughtStart {
			return false
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Errorf("err = %v, want nil (ctx not canceled)", err)
	}
	if step.Content != "" {
		t.Errorf("step.Content = %q, want empty", step.Content)
	}
}

func TestReActStrategyStepCtxErrDuringLoopReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("x"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	emit := func(_ context.Context, ev agentcore.Event) bool { return true }
	step, err := strat.Step(ctx, []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err == nil {
		t.Errorf("err = nil, want ctx.Err()")
	}
	_ = step
}

func TestReActStrategyStepToolUseReturnsStepContinue(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("c1", "ls"),
		toolUseIDDeltaChunk("c1", "ls", `{"intent":"x","path":"."}`),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	emit := func(_ context.Context, ev agentcore.Event) bool { return true }
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "ls"}}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if step.Kind != agentcore.StepContinue {
		t.Errorf("step.Kind = %v, want StepContinue", step.Kind)
	}
	if len(step.ToolCalls) == 0 {
		t.Error("expected tool calls in step")
	}
}

func TestAgentToolCallsLimitTruncatedLogsWarning(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("x"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ag.SafetyNet = 1
	for range ag.RunStream(context.Background(), "x") {
	}
}

func TestAgentRunStreamEmitsErrorEventWhenCoreFails(t *testing.T) {
	core := &fakeCore{streamErr: errors.New("upstream-bad-xyz")}
	ag := newTestAgent(core, "m")
	var sawError bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "upstream") {
			sawError = true
		}
	}
	if !sawError {
		t.Error("expected upstream-error event")
	}
}

func TestAgentLoopWithMsgsEmitsFinalAnswerWithUsage(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		streamtest.FinishChunkWithUsage(llm.FinishReasonStop, 10, 10),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	var final *agentcore.Event
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			ev := ev
			final = &ev
		}
	}
	if final == nil {
		t.Fatal("no final answer")
	}
	if final.Usage == nil || final.Usage.TotalTokens != 20 {
		t.Errorf("usage = %+v, want TotalTokens=20", final.Usage)
	}
}

func TestAgentEmitsFinalAnswerOnLengthWithContentV2(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("partial"),
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Error("expected final answer on length stop reason with content")
	}
}

func TestAgentLoopWithMsgsEmitsPreflightFailureAndSkipsRest(t *testing.T) {
	core := &fakeCore{streamChunksList: [][]llm.StreamChunk{
		{
			toolUseStartChunk("c1", "needs_path"),
			toolUseIDDeltaChunk("c1", "needs_path", `{}`),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("ok"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{
		N: "needs_path",
		P: map[string]any{
			"type":     "object",
			"required": []string{"path"},
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
		Fn: func(context.Context, string) (string, error) { return "ok", nil },
	})

	var sawFinal bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventFinalAnswer {
			sawFinal = true
		}
	}
	if !sawFinal {
		t.Error("expected final answer after preflight failure recovery")
	}
}

func TestAgentRunStreamEmitsErrorWhenCoreReturnsErr(t *testing.T) {
	core := &fakeCore{streamErr: errors.New("upstream-bad-xyz")}
	ag := newTestAgent(core, "m")
	var sawError bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "upstream") {
			sawError = true
		}
	}
	if !sawError {
		t.Error("expected upstream-error event")
	}
}

func TestReActStrategyShouldAbortIdenticalArgsHitsLimit(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"."}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
	}
	if err := strat.ShouldAbort(msgs, ""); err == nil {
		t.Error("ShouldAbort = nil, want abort after 4 identical calls")
	}
}

func readV3Session(t *testing.T, tmp string) []byte {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	if len(files) == 0 {
		t.Fatalf("no v3 session file in %s", tmp)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v3Field(t *testing.T, data []byte, kind string, key string) any {
	t.Helper()
	var lastMatch any
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["kind"] == kind {
			lastMatch = entry[key]
		}
	}
	return lastMatch
}

func TestWriteTurnStartLockedWithZeroContextWindowUsesDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 0})

	for range ag.RunStream(context.Background(), "x") {
	}

	data := readV3Session(t, tmp)
	if got := v3Field(t, data, "turn_start", "context_window"); got == nil || int(got.(float64)) != 128000 {
		t.Errorf("context_window = %v, want 128000", got)
	}
}

func findLatestV3(t *testing.T, tmp string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	if len(files) == 0 {
		t.Fatalf("no v3 session file in %s", tmp)
	}
	return files[len(files)-1]
}

func TestWriteAlignedEventToolMalformedArgsFallsBackToEmptyIntent(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category:   agentcore.EventTool,
		ToolName:   "ls",
		ToolArgs:   `{not-json`,
		ToolIntent: "",
	})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventObserve, ToolName: "ls", ToolResult: "out"})
	flushAgentLog(ag)
	if !strings.Contains(buf.String(), `"role":"toolResult"`) {
		t.Errorf("expected toolResult; got %q", buf.String())
	}
}

func TestWriteAlignedEventFinalAnswerWithUsageEmitsUsage(t *testing.T) {
	ag, buf := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Content: "x"})
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "answer",
		Usage:    &llm.Usage{PromptTokens: 3, CompletionTokens: 7, TotalTokens: 10},
	})
	flushAgentLog(ag)
	out := buf.String()
	if !strings.Contains(out, `"text":"answer"`) {
		t.Errorf("expected answer text; got %q", out)
	}
}

func TestWriteAlignedEventFinalAnswerEmptyBufReturnsNil(t *testing.T) {
	ag, _ := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "no-buf",
	})
}

func TestWriteAlignedEventThoughtEndEmptyBufReturnsNil(t *testing.T) {
	ag, _ := newV3TestAgent()
	ag.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtEnd})
}

func TestWriteMessageWithoutLogWriterReturnsErr(t *testing.T) {
	ag := &agentcore.Agent{}
	if err := ag.WriteMessage(json_rpc.MessageEvent{Message: json_rpc.Message{Role: "user", Content: []json_rpc.MessageContentPart{{Type: "text", Text: "x"}}}}); err == nil {
		t.Error("expected error when LogWriter is nil")
	}
}

func TestWriteCustomWithoutLogWriterReturnsErr(t *testing.T) {
	ag := &agentcore.Agent{}
	if err := ag.WriteCustom("p", "t", json.RawMessage(`{}`)); err == nil {
		t.Error("expected error when LogWriter is nil")
	}
}

func TestWriteCustomMessageWithoutLogWriterReturnsErr(t *testing.T) {
	ag := &agentcore.Agent{}
	if err := ag.WriteCustomMessage("p", "t", "c", nil); err == nil {
		t.Error("expected error when LogWriter is nil")
	}
}

func TestWriteEventForTestWithoutLogBufSkips(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "u1"})
}

func TestLogEventForTestNoOpOnNilWriter(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.LogEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "u1"})
}

func TestAgentRunStreamWithNegativeMaxTokensUsesDefault(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ag.WithModel(llm.Model{ID: "m", MaxContextTokens: 0})
	for range ag.RunStream(context.Background(), "x") {
	}
}

func TestWriteHeaderLockedWithLongSystemPromptTruncates(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ag.SetSystemPrompts(strings.Repeat("x", 250))

	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}
}

func TestReActStrategyShouldAbortWithEmptyMessagesReturnsNil(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	if err := strat.ShouldAbort([]llm.Message{}, ""); err != nil {
		t.Errorf("ShouldAbort = %v, want nil for empty msgs", err)
	}
}

func TestReActStrategyShouldAbortNoIdenticalSigsDoesNotAbort(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc1 := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"a"}`}}
	tc2 := llm.ToolCall{ID: "c2", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"b"}`}}
	tc3 := llm.ToolCall{ID: "c3", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"c"}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc1}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc2}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc3}},
	}
	if err := strat.ShouldAbort(msgs, ""); err != nil {
		t.Errorf("ShouldAbort = %v, want nil for varied signatures", err)
	}
}

func TestReActStrategyShouldAbortAllEmptyArgsDoesNotHitIdenticalLimit(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc1 := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: ""}}
	tc2 := llm.ToolCall{ID: "c2", Function: llm.FunctionCall{Name: "ls", Arguments: ""}}
	tc3 := llm.ToolCall{ID: "c3", Function: llm.FunctionCall{Name: "ls", Arguments: ""}}
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc1}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc2}},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc3}},
	}
	_ = strat.ShouldAbort(msgs, "")
}

func TestAgentSetSystemPromptsRoundTripLongTruncatesInHeader(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ag.SetSystemPrompts(strings.Repeat("y", 300))

	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}
}

func TestApplyOversizedGuardWithAcceptLargeOutputReturnsFull(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	bigResult := strings.Repeat("x", 200000)
	ag.WithTool(agentcore.ToolFunc{
		N:  "big_tool",
		P:  map[string]any{"type": "object"},
		Fn: func(context.Context, string) (string, error) { return bigResult, nil },
	})
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "big_tool", Arguments: `{"accept_large_output":true}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
	}
	_, _, _ = stream.AgentExecuteToolsWithChanForTest(ag, []llm.ToolCall{tc}, msgs)
}

func TestApplyOversizedGuardMalformedJSONArgsReturnsWithheld(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	bigResult := strings.Repeat("x", 200000)
	ag.WithTool(agentcore.ToolFunc{
		N:  "big_tool",
		D:  "big",
		P:  map[string]any{"type": "object"},
		Fn: func(context.Context, string) (string, error) { return bigResult, nil },
	})
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "big_tool", Arguments: `{malformed`}}
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
	}
	_, updated, _ := stream.AgentExecuteToolsWithChanForTest(ag, []llm.ToolCall{tc}, msgs)
	if len(updated) == 0 {
		t.Fatal("expected updated msgs")
	}
	last := updated[len(updated)-1]
	if !strings.Contains(last.Content, "WITHHELD") {
		t.Errorf("expected WITHHELD guard; got %q", last.Content[:min(100, len(last.Content))])
	}
}

func TestApplyOversizedGuardAcceptLargeOutputFalseReturnsWithheld(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	bigResult := strings.Repeat("x", 200000)
	ag.WithTool(agentcore.ToolFunc{
		N:  "big_tool",
		D:  "big",
		P:  map[string]any{"type": "object"},
		Fn: func(context.Context, string) (string, error) { return bigResult, nil },
	})
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "big_tool", Arguments: `{"accept_large_output":false}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
	}
	_, updated, _ := stream.AgentExecuteToolsWithChanForTest(ag, []llm.ToolCall{tc}, msgs)
	last := updated[len(updated)-1]
	if !strings.Contains(last.Content, "WITHHELD") {
		t.Errorf("expected WITHHELD; got %q", last.Content[:min(100, len(last.Content))])
	}
}

func TestLastUserMessageIDNoUserMessageReturnsEmpty(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "", []llm.Message{
		{Role: "assistant", Content: "no-user-msg"},
	})
	for range ch {
	}
}

func streamWithUsage(prompt, completion, total int) []llm.StreamChunk {
	return []llm.StreamChunk{
		streamtest.TextChunk("ok"),
		streamtest.FinishChunkWithUsage(llm.FinishReasonStop, prompt, completion),
		streamtest.NoOpChunk(),
	}
}

func TestAgentObservedInputTokensEmitsObservedField(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")
	t.Setenv("AWP_SESSION_LOG_PATH", "")

	core := &fakeCore{streamChunks: streamWithUsage(5000, 100, 5100)}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}

	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "x2", nil)
	for ev := range ch {
		_ = ev
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.v3.ndjson"))
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if bytes.Contains(data, []byte(`"observed_input_tokens":5000`)) {
			return
		}
	}
	t.Errorf("expected observed_input_tokens=5000 in some session file")
}

func TestWriteEventForTestSkipsWhenLogBufNil(t *testing.T) {
	ag := &agentcore.Agent{LogWriter: io.Discard}
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})
}

func TestWriteEventForTestNoOpOnNilWriter(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})
}

func TestWriteCustomWithInvalidRawMessageReturnsError(t *testing.T) {
	ag, _ := newTestAgentWithBuf()
	ag.SetLogFileOpenedForTest(true)
	invalid := json.RawMessage(`{not-valid-json`)
	if err := ag.WriteCustom("p", "t", invalid); err == nil {
		t.Error("expected error for invalid RawMessage")
	}
}

func TestWriteCustomMessageWithInvalidRawMessageReturnsError(t *testing.T) {
	ag, _ := newTestAgentWithBuf()
	ag.SetLogFileOpenedForTest(true)
	invalid := json.RawMessage(`{not-valid-json`)
	if err := ag.WriteCustomMessage("p", "t", "c", invalid); err == nil {
		t.Error("expected error for invalid RawMessage")
	}
}

func TestWriteMessageWithEmptyIDAutoGeneratesID(t *testing.T) {
	ag, buf := newTestAgentWithBuf()
	ag.SetLogFileOpenedForTest(true)
	if err := ag.WriteMessage(json_rpc.MessageEvent{
		Message: json_rpc.Message{Role: "user", Content: []json_rpc.MessageContentPart{{Type: "text", Text: "x"}}},
	}); err != nil {
		t.Fatal(err)
	}
	ag.FlushLogForTest()
	if !strings.Contains(buf.String(), `"id":"`) {
		t.Errorf("expected id field in output; got %q", buf.String())
	}
}

func TestWriteMessageWithEmptyTimestampAutoGeneratesTimestamp(t *testing.T) {
	ag, buf := newTestAgentWithBuf()
	ag.SetLogFileOpenedForTest(true)
	if err := ag.WriteMessage(json_rpc.MessageEvent{
		ID:      "fixed-id",
		Message: json_rpc.Message{Role: "user", Content: []json_rpc.MessageContentPart{{Type: "text", Text: "x"}}},
	}); err != nil {
		t.Fatal(err)
	}
	ag.FlushLogForTest()
	if !strings.Contains(buf.String(), `"timestamp":`) {
		t.Errorf("expected timestamp field; got %q", buf.String())
	}
}

func TestWriteJSONLineFailsOnUnmarshalableValue(t *testing.T) {
	ag, _ := newTestAgentWithBuf()
	ag.SetLogFileOpenedForTest(true)
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})

	ch := make(chan int)
	defer close(ch)
	err := ag.WriteCustom("p", "t", json.RawMessage(fmt.Sprintf(`{"ch":%v}`, ch)))
	if err == nil {
		t.Error("expected error for chan value")
	}
}

func TestWriteCompactionLockedWithNoWriterReturnsImmediately(t *testing.T) {
	ag := &agentcore.Agent{}
	ag.WriteEventForTest(0, agentcore.Event{Category: agentcore.EventUserMessage, Content: "x"})
}

func TestWriteMessageFailsOnUnmarshalableMsg(t *testing.T) {
	type bad struct {
		C chan int `json:"c"`
	}
	ch := make(chan int)
	defer close(ch)
	_, err := json.Marshal(bad{C: ch})
	if err == nil {
		t.Fatal("expected json.Marshal to fail for chan field")
	}
}

func TestWriteMessageFailsOnInvalidRawMessageArg(t *testing.T) {
	ag, _ := newTestAgentWithBuf()
	ag.SetLogFileOpenedForTest(true)
	err := ag.WriteMessage(json_rpc.MessageEvent{
		ID: "x",
		Message: json_rpc.Message{
			Role: "assistant",
			Content: []json_rpc.MessageContentPart{{
				Type:      "toolCall",
				ID:        "x",
				Name:      "ls",
				Intent:    "i",
				Arguments: json.RawMessage(`{not-valid`),
			}},
		},
	})
	if err == nil {
		t.Fatal("expected error from json.Marshal failure")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write(p []byte) (int, error) {
	return 0, errors.New("broken-write")
}

func TestWriteMessageFailsOnBrokenWriter(t *testing.T) {
	ag := agentcore.NewAgentWithLogWriterForTest(brokenWriter{})
	ag.SetLogFileOpenedForTest(true)
	bigMsg := strings.Repeat("x", 5000)
	if err := ag.WriteMessage(json_rpc.MessageEvent{ID: "x", Message: json_rpc.Message{Role: "user", Content: []json_rpc.MessageContentPart{{Type: "text", Text: bigMsg}}}}); err == nil {
		t.Error("expected error from broken writer")
	}
}

func TestWriteCustomFailsOnBrokenWriter(t *testing.T) {
	ag := agentcore.NewAgentWithLogWriterForTest(brokenWriter{})
	ag.SetLogFileOpenedForTest(true)
	bigData := json.RawMessage(`"` + strings.Repeat("x", 5000) + `"`)
	if err := ag.WriteCustom("p", "t", bigData); err == nil {
		t.Error("expected error from broken writer")
	}
}

func TestWriteCustomMessageFailsOnBrokenWriter(t *testing.T) {
	ag := agentcore.NewAgentWithLogWriterForTest(brokenWriter{})
	ag.SetLogFileOpenedForTest(true)
	if err := ag.WriteCustomMessage("p", "t", strings.Repeat("x", 5000), nil); err == nil {
		t.Error("expected error from broken writer")
	}
}

func TestWriteHeaderLockedFailsOnBrokenWriter(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core)
	ag.WithLogWriter(brokenWriter{})
	ag.WithSessionID("test-id")
	ag.WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	ag.SetSystemPrompts(strings.Repeat("a", 5000))
	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}
}

func TestWriteTurnStartLockedFailsOnBrokenWriter(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core)
	ag.WithLogWriter(brokenWriter{})
	ag.WithSessionID("test-id")
	ag.WithModel(llm.Model{ID: "m", MaxContextTokens: 128000})
	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}
}
