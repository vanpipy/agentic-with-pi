package agentcore_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/agent-core/util"
	"github.com/vanpiyp/awp/internal/llm"
)

func msgsEqual(a, b []llm.Message) bool {
	return reflect.DeepEqual(a, b)
}

func pairedCallAndResultMessages() []llm.Message {
	return []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "ls", Arguments: "{}"}},
		}},
		{Role: "tool", ToolCallID: "c1", Content: "ok"},
	}
}

func TestRepairMissingToolOutputsNoMissing(t *testing.T) {
	msgs := pairedCallAndResultMessages()
	repaired, count := util.RepairMissingToolOutputs(msgs)
	if count != 0 {
		t.Errorf("count = %d, want 0 (all paired, no repairs)", count)
	}
	if len(repaired) != len(msgs) {
		t.Fatalf("repaired len = %d, want %d (no messages added)", len(repaired), len(msgs))
	}
	for i := range msgs {
		if !reflect.DeepEqual(msgs[i], repaired[i]) {
			t.Errorf("repaired[%d] differs from msgs[%d]: got %+v want %+v", i, i, repaired[i], msgs[i])
		}
	}
}

func TestRepairMissingToolOutputsMissingOne(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "ls", Arguments: "{}"}},
		}},
	}
	repaired, count := util.RepairMissingToolOutputs(msgs)
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if len(repaired) != len(msgs)+1 {
		t.Fatalf("repaired len = %d, want %d (one synthesized)", len(repaired), len(msgs)+1)
	}
	last := repaired[len(repaired)-1]
	if last.Role != "tool" {
		t.Errorf("last.Role = %q, want tool", last.Role)
	}
	if last.ToolCallID != "c1" {
		t.Errorf("last.ToolCallID = %q, want c1", last.ToolCallID)
	}
	if !strings.Contains(last.Content, "interrupted") {
		t.Errorf("last.Content = %q, want contains 'interrupted'", last.Content)
	}
	if !strings.Contains(last.Content, "ls") {
		t.Errorf("last.Content = %q, want contains tool name 'ls'", last.Content)
	}
}

func TestRepairMissingToolOutputsMissingMultiple(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "ls", Arguments: "{}"}},
			{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "grep", Arguments: "{}"}},
			{ID: "c3", Type: "function", Function: llm.FunctionCall{Name: "read", Arguments: "{}"}},
		}},
	}
	repaired, count := util.RepairMissingToolOutputs(msgs)
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if len(repaired) != len(msgs)+3 {
		t.Fatalf("repaired len = %d, want %d", len(repaired), len(msgs)+3)
	}
	expected := []struct{ id, name string }{
		{"c1", "ls"},
		{"c2", "grep"},
		{"c3", "read"},
	}
	for i, want := range expected {
		m := repaired[len(msgs)+i]
		if m.Role != "tool" {
			t.Errorf("repaired[len(msgs)+%d].Role = %q, want tool", i, m.Role)
		}
		if m.ToolCallID != want.id {
			t.Errorf("repaired[len(msgs)+%d].ToolCallID = %q, want %q", i, m.ToolCallID, want.id)
		}
		if !strings.Contains(m.Content, want.name) {
			t.Errorf("repaired[len(msgs)+%d].Content = %q, want contains %q", i, m.Content, want.name)
		}
	}
}

func TestRepairMissingToolOutputsIdempotent(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "ls", Arguments: "{}"}},
		}},
	}
	once, count1 := util.RepairMissingToolOutputs(msgs)
	twice, count2 := util.RepairMissingToolOutputs(once)
	if count1 != 1 {
		t.Fatalf("first run count = %d, want 1", count1)
	}
	if count2 != 0 {
		t.Errorf("second run count = %d, want 0 (idempotent: no further repairs)", count2)
	}
	if len(once) != len(twice) {
		t.Fatalf("idempotent length mismatch: once=%d twice=%d", len(once), len(twice))
	}
	if !msgsEqual(once, twice) {
		t.Errorf("idempotent: once=%+v twice=%+v", once, twice)
	}
}

func TestRepairMissingToolOutputsOrderPreserved(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "ls", Arguments: "{}"}},
			{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "grep", Arguments: "{}"}},
		}},
		{Role: "tool", ToolCallID: "c1", Content: "first result"},
	}
	repaired, count := util.RepairMissingToolOutputs(msgs)
	if count != 1 {
		t.Fatalf("count = %d, want 1 (only c2 missing)", count)
	}
	if len(repaired) != len(msgs)+1 {
		t.Fatalf("repaired len = %d, want %d", len(repaired), len(msgs)+1)
	}
	if !reflect.DeepEqual(repaired[0], msgs[0]) ||
		!reflect.DeepEqual(repaired[1], msgs[1]) ||
		!reflect.DeepEqual(repaired[2], msgs[2]) {
		t.Errorf("existing messages altered:\nrepaired[0..2]=%+v\nmsgs[0..2]=%+v", repaired[:3], msgs[:3])
	}
	last := repaired[len(repaired)-1]
	if last.Role != "tool" || last.ToolCallID != "c2" {
		t.Errorf("last message: role=%q tool_call_id=%q, want tool/c2", last.Role, last.ToolCallID)
	}
	if !strings.Contains(last.Content, "grep") {
		t.Errorf("last.Content = %q, want contains 'grep'", last.Content)
	}
}

func TestRunOneTurnRepairsBeforeStep(t *testing.T) {
	chunks := []llm.LegacyStreamEvent{messageDeltaStopChunk("end_turn"), messageStopChunk()}
	core := &fakeCore{streamChunks: chunks}
	ag := newTestAgent(core, "test-model")
	ag.WithCompaction(compact.CompactionSettings{Enabled: false})

	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
			{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "ls", Arguments: "{}"}},
		}},
	}
	ch := make(chan agentcore.Event, 32)
	defer close(ch)

	_, _ = agentcore.AgentRunOneTurnForTest(ag, context.Background(), msgs, ch, 0)

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.requests) != 1 {
		t.Fatalf("LLM requests = %d, want 1 (runOneTurn should make exactly one LLM call)", len(core.requests))
	}
	got := core.requests[0].Messages
	if len(got) != len(msgs)+1 {
		t.Fatalf("LLM request messages = %d, want %d (msgs + synthesized tool result)", len(got), len(msgs)+1)
	}
	for i := range msgs {
		if !reflect.DeepEqual(got[i], msgs[i]) {
			t.Errorf("request[%d] differs from msgs[%d]: got %+v want %+v", i, i, got[i], msgs[i])
		}
	}
	last := got[len(got)-1]
	if last.Role != "tool" {
		t.Errorf("last.Role = %q, want tool", last.Role)
	}
	if last.ToolCallID != "c1" {
		t.Errorf("last.ToolCallID = %q, want c1", last.ToolCallID)
	}
	if !strings.Contains(last.Content, "interrupted") {
		t.Errorf("last.Content = %q, want contains 'interrupted'", last.Content)
	}
}
