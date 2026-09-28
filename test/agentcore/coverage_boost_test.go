package agentcore_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/agent-core/util"
	"github.com/vanpiyp/awp/internal/llm"
)

func newCBTestAgent() (*agentcore.Agent, *bytes.Buffer) {
	var buf bytes.Buffer
	a := agentcore.NewAgentWithLogBufForTest(&buf)
	return a, &buf
}

func firstCategoryFor(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()
	var line struct {
		Category string `json:"category"`
	}
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) == 0 || len(bytes.TrimSpace(lines[0])) == 0 {
		t.Fatalf("expected at least one JSONL line, got %q", buf.String())
	}
	if err := json.Unmarshal(bytes.TrimSpace(lines[0]), &line); err != nil {
		t.Fatalf("unmarshal: %v\nline: %s", err, lines[0])
	}
	return line.Category
}

func TestNewToolResultCachePositiveCapacityStoresValue(t *testing.T) {
	c := util.NewToolResultCache(7)
	if got := c.Capacity(); got != 7 {
		t.Errorf("Capacity() = %d, want 7", got)
	}
	c.Put("k1", "v1")
	if got, ok := c.Get("k1"); !ok || got != "v1" {
		t.Errorf("Get(k1) = (%q,%v), want (v1,true)", got, ok)
	}
}

func TestNewToolResultCacheOneCapacity(t *testing.T) {
	c := util.NewToolResultCache(1)
	if got := c.Capacity(); got != 1 {
		t.Errorf("Capacity() = %d, want 1", got)
	}
	c.Put("only", "value")
	if got, ok := c.Get("only"); !ok || got != "value" {
		t.Errorf("Get(only) = (%q,%v), want (value,true)", got, ok)
	}
}

func TestNewToolResultCacheNegativeCapacityDefaultsToTwenty(t *testing.T) {
	c := util.NewToolResultCache(-3)
	if got := c.Capacity(); got != 20 {
		t.Errorf("negative capacity: Capacity() = %d, want 20", got)
	}
}

func TestNewToolResultCacheZeroCapacityDefaultsToTwenty(t *testing.T) {
	c := util.NewToolResultCache(0)
	if got := c.Capacity(); got != 20 {
		t.Errorf("zero capacity: Capacity() = %d, want 20", got)
	}
}

func TestToolResultCacheLenNilReceiverReturnsZero(t *testing.T) {
	var c *util.ToolResultCache
	if got := c.Len(); got != 0 {
		t.Errorf("nil Len() = %d, want 0", got)
	}
}

func TestToolResultCacheGetNilReceiverReturnsMiss(t *testing.T) {
	var c *util.ToolResultCache
	if v, ok := c.Get("any"); ok || v != "" {
		t.Errorf("nil Get = (%q,%v), want (\"\",false)", v, ok)
	}
}

func TestToolResultCachePutNilReceiverIsNoOp(t *testing.T) {
	var c *util.ToolResultCache
	c.Put("sig", "result")
}

func TestToolResultCacheCapacityNilReceiverReturnsZero(t *testing.T) {
	var c *util.ToolResultCache
	if got := c.Capacity(); got != 0 {
		t.Errorf("nil Capacity() = %d, want 0", got)
	}
}

func TestToolResultCacheCapacityReportsAssignedValue(t *testing.T) {
	c := util.NewToolResultCache(13)
	if got := c.Capacity(); got != 13 {
		t.Errorf("Capacity() = %d, want 13", got)
	}
}

