package agentcore_test

import (
	"context"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

func newAgentWithRequiredFieldTool(t *testing.T, toolName string) *agentcore.Agent {
	t.Helper()
	ag := newTestAgent(&fakeCore{}, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N: toolName,
		D: "Tool that requires a 'command' field.",
		P: map[string]any{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "shell command"},
			},
		},
		Fn: func(_ context.Context, _ string) (string, error) {
			return "ok", nil
		},
	})
	return ag
}

func lastToolPreflightError(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == "tool" && strings.Contains(m.Content, "missing required field") {
			return m.Content
		}
	}
	return ""
}

func TestAgent_PreflightRetryCounter_AbortsAfterTwoFailures(t *testing.T) {
	ag := newAgentWithRequiredFieldTool(t, "bash")

	msgs := []llm.Message{{Role: "user", Content: "explore"}}

	for turn := 1; turn <= 2; turn++ {
		calls := []llm.ToolCall{{
			ID:       "c" + string(rune('0'+turn)),
			Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"x"}`},
		}}
		msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: calls})
		msgs, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)
	}

	streak := ag.PreflightFailureStreakForTest()
	if got := streak["bash"]; got != 2 {
		t.Fatalf("preflight streak[bash] = %d, want 2 after two preflight failures", got)
	}

	lastFailed := lastToolPreflightError(msgs)
	if lastFailed == "" {
		t.Fatal("expected at least one preflight error in msgs")
	}
	err := ag.ShouldAbort(msgs, lastFailed)
	if err == nil {
		t.Fatal("ShouldAbort returned nil, want non-nil after two preflight failures on bash")
	}
	if !strings.Contains(err.Error(), "bash") {
		t.Errorf("abort message should name the failing tool 'bash': %q", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("abort message should mention 'invalid arguments': %q", err.Error())
	}
}

func TestAgent_PreflightRetryCounter_PerToolClearing(t *testing.T) {
	ag := newAgentWithRequiredFieldTool(t, "read")
	ag.WithTool(agentcore.ToolFunc{
		N: "write",
		D: "Tool that requires a 'content' field.",
		P: map[string]any{
			"type":     "object",
			"required": []string{"content"},
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "file content"},
			},
		},
		Fn: func(_ context.Context, _ string) (string, error) {
			return "wrote", nil
		},
	})

	msgs := []llm.Message{{Role: "user", Content: "explore"}}

	badRead := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "read", Arguments: `{"intent":"x"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: badRead})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, badRead, msgs)

	if got := ag.PreflightFailureStreakForTest()["read"]; got != 1 {
		t.Fatalf("after first read failure: read streak = %d, want 1", got)
	}

	goodWrite := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "write", Arguments: `{"content":"hi","intent":"w"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: goodWrite})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, goodWrite, msgs)

	if got := ag.PreflightFailureStreakForTest()["read"]; got != 1 {
		t.Errorf("after successful write: read streak = %d, want 1 (write success clears only write's own counter, not read)", got)
	}

	badRead2 := []llm.ToolCall{{
		ID:       "c3",
		Function: llm.FunctionCall{Name: "read", Arguments: `{"intent":"y"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: badRead2})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, badRead2, msgs)

	if got := ag.PreflightFailureStreakForTest()["read"]; got != 2 {
		t.Fatalf("after second read failure: read streak = %d, want 2 (write success did not reset read's counter)", got)
	}

	lastFailed := lastToolPreflightError(msgs)
	if lastFailed == "" {
		t.Fatal("expected preflight error in msgs")
	}
	if err := ag.ShouldAbort(msgs, lastFailed); err == nil {
		t.Fatal("ShouldAbort returned nil, want non-nil after two read preflight failures")
	}
}

