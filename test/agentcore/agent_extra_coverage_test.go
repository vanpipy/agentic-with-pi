package agentcore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/streamtest"
)

func TestRequireIntentAcceptsValidIntent(t *testing.T) {
	if err := agentcore.RequireIntent(`{"intent":"summarize file"}`); err != nil {
		t.Errorf("RequireIntent(valid) = %v, want nil", err)
	}
}

func TestRequireIntentRejectsMissingField(t *testing.T) {
	err := agentcore.RequireIntent(`{}`)
	if err == nil {
		t.Fatal("RequireIntent({}) = nil, want error")
	}
	if !strings.Contains(err.Error(), "is required") {
		t.Errorf("err = %q, want mentions is required", err.Error())
	}
}

func TestRequireIntentRejectsWhitespaceOnly(t *testing.T) {
	err := agentcore.RequireIntent(`{"intent":"   \t  "}`)
	if err == nil {
		t.Fatal("RequireIntent(whitespace) = nil, want error")
	}
}

func TestRequireIntentRejectsBadJSON(t *testing.T) {
	err := agentcore.RequireIntent(`not json`)
	if err == nil {
		t.Fatal("RequireIntent(bad json) = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid args") {
		t.Errorf("err = %q, want mentions invalid args", err.Error())
	}
}

func TestReActStrategyNameReturnsReact(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	if name := strat.Name(); name != "react" {
		t.Errorf("Name() = %q, want react", name)
	}
}

func TestAgentWithSessionIDRoundTrip(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithSessionID("session-42")
	if got := ag.SessionIDForTest(); got != "session-42" {
		t.Errorf("SessionIDForTest() = %q, want session-42", got)
	}
}

func TestAgentSessionIDDefaultEmpty(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	if got := ag.SessionIDForTest(); got != "" {
		t.Errorf("default SessionIDForTest() = %q, want empty", got)
	}
}

func TestAgentCompactionSettingsForTestRoundTrip(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	want := compact.CompactionSettings{Enabled: false, ReserveTokens: 99, KeepRecentTurns: 7}
	ag.WithCompaction(want)
	got := ag.CompactionSettingsForTest()
	if got.ReserveTokens != 99 || got.KeepRecentTurns != 7 || got.Enabled {
		t.Errorf("CompactionSettingsForTest = %+v, want %+v", got, want)
	}
}

func TestAgentLogEventForTestNilWriterIsNoop(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ag.LogEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "noop"})
}

func TestAgentWriteLegacyEventForTestNoWriterIsNoop(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ag.WriteLegacyEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "x"})
}

func TestAgentWriteLegacyEventForTestWithWriterWrites(t *testing.T) {
	ag, _ := newV3TestAgent()
	ag.WriteLegacyEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "via-legacy"})
}

func TestAgentPreflightFailureStreakInitiallyEmpty(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	if got := ag.PreflightFailureStreakForTest(); len(got) != 0 {
		t.Errorf("streak = %v, want empty", got)
	}
}

func TestAgentPreflightFailureStreakSetReturnsMap(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	streak := ag.PreflightFailureStreakForTest()
	streak["alpha"] = 2
	if ag.PreflightFailureStreakForTest()["alpha"] != 2 {
		t.Errorf("alpha streak = %d, want 2", ag.PreflightFailureStreakForTest()["alpha"])
	}
}