func TestToolResultCacheCapacityUnchangedAfterEviction(t *testing.T) {
	c := util.NewToolResultCache(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	if got := c.Capacity(); got != 2 {
		t.Errorf("Capacity() after eviction = %d, want 2 (capacity invariant)", got)
	}
}

func TestToolCallDedupKeyFallsBackOnInvalidJSON(t *testing.T) {
	got := util.ToolCallDedupKeyForTest("bash", "not-valid-json")
	if got == "" {
		t.Errorf("fallback dedup key empty, want non-empty hex")
	}
	if len(got) != 64 {
		t.Errorf("fallback dedup key len = %d, want 64 (sha256 hex)", len(got))
	}
}

func TestToolCallDedupKeyInvalidJSONProducesDeterministicHash(t *testing.T) {
	first := util.ToolCallDedupKeyForTest("bash", "{invalid")
	second := util.ToolCallDedupKeyForTest("bash", "{invalid")
	if first != second {
		t.Errorf("invalid-JSON fallback non-deterministic: %q vs %q", first, second)
	}
}

func TestToolCallDedupKeyInvalidJSONHashDiffersFromCanonicalHash(t *testing.T) {
	invalid := util.ToolCallDedupKeyForTest("bash", "{invalid")
	canonical := util.ToolCallDedupKeyForTest("bash", `{"x":1}`)
	if invalid == canonical {
		t.Errorf("invalid-JSON hash should differ from canonical hash, both = %q", invalid)
	}
}

func TestCategoryNameAllKnownCategories(t *testing.T) {
	cases := []struct {
		cat  agentcore.EventCategory
		want string
	}{
		{agentcore.EventThoughtStart, "thought_start"},
		{agentcore.EventThoughtChunk, "thought_chunk"},
		{agentcore.EventThoughtEnd, "thought_end"},
		{agentcore.EventTool, "tool"},
		{agentcore.EventObserve, "observe"},
		{agentcore.EventFinalAnswer, "final_answer"},
		{agentcore.EventError, "error"},
		{agentcore.EventInvalid, "invalid"},
		{agentcore.EventUserMessage, "user_message"},
		{agentcore.EventCompaction, "compaction"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			ag, buf := newCBTestAgent()
			ag.WriteEventForTest(1, agentcore.Event{Category: tc.cat})
			ag.FlushLogForTest()
			got := firstCategoryFor(t, buf)
			if got != tc.want {
				t.Errorf("category = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCategoryNameUnknownValueReturnsUnknown(t *testing.T) {
	ag, buf := newCBTestAgent()
	ag.WriteEventForTest(1, agentcore.Event{Category: agentcore.EventCategory(9999)})
	ag.FlushLogForTest()
	got := firstCategoryFor(t, buf)
	if got != "unknown" {
		t.Errorf("unknown category = %q, want unknown", got)
	}
}

func TestWriteEventUserMessageSetsUserMessageField(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteEventForTest(7, agentcore.Event{
		Category: agentcore.EventUserMessage,
		Content:  "hello from user",
	})
	a.FlushLogForTest()

	line := bytes.TrimSpace(buf.Bytes())
	var entry struct {
		Kind        string `json:"kind"`
		Category    string `json:"category"`
		UserMessage string `json:"user_message"`
	}
	if err := json.Unmarshal(line, &entry); err != nil {
		t.Fatalf("unmarshal: %v\nline: %s", err, line)
	}
	if entry.Category != "user_message" {
		t.Errorf("category = %q, want user_message", entry.Category)
	}
	if entry.UserMessage != "hello from user" {
		t.Errorf("user_message = %q, want hello from user", entry.UserMessage)
	}
}

func TestWriteEventThoughtStartNoFieldsSet(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteEventForTest(1, agentcore.Event{Category: agentcore.EventThoughtStart})
	a.FlushLogForTest()

	line := bytes.TrimSpace(buf.Bytes())
	var entry map[string]any
	if err := json.Unmarshal(line, &entry); err != nil {
		t.Fatalf("unmarshal: %v\nline: %s", err, line)
	}
	if entry["category"].(string) != "thought_start" {
		t.Errorf("category = %v, want thought_start", entry["category"])
	}
	for _, f := range []string{"reasoning", "content", "tool_name", "tool_result", "tool_error", "user_message"} {
		if _, present := entry[f]; present {
			t.Errorf("thought_start should not emit %q, got %v", f, entry)
		}
	}
}

func TestWriteEventThoughtChunkEmitsReasoningAndContent(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteEventForTest(1, agentcore.Event{
		Category:  agentcore.EventThoughtChunk,
		Reasoning: "thinking hard",
		Content:   "intermediate text",
	})
	a.FlushLogForTest()

	line := bytes.TrimSpace(buf.Bytes())
	var entry map[string]any
	if err := json.Unmarshal(line, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["reasoning"].(string) != "thinking hard" {
		t.Errorf("reasoning = %v, want thinking hard", entry["reasoning"])
	}
	if entry["content"].(string) != "intermediate text" {
		t.Errorf("content = %v, want intermediate text", entry["content"])
	}
}

func TestWriteEventFinalAnswerEmitsContent(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteEventForTest(1, agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "the answer",
	})
	a.FlushLogForTest()

	line := bytes.TrimSpace(buf.Bytes())
	var entry map[string]any
	if err := json.Unmarshal(line, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["content"].(string) != "the answer" {
		t.Errorf("content = %v, want the answer", entry["content"])
	}
}

func TestWriteEventToolEmitsToolFields(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteEventForTest(1, agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "bash",
		ToolArgs: `{"command":"ls"}`,
		ToolCalls: []llm.ToolCall{
			{ID: "c1", Function: llm.FunctionCall{Name: "bash"}},
		},
	})
	a.FlushLogForTest()

	line := bytes.TrimSpace(buf.Bytes())
	var entry map[string]any
	if err := json.Unmarshal(line, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry["tool_name"].(string) != "bash" {
		t.Errorf("tool_name = %v, want bash", entry["tool_name"])
	}
	if entry["tool_args"].(string) != `{"command":"ls"}` {
		t.Errorf("tool_args = %v, want {\"command\":\"ls\"}", entry["tool_args"])
	}
	if int(entry["tool_calls_count"].(float64)) != 1 {
		t.Errorf("tool_calls_count = %v, want 1", entry["tool_calls_count"])
	}
}

func TestComputeFileListsTableDriven(t *testing.T) {
	cases := []struct {
		name         string
		ops          compact.FileOperations
		wantReadOnly []string
		wantModified []string
	}{
		{
			name:         "all_empty",
			ops:          compact.FileOperations{Read: map[string]bool{}, Written: map[string]bool{}, Edited: map[string]bool{}},
			wantReadOnly: nil,
			wantModified: nil,
		},
		{
			name:         "read_only_files",
			ops:          compact.FileOperations{Read: map[string]bool{"/a": true, "/b": true}, Written: map[string]bool{}, Edited: map[string]bool{}},
			wantReadOnly: []string{"/a", "/b"},
			wantModified: nil,
		},
		{
			name:         "written_files_only",
			ops:          compact.FileOperations{Read: map[string]bool{}, Written: map[string]bool{"/x": true, "/y": true}, Edited: map[string]bool{}},
			wantReadOnly: nil,
			wantModified: []string{"/x", "/y"},
		},
		{
			name:         "edited_files_only",
			ops:          compact.FileOperations{Read: map[string]bool{}, Written: map[string]bool{}, Edited: map[string]bool{"/e1": true, "/e2": true}},
			wantReadOnly: nil,
			wantModified: []string{"/e1", "/e2"},
		},
		{
			name:         "read_and_edited_overlap",
			ops:          compact.FileOperations{Read: map[string]bool{"/a": true, "/b": true}, Written: map[string]bool{}, Edited: map[string]bool{"/b": true}},
			wantReadOnly: []string{"/a"},
			wantModified: []string{"/b"},
		},
		{
			name:         "read_and_written_overlap",
			ops:          compact.FileOperations{Read: map[string]bool{"/a": true, "/b": true}, Written: map[string]bool{"/b": true}, Edited: map[string]bool{}},
			wantReadOnly: []string{"/a"},
			wantModified: []string{"/b"},
		},
		{
			name: "all_three_categories",
			ops: compact.FileOperations{
				Read:    map[string]bool{"/r1": true, "/r2": true, "/w1": true},
				Written: map[string]bool{"/w1": true, "/w2": true},
				Edited:  map[string]bool{"/e1": true},
			},
			wantReadOnly: []string{"/r1", "/r2"},
			wantModified: []string{"/e1", "/w1", "/w2"},
		},
		{
			name:         "sorted_outputs",
			ops:          compact.FileOperations{Read: map[string]bool{"/zebra": true, "/alpha": true, "/mango": true}, Written: map[string]bool{}, Edited: map[string]bool{}},
			wantReadOnly: []string{"/alpha", "/mango", "/zebra"},
			wantModified: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readOnly, modified := compact.ComputeFileLists(tc.ops)
			if !equalStringSlices(readOnly, tc.wantReadOnly) {
				t.Errorf("readOnly = %v, want %v", readOnly, tc.wantReadOnly)
			}
			if !equalStringSlices(modified, tc.wantModified) {
				t.Errorf("modified = %v, want %v", modified, tc.wantModified)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFormatFileOperationsTableDriven(t *testing.T) {
	cases := []struct {
		name           string
		readFiles      []string
		modifiedFiles  []string
		wantEmpty      bool
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:          "both_empty",
			readFiles:     nil,
			modifiedFiles: nil,
			wantEmpty:     true,
		},
		{
			name:           "read_only",
			readFiles:      []string{"/a", "/b"},
			modifiedFiles:  nil,
			mustContain:    []string{"<read-files>", "/a", "/b", "</read-files>"},
			mustNotContain: []string{"<modified-files>"},
		},
		{
			name:           "modified_only",
			readFiles:      nil,
			modifiedFiles:  []string{"/x", "/y"},
			mustContain:    []string{"<modified-files>", "/x", "/y", "</modified-files>"},
			mustNotContain: []string{"<read-files>"},
		},
		{
			name:          "both_sections",
			readFiles:     []string{"/r"},
			modifiedFiles: []string{"/m"},
			mustContain: []string{
				"<read-files>", "/r", "</read-files>",
				"<modified-files>", "/m", "</modified-files>",
			},
		},
		{
			name:          "both_empty_slices_typed",
			readFiles:     []string{},
			modifiedFiles: []string{},
			wantEmpty:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := compact.FormatFileOperations(tc.readFiles, tc.modifiedFiles)
			if tc.wantEmpty {
				if out != "" {
					t.Errorf("output = %q, want empty", out)
				}
				return
			}
			if out == "" {
				t.Fatalf("output empty, want sections")
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q in %q", want, out)
				}
			}
			for _, banned := range tc.mustNotContain {
				if strings.Contains(out, banned) {
					t.Errorf("output contains banned %q in %q", banned, out)
				}
			}
		})
	}
}

func TestExtractPreviousSummaryHitReturnsTrimmed(t *testing.T) {
	const prefix = "Previous conversation summary:\n"
	body := "## Goal\nfix the bug\n## Progress\n- [x] done"
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: prefix + body},
		{Role: "user", Content: "follow up"},
	}
	got := compact.ExtractPreviousSummary(msgs)
	if got != body {
		t.Errorf("ExtractPreviousSummary = %q, want %q", got, body)
	}
}

func TestExtractPreviousSummaryMissWhenNoPrefix(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "no prefix here"},
	}
	if got := compact.ExtractPreviousSummary(msgs); got != "" {
		t.Errorf("ExtractPreviousSummary = %q, want empty", got)
	}
}

func TestExtractPreviousSummaryIgnoresNonAssistantRoles(t *testing.T) {
	const prefix = "Previous conversation summary:\n"
	msgs := []llm.Message{
		{Role: "user", Content: prefix + "should not match user role"},
		{Role: "system", Content: prefix + "should not match system role"},
		{Role: "tool", Content: prefix + "should not match tool role"},
	}
	if got := compact.ExtractPreviousSummary(msgs); got != "" {
		t.Errorf("ExtractPreviousSummary = %q, want empty (non-assistant roles)", got)
	}
}

func TestCompactionStrategyActOnUnknownReturnsError(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	unknown := compact.CompactionStrategy("not_a_real_strategy")
	out, action, err := unknown.ActOn(context.Background(), a, msgs, nil)
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
	if !strings.Contains(err.Error(), "unknown compaction strategy") {
		t.Errorf("err = %q, want contains 'unknown compaction strategy'", err.Error())
	}
	if !strings.Contains(err.Error(), "not_a_real_strategy") {
		t.Errorf("err = %q, want contains the bad strategy name", err.Error())
	}
	if action != compact.ActionNone {
		t.Errorf("action = %q, want ActionNone", action)
	}
	if len(out) != len(msgs) {
		t.Errorf("out len = %d, want %d (msgs unchanged)", len(out), len(msgs))
	}
	for i := range out {
		if out[i].Content != msgs[i].Content {
			t.Errorf("out[%d] mutated: %q vs %q", i, out[i].Content, msgs[i].Content)
		}
	}
}

func TestCompactionStrategyActOnUnknownIgnoresNilAgent(t *testing.T) {
	unknown := compact.CompactionStrategy("foo")
	_, _, err := unknown.ActOn(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown strategy, got nil")
	}
}

func TestProactiveActOnAcceptsObservedPointer(t *testing.T) {
	a := &agentcore.Agent{}
	msgs := []llm.Message{{Role: "user", Content: "hi"}}
	v := 123
	_, _, err := compact.StrategyProactive.ActOn(context.Background(), a, msgs, &v)
	if err != nil {
		t.Fatalf("proactive should be a noop: %v", err)
	}
}

func TestWriteHeaderTruncatesLongSystemPrompt(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.LegacyStreamEvent{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.SetSystemPrompts(strings.Repeat("a", 300))
	buf := &bytes.Buffer{}
	ag.WithLogWriter(buf)
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(buf)
	var foundHeader bool
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["kind"] == "session" {
			sys, _ := entry["system"].(string)
			if len(sys) > 200+3 {
				t.Errorf("system field len = %d, want ≤203 (200 chars + '...')", len(sys))
			}
			if !strings.HasSuffix(sys, "...") {
				t.Errorf("system field should end with '...' (truncation marker), got %q", sys)
			}
			foundHeader = true
			break
		}
	}
	if !foundHeader {
		t.Fatalf("no session header found in log:\n%s", buf.String())
	}
}

func TestWriteHeaderIncludesToolsWhenRegistered(t *testing.T) {
	core := &fakeCore{streamChunks: []llm.LegacyStreamEvent{
		textDeltaChunk("ok"),
		messageDeltaStopChunk("end_turn"),
		messageStopChunk(),
	}}
	ag := newTestAgent(core, "test-model")
	ag.WithTool(agentcore.ToolFunc{
		N:  "alpha",
		D:  "first tool",
		P:  map[string]any{"type": "object"},
		Fn: func(ctx context.Context, argsJSON string) (string, error) { return "ok", nil },
	})
	ag.WithTool(agentcore.ToolFunc{
		N:  "beta",
		D:  "second tool",
		P:  map[string]any{"type": "object"},
		Fn: func(ctx context.Context, argsJSON string) (string, error) { return "ok", nil },
	})
	buf := &bytes.Buffer{}
	ag.WithLogWriter(buf)
	if _, err := runAgent(t, ag, "x"); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["kind"] != "session" {
			continue
		}
		toolsRaw, ok := entry["tools"].([]any)
		if !ok {
			t.Fatalf("session header missing tools array: %v", entry)
		}
		got := map[string]bool{}
		for _, n := range toolsRaw {
			got[n.(string)] = true
		}
		if !got["alpha"] || !got["beta"] {
			t.Errorf("session header tools missing alpha/beta: %v", got)
		}
		return
	}
	t.Fatalf("no session header found in log:\n%s", buf.String())
}

func TestWriteAlignedEventUserMessageWritesEnvelope(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteAlignedEventForTest(agentcore.Event{
		Category: agentcore.EventUserMessage,
		Content:  "hello from user",
	})
	a.FlushLogForTest()

	scanner := bufio.NewScanner(buf)
	var sawMessage bool
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var envelope map[string]any
		if err := json.Unmarshal(line, &envelope); err != nil {
			continue
		}
		if envelope["kind"] != "message" {
			continue
		}
		sawMessage = true
		rawEntry, _ := envelope["entry"].(map[string]any)
		if rawEntry == nil {
			t.Fatalf("envelope.entry missing or wrong type: %v", envelope)
		}
		msgField, _ := rawEntry["message"].(map[string]any)
		if msgField == nil {
			t.Fatalf("envelope.entry.message missing: %v", rawEntry)
		}
		if msgField["role"].(string) != "user" {
			t.Errorf("role = %v, want user", msgField["role"])
		}
	}
	if !sawMessage {
		t.Fatalf("no message envelope written for EventUserMessage; buf=%q", buf.String())
	}
}

func TestWriteAlignedEventThoughtStartOpensStreamBuffer(t *testing.T) {
	a, buf := newCBTestAgent()
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtStart})
	if buf.Len() != 0 {
		t.Errorf("EventThoughtStart should not write to logBuf directly, got %q", buf.String())
	}
}

