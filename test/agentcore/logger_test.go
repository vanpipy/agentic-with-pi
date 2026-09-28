package agentcore_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestLegacyWriteEventErrorIncludesToolName(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	ag.WriteLegacyEventForTest(agentcore.Event{
		Category:  agentcore.EventError,
		ToolName:  "bash",
		ToolError: "Tool bash: missing required field(s) [command]",
	})

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("expected one JSONL line, got empty buffer")
	}

	var entry struct {
		Kind      string `json:"kind"`
		Category  string `json:"category"`
		ToolName  string `json:"tool_name"`
		ToolError string `json:"tool_error"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("unmarshal legacy entry: %v\nline: %s", err, line)
	}

	if entry.Kind != "event" {
		t.Errorf("entry.kind = %q, want event", entry.Kind)
	}
	if entry.Category != "error" {
		t.Errorf("entry.category = %q, want error", entry.Category)
	}
	if entry.ToolName != "bash" {
		t.Errorf("entry.tool_name = %q, want bash (EventError must propagate ToolName)", entry.ToolName)
	}
	if entry.ToolError == "" {
		t.Errorf("entry.tool_error empty, want non-empty ToolError")
	}
}

func TestLegacyWriteEventErrorOmitsToolNameWhenUnset(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	ag.WriteLegacyEventForTest(agentcore.Event{
		Category:  agentcore.EventError,
		ToolError: "compaction failure",
	})

	var entry struct {
		Kind      string `json:"kind"`
		Category  string `json:"category"`
		ToolName  string `json:"tool_name"`
		ToolError string `json:"tool_error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("unmarshal legacy entry: %v\nbuf: %q", err, buf.String())
	}
	if entry.ToolName != "" {
		t.Errorf("entry.tool_name = %q, want empty (untouched Error events without ToolName must not emit field)", entry.ToolName)
	}
	if entry.ToolError != "compaction failure" {
		t.Errorf("entry.tool_error = %q, want 'compaction failure'", entry.ToolError)
	}
}

func writeAndFlush(t *testing.T, ag *agentcore.Agent, seq int, ev agentcore.Event) {
	t.Helper()
	ag.WriteEventForTest(seq, ev)
	ag.FlushLogForTest()
}

func decodeLegacyEventLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("expected one JSONL line, got empty buffer")
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("unmarshal entry: %v\nline: %s", err, line)
	}
	return out
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func TestWriteEventCategoryThoughtChunkMapsContent(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 7, agentcore.Event{
		Category: agentcore.EventThoughtChunk,
		Content:  "hello chunk",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["kind"] != "event" {
		t.Errorf("kind = %v, want event", got["kind"])
	}
	if got["seq"].(float64) != 7 {
		t.Errorf("seq = %v, want 7", got["seq"])
	}
	if got["category"] != "thought_chunk" {
		t.Errorf("category = %v, want thought_chunk", got["category"])
	}
	if got["content"] != "hello chunk" {
		t.Errorf("content = %v, want 'hello chunk'", got["content"])
	}
	if got["reasoning"] != nil {
		t.Errorf("reasoning = %v, want absent (Reasoning empty so field omitted)", got["reasoning"])
	}
}

func TestWriteEventCategoryThoughtChunkMapsReasoning(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 1, agentcore.Event{
		Category:  agentcore.EventThoughtChunk,
		Reasoning: "thinking hard",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["reasoning"] != "thinking hard" {
		t.Errorf("reasoning = %v, want 'thinking hard'", got["reasoning"])
	}
	if got["content"] != nil {
		t.Errorf("content = %v, want absent (Content empty so field omitted)", got["content"])
	}
}

func TestWriteEventCategoryThoughtEndMapsAllFields(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 2, agentcore.Event{
		Category:  agentcore.EventThoughtEnd,
		Reasoning: "done thinking",
		Content:   "final thought content",
		ToolCalls: []llm.ToolCall{{ID: "t1"}},
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "thought_end" {
		t.Errorf("category = %v, want thought_end", got["category"])
	}
	if got["reasoning"] != "done thinking" {
		t.Errorf("reasoning = %v, want 'done thinking'", got["reasoning"])
	}
	if got["content"] != "final thought content" {
		t.Errorf("content = %v, want 'final thought content'", got["content"])
	}
	if v, ok := got["tool_calls_count"].(float64); !ok || v != 1 {
		t.Errorf("tool_calls_count = %v, want 1", got["tool_calls_count"])
	}
}

func TestWriteEventCategoryToolMapsAllFields(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 3, agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "bash",
		ToolArgs: `{"command":"ls"}`,
		ToolCalls: []llm.ToolCall{
			{ID: "t1"},
			{ID: "t2"},
			{ID: "t3"},
		},
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "tool" {
		t.Errorf("category = %v, want tool", got["category"])
	}
	if got["tool_name"] != "bash" {
		t.Errorf("tool_name = %v, want bash", got["tool_name"])
	}
	if got["tool_args"] != `{"command":"ls"}` {
		t.Errorf("tool_args = %v, want tool args payload", got["tool_args"])
	}
	if v, ok := got["tool_calls_count"].(float64); !ok || v != 3 {
		t.Errorf("tool_calls_count = %v, want 3", got["tool_calls_count"])
	}
}

func TestWriteEventCategoryObserveMapsToolFields(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 4, agentcore.Event{
		Category:   agentcore.EventObserve,
		ToolName:   "bash",
		ToolResult: "file1\nfile2\n",
		ToolError:  "",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "observe" {
		t.Errorf("category = %v, want observe", got["category"])
	}
	if got["tool_name"] != "bash" {
		t.Errorf("tool_name = %v, want bash", got["tool_name"])
	}
	if got["content"] != "file1\nfile2\n" {
		t.Errorf("content = %v, want ToolResult mirrored into content", got["content"])
	}
	if hasKey(got, "tool_error") {
		t.Errorf("tool_error present when empty, want omitted: %v", got["tool_error"])
	}
}

func TestWriteEventCategoryObserveMapsToolError(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 5, agentcore.Event{
		Category:   agentcore.EventObserve,
		ToolName:   "bash",
		ToolError:  "tool crashed",
		ToolResult: "",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["tool_error"] != "tool crashed" {
		t.Errorf("tool_error = %v, want 'tool crashed'", got["tool_error"])
	}
	if hasKey(got, "content") {
		t.Errorf("content present when ToolResult empty, want omitted: %v", got["content"])
	}
}

func TestWriteEventCategoryFinalAnswerMapsContent(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 6, agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "the final answer is 42",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "final_answer" {
		t.Errorf("category = %v, want final_answer", got["category"])
	}
	if got["content"] != "the final answer is 42" {
		t.Errorf("content = %v, want 'the final answer is 42'", got["content"])
	}
}

func TestWriteEventCategoryErrorMapsFields(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 8, agentcore.Event{
		Category:  agentcore.EventError,
		ToolError: "boom",
		ToolName:  "bash",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "error" {
		t.Errorf("category = %v, want error", got["category"])
	}
	if got["tool_error"] != "boom" {
		t.Errorf("tool_error = %v, want boom", got["tool_error"])
	}
	if got["tool_name"] != "bash" {
		t.Errorf("tool_name = %v, want bash", got["tool_name"])
	}
}

func TestWriteEventCategoryUserMessageMapsContent(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 9, agentcore.Event{
		Category: agentcore.EventUserMessage,
		Content:  "user asked this",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "user_message" {
		t.Errorf("category = %v, want user_message", got["category"])
	}
	if got["user_message"] != "user asked this" {
		t.Errorf("user_message = %v, want 'user asked this'", got["user_message"])
	}
}

func TestWriteEventCategoryThoughtStartIsNoOpBeyondBase(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 10, agentcore.Event{
		Category:  agentcore.EventThoughtStart,
		Reasoning: "ignored",
		Content:   "ignored",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "thought_start" {
		t.Errorf("category = %v, want thought_start", got["category"])
	}
	for _, key := range []string{"reasoning", "content", "tool_name", "tool_args", "tool_error", "user_message", "tool_calls_count"} {
		if hasKey(got, key) {
			t.Errorf("%s present for thought_start, want omitted (default no-op case): %v", key, got[key])
		}
	}
}

func TestWriteEventCategoryEmptyFieldsAreOmitted(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 11, agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "",
		ToolArgs: "",
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["category"] != "tool" {
		t.Errorf("category = %v, want tool", got["category"])
	}
	if hasKey(got, "tool_name") {
		t.Errorf("tool_name present when empty, want omitted (omitempty tag): %v", got["tool_name"])
	}
	if hasKey(got, "tool_args") {
		t.Errorf("tool_args present when empty, want omitted (omitempty tag): %v", got["tool_args"])
	}
}

func TestWriteEventAlwaysSetsSeqAtCategoryKind(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	writeAndFlush(t, ag, 42, agentcore.Event{
		Category: agentcore.EventInvalid,
	})

	got := decodeLegacyEventLine(t, &buf)
	if got["kind"] != "event" {
		t.Errorf("kind = %v, want event", got["kind"])
	}
	if got["seq"].(float64) != 42 {
		t.Errorf("seq = %v, want 42", got["seq"])
	}
	if got["category"] != "invalid" {
		t.Errorf("category = %v, want invalid", got["category"])
	}
	if _, ok := got["at"].(string); !ok {
		t.Errorf("at = %v, want RFC3339Nano timestamp string", got["at"])
	}
}