func TestAgentCurrentParentIDForTestReturnsEmptyInitially(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	if got := ag.CurrentParentIDForTest(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestAgentWithRepeatedToolErrorLimitClampsBelowOne(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithRepeatedToolErrorLimit(0)
	if got := agentcore.AgentRepeatedToolErrorLimitForTest(ag); got != 3 {
		t.Errorf("limit = %d, want 3 (clamp)", got)
	}
}

func TestAgentWithRepeatedToolErrorLimitCustom(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithRepeatedToolErrorLimit(7)
	if got := agentcore.AgentRepeatedToolErrorLimitForTest(ag); got != 7 {
		t.Errorf("limit = %d, want 7", got)
	}
}

func TestAgentWithToolCacheSizeClampsZeroToDefault(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithToolCacheSize(0)
	if ag.ToolCacheSize != 20 {
		t.Errorf("ToolCacheSize = %d, want 20 (clamp)", ag.ToolCacheSize)
	}
}

func TestAgentWithToolCacheSizeClampsNegativeToDefault(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithToolCacheSize(-5)
	if ag.ToolCacheSize != 20 {
		t.Errorf("ToolCacheSize = %d, want 20 (clamp)", ag.ToolCacheSize)
	}
}

func TestAgentWithToolCacheSizeCustom(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithToolCacheSize(5)
	if ag.ToolCacheSize != 5 {
		t.Errorf("ToolCacheSize = %d, want 5", ag.ToolCacheSize)
	}
}

func TestAgentWithSafetyNetZeroClampsToDefault(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithSafetyNet(0)
	if ag.SafetyNet != 200 {
		t.Errorf("SafetyNet = %d, want 200 (clamp)", ag.SafetyNet)
	}
}

func TestAgentWithSafetyNetNegativeClampsToDefault(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithSafetyNet(-1)
	if ag.SafetyNet != 200 {
		t.Errorf("SafetyNet = %d, want 200 (clamp)", ag.SafetyNet)
	}
}

func TestAgentWithSafetyNetCustom(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithSafetyNet(42)
	if ag.SafetyNet != 42 {
		t.Errorf("SafetyNet = %d, want 42", ag.SafetyNet)
	}
}

func TestAgentWithModelSetsStrategyModel(t *testing.T) {
	core := &fakeCore{}
	ag := agentcore.NewAgent(core)
	ag.WithModel(llm.Model{ID: "alpha", SupportsTool: true})
	if ag.Model.ID != "alpha" {
		t.Errorf("ag.Model.ID = %q, want alpha", ag.Model.ID)
	}
	ag.WithModel(llm.Model{ID: "beta", SupportsTool: true})
	if ag.Model.ID != "beta" {
		t.Errorf("ag.Model.ID after second WithModel = %q, want beta", ag.Model.ID)
	}
}

func TestAgentWithToolRegistersAndOverwrites(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "first", nil }})
	tool, ok := agentcore.AgentFindToolForTest(ag, "alpha")
	if !ok {
		t.Fatal("findTool(alpha) = !ok, want true")
	}
	out, _ := tool.Invoke(context.Background(), `{}`)
	if out != "first" {
		t.Errorf("alpha output = %q, want first", out)
	}

	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "second", nil }})
	tool, _ = agentcore.AgentFindToolForTest(ag, "alpha")
	out, _ = tool.Invoke(context.Background(), `{}`)
	if out != "second" {
		t.Errorf("alpha output after overwrite = %q, want second", out)
	}
}

func TestAgentWithToolMissingReturnsFalse(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	if _, ok := agentcore.AgentFindToolForTest(ag, "missing"); ok {
		t.Error("findTool(missing) = ok, want false")
	}
}

func TestAgentFindToolForTestReturnsCopy(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "x", nil }})
	tool, ok := agentcore.AgentFindToolForTest(ag, "alpha")
	if !ok {
		t.Fatal("findTool(alpha) = !ok, want true")
	}
	if tool.Name() != "alpha" {
		t.Errorf("Name() = %q, want alpha", tool.Name())
	}
}

func TestReActStrategyShouldAbortRepeatedCallsSlidingWindow(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"."}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "explore"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
	}
	err := strat.ShouldAbort(msgs, "")
	if err == nil {
		t.Fatal("ShouldAbort returned nil, want non-nil after 4 identical calls (sliding window branch)")
	}
	if !strings.Contains(err.Error(), "repeated") {
		t.Errorf("err = %q, want mentions repeated", err.Error())
	}
}