func TestWriteAlignedEventNoBufDoesNotPanic(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventObserve, ToolName: "x", ToolResult: "y"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "ok"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventError, ToolError: "boom"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "thinking"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventThoughtEnd})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventTool, ToolName: "bash", ToolArgs: "{}"})
	a.WriteAlignedEventForTest(agentcore.Event{Category: agentcore.EventUserMessage, Content: "hi"})
}

func TestWriteAlignedEventFinalAnswerWithoutBufIsNoOp(t *testing.T) {
	a := &agentcore.Agent{}
	a.WriteAlignedEventForTest(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "answer",
	})
}

func TestRunReactiveCompactionEarlyExitWhenCutAtEnd(t *testing.T) {
	t.Skip("cutPoint >= len(restMsgs) is a defensive guard; FindCutPoint only returns 0 or i<len, making this branch unreachable from external tests without internal edits")
}

func TestRunReactiveCompactionIncludesPreviousSummaryMarker(t *testing.T) {
	a := &agentcore.Agent{}
	a.SetCoreForTest(&strategyTestFakeCore{})
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "assistant", Content: "Previous conversation summary:\nold summary"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
		{Role: "user", Content: "u4"},
		{Role: "assistant", Content: "a4"},
	}
	out, action, err := compact.StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if action != compact.ActionFullCompacted {
		t.Errorf("action = %q, want full", action)
	}
	if len(out) < 2 {
		t.Fatalf("expected at least system + summary, got %d msgs", len(out))
	}
	if !strings.Contains(out[1].Content, "Previous conversation summary:") {
		t.Errorf("summary msg missing prefix in %q", out[1].Content)
	}
}