func TestAgent_PreflightRetryCounter_DifferentToolsIndependent(t *testing.T) {
	ag := newAgentWithRequiredFieldTool(t, "read")
	ag.WithTool(agentcore.ToolFunc{
		N: "write",
		D: "Tool that requires a 'content' field.",
		P: map[string]any{
			"type":     "object",
			"required": []string{"content"},
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "file content"},
			},
		},
		Fn: func(_ context.Context, _ string) (string, error) {
			return "ok", nil
		},
	})

	msgs := []llm.Message{{Role: "user", Content: "explore"}}

	badRead := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "read", Arguments: `{"intent":"x"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: badRead})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, badRead, msgs)

	badWrite := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "write", Arguments: `{"intent":"y"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: badWrite})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, badWrite, msgs)

	streak := ag.PreflightFailureStreakForTest()
	if got := streak["read"]; got != 1 {
		t.Errorf("read streak = %d, want 1", got)
	}
	if got := streak["write"]; got != 1 {
		t.Errorf("write streak = %d, want 1 (independent of read)", got)
	}

	lastFailed := lastToolPreflightError(msgs)
	if lastFailed == "" {
		t.Fatal("expected preflight error in msgs")
	}
	if err := ag.ShouldAbort(msgs, lastFailed); err != nil {
		t.Fatalf("ShouldAbort returned %v, want nil (each tool at streak 1, threshold is 2)", err)
	}
}

func TestAgent_PreflightRetryCounter_ResetForRunClears(t *testing.T) {
	ag := newAgentWithRequiredFieldTool(t, "bash")

	calls := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"x"}`},
	}}
	msgs := []llm.Message{{Role: "user", Content: "explore"}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: calls})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, calls, msgs)

	if got := ag.PreflightFailureStreakForTest()["bash"]; got != 1 {
		t.Fatalf("after first failure: bash streak = %d, want 1", got)
	}

	ag.ResetForRun()
	if got := ag.PreflightFailureStreakForTest()["bash"]; got != 0 {
		t.Errorf("after ResetForRun: bash streak = %d, want 0", got)
	}
}

func TestAgent_PreflightRetryCounter_ThresholdIsTwo(t *testing.T) {
	ag := newAgentWithRequiredFieldTool(t, "bash")

	msgs := []llm.Message{{Role: "user", Content: "explore"}}

	badCall := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"x"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: badCall})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, badCall, msgs)

	if got := ag.PreflightFailureStreakForTest()["bash"]; got != 1 {
		t.Fatalf("after one failure: bash streak = %d, want 1", got)
	}

	lastFailed := lastToolPreflightError(msgs)
	if lastFailed == "" {
		t.Fatal("expected preflight error in msgs")
	}
	if err := ag.ShouldAbort(msgs, lastFailed); err != nil {
		t.Fatalf("ShouldAbort returned %v after single failure, want nil (threshold is 2)", err)
	}
}

func TestAgent_PreflightRetryCounter_SuccessClearsOwnToolOnly(t *testing.T) {
	ag := newAgentWithRequiredFieldTool(t, "read")

	msgs := []llm.Message{{Role: "user", Content: "explore"}}

	badRead := []llm.ToolCall{{
		ID:       "c1",
		Function: llm.FunctionCall{Name: "read", Arguments: `{"intent":"x"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: badRead})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, badRead, msgs)

	if got := ag.PreflightFailureStreakForTest()["read"]; got != 1 {
		t.Fatalf("after first failure: read streak = %d, want 1", got)
	}

	goodRead := []llm.ToolCall{{
		ID:       "c2",
		Function: llm.FunctionCall{Name: "read", Arguments: `{"command":"ls","intent":"y"}`},
	}}
	msgs = append(msgs, llm.Message{Role: "assistant", ToolCalls: goodRead})
	msgs, _ = agentcore.AgentExecuteToolsForTest(ag, goodRead, msgs)

	if got := ag.PreflightFailureStreakForTest()["read"]; got != 0 {
		t.Errorf("after successful read: read streak = %d, want 0 (own-tool success clears own counter)", got)
	}
}