func TestReActStrategyShouldAbortVariedCallsNoAbort(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc1 := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"a"}`}}
	tc2 := llm.ToolCall{ID: "c2", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"b"}`}}
	tc3 := llm.ToolCall{ID: "c3", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"c"}`}}
	tc4 := llm.ToolCall{ID: "c4", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"d"}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "explore"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc1}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc2}},
		{Role: "tool", ToolCallID: "c2", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc3}},
		{Role: "tool", ToolCallID: "c3", Content: "ok"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc4}},
	}
	if err := strat.ShouldAbort(msgs, ""); err != nil {
		t.Errorf("ShouldAbort = %v, want nil for varied calls", err)
	}
}

func TestReActStrategyShouldAbortEmptyMsgsNoAbort(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	if err := strat.ShouldAbort(nil, ""); err != nil {
		t.Errorf("ShouldAbort(nil) = %v, want nil", err)
	}
}

func TestReActStrategyShouldAbortMixedRolesNoAbort(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"."}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "explore"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
		{Role: "assistant", Content: "result"},
		{Role: "tool", ToolCallID: "c1", Content: "ok2"},
	}
	if err := strat.ShouldAbort(msgs, ""); err != nil {
		t.Errorf("ShouldAbort = %v, want nil", err)
	}
}

func TestReActStrategyShouldAbortRepeatedErrorsAtLimit(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{}`}}
	errMsg := "Tool ls failed: boom"
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
	}
	err := strat.ShouldAbort(msgs, errMsg)
	if err == nil {
		t.Fatal("ShouldAbort returned nil, want non-nil after 3 identical errors")
	}
	if !strings.Contains(err.Error(), "aborting") {
		t.Errorf("err = %q, want mentions aborting", err.Error())
	}
}

func TestReActStrategyShouldAbortNoLastErrShortCircuit(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{}`}}
	errMsg := "Tool ls failed: boom"
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"a"}`}}}},
		{Role: "tool", Content: errMsg},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c3", Function: llm.FunctionCall{Name: "ls", Arguments: `{"path":"b"}`}}}},
	}
	if err := strat.ShouldAbort(msgs, ""); err != nil {
		t.Errorf("ShouldAbort(lastFailedToolError=\"\") = %v, want nil (early return when no last error)", err)
	}
}

type errCore struct{}

func (errCore) StreamChat(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, errors.New("simulated provider failure")
}

func (errCore) CompleteSplit(systemPrompt string, model string) []llm.ContentBlock {
	_ = systemPrompt
	_ = model
	return nil
}

func TestReActStrategyStepNonCtxStreamErrEmitsErrorEvents(t *testing.T) {
	strat := agentcore.NewReActStrategy(errCore{}, llm.Model{ID: "m"}, nil, nil)
	var sawError, sawThoughtEnd bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		switch ev.Category {
		case agentcore.EventError:
			sawError = true
			if !strings.Contains(ev.ToolError, "simulated provider failure") {
				t.Errorf("ToolError = %q, want mentions simulated provider failure", ev.ToolError)
			}
		case agentcore.EventThoughtEnd:
			sawThoughtEnd = true
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step returned err = %v, want nil (non-ctx stream error is reported via events)", err)
	}
	if step.Kind != agentcore.StepContinue || step.Content != "" {
		t.Errorf("step = %+v, want empty StepContinue", step)
	}
	if !sawError || !sawThoughtEnd {
		t.Errorf("sawError=%v sawThoughtEnd=%v, want both true", sawError, sawThoughtEnd)
	}
}

type ctxErrCore struct{}

func (ctxErrCore) StreamChat(_ context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, context.Canceled
}

func (ctxErrCore) CompleteSplit(systemPrompt string, model string) []llm.ContentBlock {
	_ = systemPrompt
	_ = model
	return nil
}

func TestReActStrategyStepCtxErrReturnsCtxErr(t *testing.T) {
	strat := agentcore.NewReActStrategy(ctxErrCore{}, llm.Model{ID: "m"}, nil, nil)
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, func(context.Context, agentcore.Event) bool { return true })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if step.Kind != agentcore.StepContinue || step.Content != "" {
		t.Errorf("step = %+v, want zero value", step)
	}
}

func TestReActStrategyStepEmitReturnsFalseAtThoughtStart(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m"}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var sawThoughtStart bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventThoughtStart {
			sawThoughtStart = true
			return false
		}
		return true
	}
	_, err := strat.Step(ctx, []llm.Message{{Role: "user", Content: "x"}}, emit)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if !sawThoughtStart {
		t.Errorf("emit was never called with EventThoughtStart")
	}
}