func TestRunReactiveCompactionLogsToBufferWhenLogWriterSet(t *testing.T) {
	var buf bytes.Buffer
	a := agentcore.NewAgentWithLogBufForTest(&buf)
	a.SetCoreForTest(&strategyTestFakeCore{})
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
		{Role: "user", Content: "u4"},
		{Role: "assistant", Content: "a4"},
		{Role: "user", Content: "u5"},
		{Role: "assistant", Content: "a5"},
		{Role: "user", Content: "u6"},
		{Role: "assistant", Content: "a6"},
		{Role: "user", Content: "u7"},
		{Role: "assistant", Content: "a7"},
		{Role: "user", Content: "u8"},
		{Role: "assistant", Content: "a8"},
		{Role: "user", Content: "u9"},
		{Role: "assistant", Content: "a9"},
		{Role: "user", Content: "u10"},
		{Role: "assistant", Content: "a10"},
		{Role: "user", Content: "u11"},
		{Role: "assistant", Content: "a11"},
	}
	_, _, err := compact.StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	a.FlushLogForTest()
	if buf.Len() == 0 {
		t.Fatalf("expected compaction log entry to be written, got empty buffer")
	}
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["kind"] == "compaction" {
			return
		}
	}
	t.Errorf("no compaction log entry found in:\n%s", buf.String())
}

