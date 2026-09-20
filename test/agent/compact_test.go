package agent_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
)


func TestSummarizationPromptSections(t *testing.T) {
	prompt := agent.SummarizationPrompt
	requiredSections := []string{
		"## Goal",
		"## Constraints & Preferences",
		"## Progress",
		"### Done",
		"### In Progress",
		"### Blocked",
		"## Key Decisions",
		"## Next Steps",
		"## Critical Context",
	}
	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("SummarizationPrompt missing section %q", section)
		}
	}
}

func TestSummarizationPromptInstructsNotToContinue(t *testing.T) {
	prompt := agent.SummarizationPrompt
	lowered := strings.ToLower(prompt)
	wantPhrases := []string{"do not continue", "do not respond", "only"}
	for _, want := range wantPhrases {
		if !strings.Contains(lowered, want) {
			t.Errorf("SummarizationPrompt missing %q (must instruct model to summarize only)", want)
		}
	}
}

func TestUpdateSummarizationPromptPreservesRules(t *testing.T) {
	prompt := agent.UpdateSummarizationPrompt
	requiredSections := []string{
		"## Goal",
		"## Constraints & Preferences",
		"## Progress",
		"### Done",
		"### In Progress",
		"### Blocked",
		"## Key Decisions",
		"## Next Steps",
		"## Critical Context",
	}
	for _, section := range requiredSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("UpdateSummarizationPrompt missing section %q", section)
		}
	}
	lowered := strings.ToLower(prompt)
	for _, phrase := range []string{"preserve", "update", "previous"} {
		if !strings.Contains(lowered, phrase) {
			t.Errorf("UpdateSummarizationPrompt must mention %q (iterative update semantics)", phrase)
		}
	}
}

func TestExtractFileOpsReadsWritesEdits(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "do work"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "c1", Function: llm.FunctionCall{Name: "read", Arguments: `{"path":"/a.go"}`}},
			{ID: "c2", Function: llm.FunctionCall{Name: "write", Arguments: `{"path":"/b.go"}`}},
			{ID: "c3", Function: llm.FunctionCall{Name: "edit", Arguments: `{"path":"/c.go","edits":[]}`}},
			{ID: "c4", Function: llm.FunctionCall{Name: "bash", Arguments: `{"command":"ls"}`}},
		}},
	}
	ops := agent.ExtractFileOps(msgs)
	if !ops.Read["/a.go"] {
		t.Errorf("missing read /a.go: %+v", ops.Read)
	}
	if !ops.Written["/b.go"] {
		t.Errorf("missing written /b.go: %+v", ops.Written)
	}
	if !ops.Edited["/c.go"] {
		t.Errorf("missing edited /c.go: %+v", ops.Edited)
	}
	if ops.Read["/b.go"] || ops.Read["/c.go"] {
		t.Errorf("read set leaked into write/edit: %+v", ops.Read)
	}
}

func TestComputeFileListsSeparatesReadOnlyFromModified(t *testing.T) {
	ops := agent.FileOperations{
		Read:    map[string]bool{"/x.go": true, "/y.go": true},
		Written: map[string]bool{"/y.go": true},
		Edited:  map[string]bool{"/z.go": true},
	}
	readOnly, modified := agent.ComputeFileLists(ops)
	if len(readOnly) != 1 || readOnly[0] != "/x.go" {
		t.Errorf("readOnly = %v, want [/x.go]", readOnly)
	}
	if len(modified) != 2 || modified[0] != "/y.go" || modified[1] != "/z.go" {
		t.Errorf("modified = %v, want [/y.go /z.go]", modified)
	}
}

func TestFormatFileOperationsWrapsInXMLTags(t *testing.T) {
	got := agent.FormatFileOperations([]string{"/r.go"}, []string{"/m.go"})
	if !strings.Contains(got, "<read-files>") || !strings.Contains(got, "/r.go") || !strings.Contains(got, "</read-files>") {
		t.Errorf("missing <read-files>: %q", got)
	}
	if !strings.Contains(got, "<modified-files>") || !strings.Contains(got, "/m.go") || !strings.Contains(got, "</modified-files>") {
		t.Errorf("missing <modified-files>: %q", got)
	}
}