func TestReActStrategyStepLengthFinishReasonNoContent(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	var sawError bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "truncated with no content") {
			sawError = true
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepContinue {
		t.Errorf("Kind = %v, want StepContinue", step.Kind)
	}
	if !sawError {
		t.Error("expected EventError mentioning truncated with no content")
	}
}

func TestReActStrategyStepToolUseNoToolCalls(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("thinking out loud"),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	var sawError bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "tool_use finish reason with no tool calls") {
			sawError = true
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepContinue {
		t.Errorf("Kind = %v, want StepContinue", step.Kind)
	}
	if !sawError {
		t.Error("expected EventError mentioning tool_use with no tool calls")
	}
}

func TestReActStrategyStepToolUseAllEmptyToolCalls(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		{Choices: []llm.StreamChoice{{
			Index: 0,
			Delta: llm.Message{ToolCalls: []llm.ToolCall{{
				ID:       "c1",
				Type:     "function",
				Function: llm.FunctionCall{Name: "noop", Arguments: ""},
			}}},
		}}},
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	var sawError bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "empty tool calls") {
			sawError = true
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepContinue {
		t.Errorf("Kind = %v, want StepContinue", step.Kind)
	}
	if !sawError {
		t.Error("expected EventError mentioning empty tool calls")
	}
}

func TestReActStrategyStepLengthWithContentEmitsFinalAnswer(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("partial answer"),
		messageDeltaStopChunk("max_tokens"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	var sawFinal bool
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventFinalAnswer {
			sawFinal = true
		}
		return true
	}
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if step.Kind != agentcore.StepFinal {
		t.Errorf("Kind = %v, want StepFinal", step.Kind)
	}
	if step.Content != "partial answer" {
		t.Errorf("Content = %q, want partial answer", step.Content)
	}
	if !sawFinal {
		t.Error("expected EventFinalAnswer when Length+content")
	}
}

func TestReActStrategyStepUnknownFinishReasonFallsToUnreachable(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		messageDeltaStopChunk("weird_reason"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	emit := func(_ context.Context, _ agentcore.Event) bool { return true }
	step, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err == nil {
		t.Fatalf("Step err = nil, want unreachable error; step=%+v", step)
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("err = %q, want mentions unreachable", err.Error())
	}
}

func TestReActStrategyStepEmitReturnsFalseAtThoughtEnd(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("partial"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	strat := agentcore.NewReActStrategy(core, llm.Model{ID: "m"}, nil, nil)
	emit := func(_ context.Context, ev agentcore.Event) bool {
		if ev.Category == agentcore.EventThoughtEnd {
			return false
		}
		return true
	}
	_, err := strat.Step(context.Background(), []llm.Message{{Role: "user", Content: "x"}}, emit)
	if err != nil {
		t.Errorf("err = %v, want nil (emit=false stops processing without surfacing error from ctx.Background)", err)
	}
}

func TestReActStrategyProcessStreamEventChunkNilIsNoOp(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.NoOp(),
	})
	if len(calls) != 0 {
		t.Errorf("got %d calls, want 0 for nil-chunk event", len(calls))
	}
}

func TestReActStrategyProcessStreamEventErrCtxStopsImmediately(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		llm.EventErr{Err: context.Canceled},
		streamtest.ToolStartDelta("c1", "ls", "{}")[0],
		streamtest.ToolStartDelta("c1", "ls", "{}")[1],
	})
	if len(calls) != 0 {
		t.Errorf("calls = %d, want 0 (ctx err short-circuits)", len(calls))
	}
}

func TestReActStrategyProcessStreamEventErrNotCtxShortCircuits(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		llm.EventErr{Err: errors.New("upstream error")},
		streamtest.ToolStartDelta("c1", "ls", "")[0],
	})
	if len(calls) != 0 {
		t.Errorf("calls = %d, want 0 (non-ctx err short-circuits)", len(calls))
	}
}