func TestRunReactiveCompactionStreamErrPropagates(t *testing.T) {
	a := &agentcore.Agent{}
	a.SetCoreForTest(&errStreamCore{err: errors.New("upstream boom")})
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
		{Role: "user", Content: "u4"},
		{Role: "assistant", Content: "a4"},
		{Role: "user", Content: "u5"},
		{Role: "assistant", Content: "a5"},
	}
	_, _, err := compact.StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err == nil {
		t.Fatal("expected error from StreamChat, got nil")
	}
	if !strings.Contains(err.Error(), "upstream boom") {
		t.Errorf("err = %q, want contains upstream boom", err.Error())
	}
}

func TestRunReactiveCompactionLengthFinishReasonFails(t *testing.T) {
	a := &agentcore.Agent{}
	a.SetCoreForTest(&lengthStopCore{})
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a3"},
		{Role: "user", Content: "u4"},
		{Role: "assistant", Content: "a4"},
		{Role: "user", Content: "u5"},
		{Role: "assistant", Content: "a5"},
	}
	_, _, err := compact.StrategyReactive.ActOn(context.Background(), a, msgs, nil)
	if err == nil {
		t.Fatal("expected error for length finish, got nil")
	}
	if !strings.Contains(err.Error(), "token cap") {
		t.Errorf("err = %q, want contains 'token cap'", err.Error())
	}
}