func TestFormatFileOperationsEmptyReturnsEmpty(t *testing.T) {
	if got := agent.FormatFileOperations(nil, nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTruncateForSummaryKeepsShort(t *testing.T) {
	got := agent.TruncateForSummary("hello", 100)
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTruncateForSummaryCapsLong(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got := agent.TruncateForSummary(long, 200)
	if len(got) > 400 {
		t.Errorf("truncated output too long: %d chars", len(got))
	}
	if !strings.Contains(got, "[... 4800 more characters truncated]") {
		t.Errorf("missing truncation marker: %q", got)
	}
	if !strings.HasPrefix(got, strings.Repeat("x", 200)) {
		t.Errorf("did not keep first 200 chars: prefix=%q", got[:50])
	}
}

func TestSerializeForSummaryTruncatesToolResult(t *testing.T) {
	toolResult := strings.Repeat("a", 5000)
	msgs := []llm.Message{
		{Role: "tool", ToolCallID: "c1", Content: toolResult},
	}
	out := agent.SerializeForSummary(msgs)
	if !strings.Contains(out, "[... 3000 more characters truncated]") {
		t.Errorf("tool result not truncated: %q", out)
	}
}

func TestExtractPreviousSummaryFindsPriorCompaction(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "assistant", Content: "Previous conversation summary:\n## Goal\ndo stuff\n"},
		{Role: "user", Content: "more"},
	}
	got := agent.ExtractPreviousSummary(msgs)
	if !strings.Contains(got, "## Goal") {
		t.Errorf("missing goal section: %q", got)
	}
	if strings.Contains(got, "Previous conversation summary:") {
		t.Errorf("still contains prefix: %q", got)
	}
}

func TestExtractPreviousSummaryEmptyWhenAbsent(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	if got := agent.ExtractPreviousSummary(msgs); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := agent.EstimateTokens("hello world"); got != 3 {
		t.Errorf("got %d, want ~3", got)
	}
}

func TestCountUserTurns(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "user", Content: "u3"},
	}
	if got := agent.CountUserTurns(msgs); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestFindCutPointKeepsRecent(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system"},
		{Role: "user", Content: "u1"},
		{Role: "assistant"},
		{Role: "user", Content: "u2"},
		{Role: "assistant"},
		{Role: "user", Content: "u3"},
		{Role: "assistant"},
		{Role: "user", Content: "u4"},
		{Role: "assistant"},
	}

	if got := agent.FindCutPoint(msgs, 2); got != 5 {
		t.Errorf("cut at %d, want 5 (u3 boundary, keeping u3+u4 = 2 user turns)", got)
	}
}

func TestFindCutPointKeepZero(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system"},
		{Role: "user", Content: "u1"},
		{Role: "assistant"},
	}
	if got := agent.FindCutPoint(msgs, 0); got != 0 {
		t.Errorf("got %d, want 0 (keep 0 turns = drop everything)", got)
	}
}

func TestShouldCompactTriggersAboveReserve(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: string(make([]byte, 40000))},
		{Role: "assistant", Content: string(make([]byte, 40000))},
	}
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 1024}
	if got := agent.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 10000}, settings); !got {
		t.Errorf("expected shouldCompact=true (20000 tokens > 10000-1024=8976)")
	}
}

func TestShouldCompactDisabledAlwaysFalse(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 100000))}}
	settings := agent.CompactionSettings{Enabled: false, ReserveTokens: 0}
	if got := agent.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 1000}, settings); got {
		t.Errorf("disabled should never compact")
	}
}

func TestShouldCompactBelowReserveStays(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 1000))}}
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 5000}
	if got := agent.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 10000}, settings); got {
		t.Errorf("expected shouldCompact=false (250 tokens < 10000-5000=5000)")
	}
}

func TestShouldCompactFallsBackToDefaultWhenModelZero(t *testing.T) {
	msgs := []llm.Message{{Role: "user", Content: string(make([]byte, 600000))}}
	settings := agent.CompactionSettings{Enabled: true, ReserveTokens: 100}
	if got := agent.ShouldCompactWithModel(msgs, llm.Model{ID: "m", MaxContextTokens: 0}, settings); !got {
		t.Errorf("expected compact: model has no window, fallback 128000, 150000 tokens > 128000-100")
	}
}

func TestSerializeForSummary(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "ignore me"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
		{Role: "tool", ToolCallID: "c1", Content: "result1"},
	}
	out := agent.SerializeForSummary(msgs)
	if !strings.Contains(out, "[user] hello") {
		t.Errorf("missing user content in %q", out)
	}
	if !strings.Contains(out, "[assistant] world") {
		t.Errorf("missing assistant content in %q", out)
	}
	if !strings.Contains(out, "[tool result c1] result1") {
		t.Errorf("missing tool result in %q", out)
	}
	if strings.Contains(out, "ignore me") {
		t.Errorf("system content leaked into summary")
	}
}