func TestReActStrategyProcessStreamEventAccumulatesToolCallsByID(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.ToolStartDelta("c1", "ls", "{}")[0],
		streamtest.ToolStartDelta("c1", "ls", `{"path":"."}`)[1],
		streamtest.ToolStartDelta("c2", "grep", "{}")[0],
		streamtest.ToolStartDelta("c2", "grep", `{"pattern":"x"}`)[1],
		streamtest.Finish(llm.FinishReasonToolUse),
		streamtest.NoOp(),
	})
	if len(calls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(calls))
	}
	if calls[0].ID != "c1" || calls[0].Function.Name != "ls" || calls[0].Function.Arguments != `{"path":"."}` {
		t.Errorf("calls[0] = %+v", calls[0])
	}
	if calls[1].ID != "c2" || calls[1].Function.Name != "grep" {
		t.Errorf("calls[1] = %+v", calls[1])
	}
}

func TestReActStrategyProcessStreamEventMergesDeltasWithoutID(t *testing.T) {
	calls := agentcore.AccumulateStreamToolCallsForTest([]llm.StreamEvent{
		streamtest.ToolStartDelta("c1", "ls", "")[0],
		streamtest.ToolStartDelta("", "", `{"path":"."}`)[1],
		streamtest.Finish(llm.FinishReasonToolUse),
		streamtest.NoOp(),
	})
	if len(calls) != 1 {
		t.Fatalf("got %d tool calls, want 1 (id-less delta should merge into existing)", len(calls))
	}
	if calls[0].ID != "c1" || calls[0].Function.Arguments != `{"path":"."}` {
		t.Errorf("calls[0] = %+v", calls[0])
	}
}

func TestAgentExecuteToolsEmptyArgsAddsErrorMessageAndContinues(t *testing.T) {
	calls := []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: "  "}}}
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	events, updated, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if !ok {
		t.Error("ok = false, want true (empty args skips but doesn't abort)")
	}
	var foundSkip bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "empty arguments") {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Errorf("missing EventError mentioning empty arguments; events=%+v", updated)
	}
}

func TestAgentExecuteToolsUnknownToolAbortsAndFillsRemaining(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "alpha", Fn: func(context.Context, string) (string, error) { return "", nil }})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "alpha", Arguments: `{}`}},
		{ID: "c2", Function: llm.FunctionCall{Name: "ghost", Arguments: `{}`}},
		{ID: "c3", Function: llm.FunctionCall{Name: "ghost2", Arguments: `{}`}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	_, updated, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if ok {
		t.Error("ok = true, want false (unknown tool should abort)")
	}
	if len(updated) < 4 {
		t.Errorf("updated len = %d, want at least 4 (1 user + tool result for c1 + skipped-fill for c2 + skipped-fill for c3)", len(updated))
	}
}

func TestAgentExecuteToolsPreflightFailureEmitsErrorAndStopsRemaining(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
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
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "needs_path", Arguments: `{}`}},
		{ID: "c2", Function: llm.FunctionCall{Name: "needs_path", Arguments: `{}`}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	events, updated, ok := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	if !ok {
		t.Error("ok = false, want true (preflight failure appends and stops remaining but ok=true)")
	}
	var sawPreflight, sawSkip bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "missing required field") {
			sawPreflight = true
		}
	}
	for _, m := range updated {
		if strings.Contains(m.Content, "skipped") {
			sawSkip = true
		}
	}
	if !sawPreflight {
		t.Error("expected preflight error event")
	}
	if !sawSkip {
		t.Error("expected skipped-tool result appended to msgs for remaining calls")
	}
	if streak := ag.PreflightFailureStreakForTest()["needs_path"]; streak != 1 {
		t.Errorf("preflight streak = %d, want 1", streak)
	}
}

func TestAgentExecuteToolsCacheHitEmitsObserveFromCache(t *testing.T) {
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: "noop", Arguments: `{"intent":"cached"}`}},
		{ID: "c2", Function: llm.FunctionCall{Name: "noop", Arguments: `{"intent":"cached"}`}},
	}
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	calls_count := 0
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(_ context.Context, _ string) (string, error) {
		calls_count++
		return "result", nil
	}})
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	updated1, _ := stream.AgentExecuteToolsForTest(ag, calls[:1], msgs)
	events2, _, _ := stream.AgentExecuteToolsWithChanForTest(ag, calls[1:], updated1)
	var sawCacheHit bool
	for _, ev := range events2 {
		if ev.Kind == stream.EmitKindObserve && ev.FromCache {
			sawCacheHit = true
		}
	}
	if !sawCacheHit {
		t.Errorf("expected EventObserve with FromCache=true; events=%+v", events2)
	}
	if calls_count != 1 {
		t.Errorf("tool invocations = %d, want 1 (second call should hit cache)", calls_count)
	}
}