type errStreamCore struct{ err error }

func (e errStreamCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.LegacyStreamEvent, error) {
	return nil, e.err
}

type lengthStopCore struct{}

func (lengthStopCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.LegacyStreamEvent, error) {
	ch := make(chan llm.LegacyStreamEvent, 2)
	ch <- llm.LegacyStreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{Content: "truncated"},
	}}}}
	ch <- llm.LegacyStreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: llm.FinishReasonLength,
	}}}}
	close(ch)
	return ch, nil
}

func TestExtractPreviousSummaryReturnsFirstMatch(t *testing.T) {
	const prefix = "Previous conversation summary:\n"
	msgs := []llm.Message{
		{Role: "assistant", Content: prefix + "first"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: prefix + "second"},
	}
	got := compact.ExtractPreviousSummary(msgs)
	if got != "first" {
		t.Errorf("got %q, want first (first match wins)", got)
	}
}

func TestExtractPreviousSummaryEmptyMsgs(t *testing.T) {
	if got := compact.ExtractPreviousSummary(nil); got != "" {
		t.Errorf("nil msgs: got %q, want empty", got)
	}
	if got := compact.ExtractPreviousSummary([]llm.Message{}); got != "" {
		t.Errorf("empty msgs: got %q, want empty", got)
	}
}

