package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
)

var _ = llm.Message{}

func runAgentCaptureFirstToolError(t *testing.T, ag *agent.Agent, msg string) string {
	t.Helper()
	var firstToolError string
	for ev := range ag.RunStream(context.Background(), msg) {
		if ev.ToolError != "" && firstToolError == "" {
			firstToolError = ev.ToolError
		}
	}
	return firstToolError
}

func TestAgentUnknownToolErrorListsAvailableTools(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		toolUseStartChunk("call_1", "boom"),
		toolUseIDDeltaChunk("call_1", "boom", `{}`),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.ToolFunc{N: "bash", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agent.ToolFunc{N: "read", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})
	ag.WithTool(agent.ToolFunc{N: "write", Fn: func(_ context.Context, _ string) (string, error) { return "ok", nil }})

	got := runAgentCaptureFirstToolError(t, ag, "explore")
	if got == "" {
		t.Fatal("expected a tool error event")
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("error should name the offending tool 'boom': %q", got)
	}
	if !strings.Contains(got, "not found") && !strings.Contains(got, "unknown") && !strings.Contains(got, "not registered") {
		t.Errorf("error should say the tool is unknown/not found/not registered: %q", got)
	}
	for _, want := range []string{"bash", "read", "write"} {
		if !strings.Contains(got, want) {
			t.Errorf("error should list available tool %q: %q", want, got)
		}
	}
}

func TestAgentBatchContinuesAfterOneToolFails(t *testing.T) {
	readCalled := false
	bashCalled := false
	core := &fakeCore{streamChunksList: [][]llm.StreamEvent{
		{
			toolUseStartChunk("call_r", "read"),
			toolUseStartChunk("call_b", "bash"),
			toolUseIDDeltaChunk("call_r", "read", `{}`),
			toolUseIDDeltaChunk("call_b", "bash", `{"command":"pwd"}`),
			messageDeltaStopChunk("tool_use"),
			messageStopChunk(),
		},
		{
			textDeltaChunk("done"),
			messageDeltaStopChunk("end_turn"),
			messageStopChunk(),
		},
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.ToolFunc{N: "read", Fn: func(_ context.Context, _ string) (string, error) {
		readCalled = true
		return "", &fakeErr{"path is required"}
	}})
	ag.WithTool(agent.ToolFunc{N: "bash", Fn: func(_ context.Context, _ string) (string, error) {
		bashCalled = true
		return "ok", nil
	}})

	_, _ = runAgent(t, ag, "explore")

	if !readCalled {
		t.Fatal("read tool should have been invoked")
	}
	if !bashCalled {
		t.Errorf("bash tool should still have been invoked after read failed; one failure should not abort the batch")
	}
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

func TestAgentPreflightRejectsMissingRequiredField(t *testing.T) {
	called := false
	core := &fakeCore{streamChunks: []llm.StreamEvent{
		toolUseStartChunk("call_1", "bash"),
		toolUseIDDeltaChunk("call_1", "bash", `{"intent":"check"}`),
		messageDeltaStopChunk("tool_use"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agent.ToolFunc{
		N: "bash",
		D: "Run a shell command.",
		P: map[string]any{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "shell command"},
			},
		},
		Fn: func(_ context.Context, _ string) (string, error) {
			called = true
			return "ok", nil
		},
	})

	got := runAgentCaptureFirstToolError(t, ag, "explore")
	if got == "" {
		t.Fatal("expected preflight error")
	}
	if called {
		t.Errorf("tool should not have been invoked when required field missing")
	}
	if !strings.Contains(got, "command") {
		t.Errorf("preflight error should name the missing field 'command': %q", got)
	}
	if !strings.Contains(got, "required") {
		t.Errorf("preflight error should say 'required': %q", got)
	}
	if !strings.Contains(got, "bash") {
		t.Errorf("preflight error should name the tool 'bash': %q", got)
	}
}