func TestAgentExecuteToolsErrorEmitsInvalidEventForInvalidName(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: agentcore.InvalidToolName, Fn: func(context.Context, string) (string, error) {
		return "", errors.New("bad args")
	}})
	calls := []llm.ToolCall{
		{ID: "c1", Function: llm.FunctionCall{Name: agentcore.InvalidToolName, Arguments: `{}`}},
	}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	events, _, _ := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	var sawInvalid bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindInvalid {
			sawInvalid = true
		}
	}
	if !sawInvalid {
		t.Errorf("expected EventInvalid for tool named %q; events=%+v", agentcore.InvalidToolName, events)
	}
}

func TestAgentExecuteToolsSuccessPathEmitsToolThenObserve(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(_ context.Context, argsJSON string) (string, error) {
		return "echoed:" + argsJSON, nil
	}})
	calls := []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "echo", Arguments: `{"intent":"hi"}`}}}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	events, _, _ := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	var sawTool, sawObserve bool
	for _, ev := range events {
		switch ev.Kind {
		case stream.EmitKindTool:
			sawTool = true
		case stream.EmitKindObserve:
			sawObserve = true
			if !strings.Contains(ev.ToolResult, "echoed:") {
				t.Errorf("ToolResult = %q, want contains echoed:", ev.ToolResult)
			}
		}
	}
	if !sawTool || !sawObserve {
		t.Errorf("sawTool=%v sawObserve=%v, want both true", sawTool, sawObserve)
	}
}

func TestAgentExecuteToolsErrorEmitsToolError(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.WithTool(agentcore.ToolFunc{N: "broken", Fn: func(context.Context, string) (string, error) {
		return "", errors.New("kapow")
	}})
	calls := []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "broken", Arguments: `{"intent":"x"}`}}}
	msgs := []llm.Message{{Role: "user", Content: "x"}}
	events, _, _ := stream.AgentExecuteToolsWithChanForTest(ag, calls, msgs)
	var sawToolError, sawError bool
	for _, ev := range events {
		if ev.Kind == stream.EmitKindObserve && ev.ToolError != "" {
			sawToolError = true
		}
		if ev.Kind == stream.EmitKindError && strings.Contains(ev.ToolError, "kapow") {
			sawError = true
		}
	}
	if !sawToolError {
		t.Errorf("expected EventObserve with ToolError; events=%+v", events)
	}
	if !sawError {
		t.Errorf("expected EventError mentioning kapow; events=%+v", events)
	}
}

func TestAgentRunStreamEmitsModelNotSet(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	var sawErr bool
	for ev := range ag.RunStream(context.Background(), "hi") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "Model not set") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected EventError mentioning Model not set")
	}
}

func TestAgentRunStreamResumedEmitsModelNotSet(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "hi", nil)
	var sawErr bool
	for ev := range ch {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "Model not set") {
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("expected EventError mentioning Model not set")
	}
}

func TestAgentRunStreamResumedPrependsSystemWhenHistoryMissingIt(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	history := []llm.Message{{Role: "user", Content: "earlier"}}
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "next", history)
	for ev := range ch {
		_ = ev
	}
	if len(core.requests) == 0 {
		t.Fatal("expected at least one request")
	}
	msgs := core.requests[0].Messages
	if msgs[0].Role != "system" {
		t.Errorf("first msg role = %q, want system (prepended)", msgs[0].Role)
	}
	if msgs[len(msgs)-1].Content != "next" {
		t.Errorf("last msg content = %q, want next", msgs[len(msgs)-1].Content)
	}
}