func TestSelectStrategyEmergencyBranch(t *testing.T) {
	obs := 10
	got := compact.SelectStrategy(
		[]llm.Message{{Role: "user", Content: "short"}},
		compact.CompactionSettings{
			Enabled:          true,
			ReserveTokens:    100,
			MaxContextTokens: 100000,
			Proactive:        false,
			Semantic:         false,
		},
		&obs,
	)
	if got != compact.StrategyEmergency {
		t.Errorf("got %q, want StrategyEmergency (default branch when nothing fires)", got)
	}
}

func TestSelectStrategyEmergencyBranchWhenDisabled(t *testing.T) {
	obs := 100000
	got := compact.SelectStrategy(
		[]llm.Message{{Role: "user", Content: strings.Repeat("a", 4000)}},
		compact.CompactionSettings{
			Enabled:          false,
			ReserveTokens:    100,
			MaxContextTokens: 1000,
			Proactive:        true,
			Semantic:         true,
		},
		&obs,
	)
	if got != compact.StrategyEmergency {
		t.Errorf("got %q, want StrategyEmergency (disabled short-circuits)", got)
	}
}

func TestComputeFileListsSortStabilityIsInputIndependent(t *testing.T) {
	ops := compact.FileOperations{
		Read:    map[string]bool{"/x": true, "/y": true, "/z": true},
		Written: map[string]bool{"/m": true},
		Edited:  map[string]bool{},
	}
	r1, m1 := compact.ComputeFileLists(ops)
	r2, m2 := compact.ComputeFileLists(ops)
	if !equalStringSlices(r1, r2) {
		t.Errorf("readOnly non-deterministic: %v vs %v", r1, r2)
	}
	if !equalStringSlices(m1, m2) {
		t.Errorf("modified non-deterministic: %v vs %v", m1, m2)
	}
}

func TestFormatFileOperationsBothSectionsOrderReadFirst(t *testing.T) {
	out := compact.FormatFileOperations([]string{"/r"}, []string{"/m"})
	rIdx := strings.Index(out, "<read-files>")
	mIdx := strings.Index(out, "<modified-files>")
	if rIdx < 0 || mIdx < 0 {
		t.Fatalf("missing section markers in %q", out)
	}
	if rIdx >= mIdx {
		t.Errorf("read-files should appear before modified-files, got readIdx=%d modIdx=%d in %q", rIdx, mIdx, out)
	}
}

func TestFormatFileOperationsEmptyNonNilSlices(t *testing.T) {
	out := compact.FormatFileOperations([]string{}, []string{})
	if out != "" {
		t.Errorf("got %q, want empty", out)
	}
}

func TestSortReadAndModifiedAlphabetically(t *testing.T) {
	ops := compact.FileOperations{
		Read:    map[string]bool{"/c": true, "/a": true, "/b": true},
		Written: map[string]bool{"/z": true, "/y": true, "/x": true},
		Edited:  map[string]bool{},
	}
	readOnly, modified := compact.ComputeFileLists(ops)
	if !sort.StringsAreSorted(readOnly) {
		t.Errorf("readOnly not sorted: %v", readOnly)
	}
	if !sort.StringsAreSorted(modified) {
		t.Errorf("modified not sorted: %v", modified)
	}
}