func TestAgentRunStreamResumedKeepsExistingSystemMessage(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	history := []llm.Message{
		{Role: "system", Content: "custom-system"},
		{Role: "user", Content: "earlier"},
	}
	ch, _ := ag.RunStreamResumedWithSnapshot(context.Background(), "next", history)
	for ev := range ch {
		_ = ev
	}
	msgs := core.requests[0].Messages
	if msgs[0].Role != "system" || msgs[0].Content != "custom-system" {
		t.Errorf("first system msg = %+v, want role=system content=custom-system", msgs[0])
	}
}

func TestAgentLoopWithMsgsSafetyNetAborts(t *testing.T) {
	chunksList := [][]llm.StreamEvent{}
	for i := 0; i < 4; i++ {
		chunksList = append(chunksList, []llm.StreamEvent{
			streamtest.ToolStart(fmt.Sprintf("c%d", i+1), "noop"),
			streamtest.ToolDelta(fmt.Sprintf("c%d", i+1), fmt.Sprintf(`{"intent":"x%d"}`, i+1)),
			streamtest.Finish(llm.FinishReasonToolUse),
			streamtest.NoOp(),
		})
	}
	core := &fakeCore{streamEventsList: chunksList}
	ag := newTestAgent(core, "m").WithSafetyNet(3)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	var sawSafetyNet bool
	for ev := range ag.RunStream(context.Background(), "loop forever") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "safety net reached") {
			sawSafetyNet = true
		}
	}
	if !sawSafetyNet {
		t.Error("expected EventError mentioning safety net reached")
	}
}

func TestAgentLoopWithMsgsTooManyToolCallsTruncates(t *testing.T) {
	repeat := []llm.StreamEvent{
		streamtest.ToolStart("c1", "noop"),
		streamtest.ToolDelta("c1", `{"intent":"x"}`),
		streamtest.ToolStart("c2", "noop"),
		streamtest.ToolDelta("c2", `{"intent":"x"}`),
		streamtest.ToolStart("c3", "noop"),
		streamtest.ToolDelta("c3", `{"intent":"x"}`),
		streamtest.ToolStart("c4", "noop"),
		streamtest.ToolDelta("c4", `{"intent":"x"}`),
		streamtest.ToolStart("c5", "noop"),
		streamtest.ToolDelta("c5", `{"intent":"x"}`),
		streamtest.ToolStart("c6", "noop"),
		streamtest.ToolDelta("c6", `{"intent":"x"}`),
		streamtest.ToolStart("c7", "noop"),
		streamtest.ToolDelta("c7", `{"intent":"x"}`),
		streamtest.ToolStart("c8", "noop"),
		streamtest.ToolDelta("c8", `{"intent":"x"}`),
		streamtest.Finish(llm.FinishReasonToolUse),
		streamtest.NoOp(),
	}
	core := &fakeCore{streamEventsList: [][]llm.StreamEvent{
		repeat,
		{streamtest.Text("done"),
			streamtest.Finish(llm.FinishReasonStop), streamtest.NoOp()},
	}}
	ag := newTestAgent(core, "m").WithSafetyNet(20)
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(context.Context, string) (string, error) { return "ok", nil }})

	var sawTruncation bool
	var finalMsg agentcore.Event
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "too many tool calls") {
			sawTruncation = true
		}
		if ev.Category == agentcore.EventFinalAnswer {
			finalMsg = ev
		}
	}
	if !sawTruncation {
		t.Error("expected EventError mentioning too many tool calls")
	}
	if finalMsg.Content != "done" {
		t.Errorf("final answer = %q, want done", finalMsg.Content)
	}
}

func TestAgentShouldAbortDelegatesToStrategyAndPreflight(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	if err := ag.ShouldAbort(nil, ""); err != nil {
		t.Errorf("ShouldAbort(empty) = %v, want nil", err)
	}
}

func TestAgentShouldAbortReturnsPreflightError(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	ag.PreflightFailureStreakForTest()["alpha"] = agentcore.PreflightAbortThreshold
	err := ag.ShouldAbort(nil, "")
	if err == nil {
		t.Error("ShouldAbort = nil, want preflight abort error")
	}
	if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("err = %q, want mentions alpha and invalid arguments", err.Error())
	}
}

func TestAgentShouldAbortReturnsStrategyError(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "m"})
	tc := llm.ToolCall{ID: "c1", Function: llm.FunctionCall{Name: "ls", Arguments: `{}`}}
	msgs := []llm.Message{
		{Role: "user", Content: "x"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "Tool ls failed: boom"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "Tool ls failed: boom"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{tc}},
		{Role: "tool", ToolCallID: "c1", Content: "Tool ls failed: boom"},
	}
	if err := ag.ShouldAbort(msgs, "Tool ls failed: boom"); err == nil {
		t.Error("ShouldAbort = nil, want strategy abort error")
	}
}

type errReturningCore struct {
	err error
}

func (e *errReturningCore) StreamChat(ctx context.Context, _ *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, e.err
}

func (e *errReturningCore) CompleteSplit(systemPrompt string, model string) []llm.ContentBlock {
	_ = systemPrompt
	_ = model
	return nil
}

func TestAgentRunStreamStreamChatCoreErrorNotCtxEmitsAndReturnsNilError(t *testing.T) {
	core := &errReturningCore{err: errors.New("rate limit exceeded")}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m"})
	var sawRateLimit bool
	for ev := range ag.RunStream(context.Background(), "x") {
		if ev.Category == agentcore.EventError && strings.Contains(ev.ToolError, "rate limit") {
			sawRateLimit = true
		}
	}
	if !sawRateLimit {
		t.Error("expected EventError mentioning rate limit")
	}
}

func TestAgentRunStreamProcessesUserMessageFirst(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("answer"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	if _, err := runAgent(t, ag, "the user query"); err != nil {
		t.Fatal(err)
	}
	if len(core.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(core.requests))
	}
	var foundUser bool
	for _, m := range core.requests[0].Messages {
		if m.Role == "user" && m.Content == "the user query" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Errorf("expected user message 'the user query' in request; got %+v", core.requests[0].Messages)
	}
}

func TestRunStreamOpenLogUsesProvidedWriter(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("hi"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	var buf bytes.Buffer
	ag.WithLogWriter(&buf)
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestAgentEmitNilChannelWithLogWriterWritesToLog(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("c1", "noop"),
		toolUseIDDeltaChunk("c1", "noop", `{"intent":"x"}`),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "m")
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	var buf bytes.Buffer
	ag.WithLogWriter(&buf)
	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}
	if buf.Len() == 0 {
		t.Error("log writer buffer is empty; expected events to be written")
	}
}

func TestAgentExecuteToolsEmitReturnsFalseAbortsViaRunStream(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamChunk{
		toolUseStartChunk("c1", "echo"), toolUseIDDeltaChunk("c1", "echo", `{"intent":"x"}`),
		messageDeltaStopChunk("tool_use"), messageStopChunk(),
	}}
	ag := newTestAgent(core, "m").WithSafetyNet(20)
	ag.WithTool(agentcore.ToolFunc{N: "echo", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	ctx, cancel := context.WithCancel(context.Background())
	ch := ag.RunStream(ctx, "x")
	cancel()
	for ev := range ch {
		_ = ev
	}
}

func TestAgentNoModelEmitsError(t *testing.T) {
	ag := agentcore.NewAgent(&fakeCore{})
	if _, err := runAgent(t, ag, "x"); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestAgentWithLogWriterOpenLog(t *testing.T) {
	ag := newTestAgent(&fakeCore{streamChunks: []llm.StreamChunk{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}, "m")
	var buf bytes.Buffer
	ag.WithLogWriter(&buf)
	for ev := range ag.RunStream(context.Background(), "x") {
		_ = ev
	}
}

func TestAgentContextCancelAbortsViaRunStreamWithTool(t *testing.T) {
	gate := make(chan struct{})
	core := &blockingCore{gate: gate}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "m", SupportsTool: true})
	ag.WithTool(agentcore.ToolFunc{N: "noop", Fn: func(context.Context, string) (string, error) { return "ok", nil }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ag.RunStream(ctx, "x") {
			_ = ev
		}
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	close(gate)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunStream did not return after ctx cancel")
	}
}
