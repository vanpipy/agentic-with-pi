package agentcore_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func userPromptMsg(text string, promptNum int) tui.ChatMsg {
	return tui.ChatMsg{Role: tui.RoleUser, Text: text, PromptNum: promptNum}
}

func assistantMsg(text string) tui.ChatMsg {
	return tui.ChatMsg{Role: tui.RoleAssistant, Text: text}
}

func toolMsg(text string, collapsed bool) tui.ChatMsg {
	return tui.ChatMsg{Role: tui.RoleTool, Text: text, Collapsed: collapsed}
}

func toolMsgWithData(text string, collapsed bool, name string, args string) tui.ChatMsg {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      name,
		Arguments: json.RawMessage(args),
	}
	return tui.ChatMsg{
		Role:      tui.RoleTool,
		Text:      text,
		Collapsed: collapsed,
		ToolData:  part,
	}
}

func observeMsg(text string, collapsed bool) tui.ChatMsg {
	return tui.ChatMsg{Role: tui.RoleObserve, Text: text, Collapsed: collapsed}
}

func TestRenderedLinesCacheHitOnSameKey(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	msg := assistantMsg("hello world")
	first := m.RenderedLinesForTest(msg, 80)
	if first == nil {
		t.Fatalf("first render should return non-nil lines")
	}
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after first render, cache size = %d, want 1", size)
	}
	second := m.RenderedLinesForTest(msg, 80)
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after second render (same key), cache size = %d, want 1", size)
	}
	if len(first) != len(second) {
		t.Errorf("cached lines length mismatch: first=%d second=%d", len(first), len(second))
	}
}

func TestRenderedLinesCacheMissOnDifferentText(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	m.RenderedLinesForTest(assistantMsg("alpha"), 80)
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after first render, cache size = %d, want 1", size)
	}
	m.RenderedLinesForTest(assistantMsg("beta"), 80)
	if size := m.LinesCacheSizeForTest(); size != 2 {
		t.Fatalf("after second render (different text), cache size = %d, want 2", size)
	}
}

func TestRenderedLinesCacheMissOnWidthChange(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	msg := assistantMsg("a longer paragraph that should wrap differently depending on terminal width")
	m.RenderedLinesForTest(msg, 80)
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after first render at width 80, cache size = %d, want 1", size)
	}
	m.RenderedLinesForTest(msg, 40)
	if size := m.LinesCacheSizeForTest(); size != 2 {
		t.Fatalf("after render at width 40 (different key), cache size = %d, want 2", size)
	}
}

func TestRenderedLinesCacheMissOnCollapseChange(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "list",
		Arguments: json.RawMessage(`{"path":"/tmp"}`),
	}
	m.RenderedLinesForTest(tui.ChatMsg{
		Role:      tui.RoleTool,
		Text:      "list · bash(...)",
		Collapsed: false,
		ToolData:  part,
	}, 80)
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after first render (expanded), cache size = %d, want 1", size)
	}
	m.RenderedLinesForTest(tui.ChatMsg{
		Role:      tui.RoleTool,
		Text:      "list · bash(...)",
		Collapsed: true,
		ToolData:  part,
	}, 80)
	if size := m.LinesCacheSizeForTest(); size != 2 {
		t.Fatalf("after render with collapsed=true (different key), cache size = %d, want 2", size)
	}
}

func TestRenderedLinesCacheInvalidationOnSetSize(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	m.RenderedLinesForTest(assistantMsg("hi"), 80)
	m.RenderedLinesForTest(userPromptMsg("yo", 1), 80)
	if size := m.LinesCacheSizeForTest(); size != 2 {
		t.Fatalf("setup: cache size = %d, want 2", size)
	}
	m.SetSizeForTest(100, 40)
	if size := m.LinesCacheSizeForTest(); size != 0 {
		t.Errorf("SetSize should clear cache, got size %d", size)
	}
}

func TestRenderedLinesCacheInvalidationOnAppend(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	m.SubmitForTest("hello")
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after first submit, cache size = %d, want 1 (content pre-warms cache during refresh)", size)
	}
	m.SubmitForTest("second prompt")
	if size := m.LinesCacheSizeForTest(); size != 2 {
		t.Errorf("after second submit, cache size = %d, want 2 (refresh invalidates and pre-warms for new message set)", size)
	}
}

func TestContentAndLineCountShareCache(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	m.SubmitForTest("hi there")
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after submit, cache size = %d, want 1", size)
	}
	first := m.LineCountForTest(userPromptMsg("hi there", 1))
	if first <= 0 {
		t.Fatalf("lineCount for user prompt should be > 0, got %d", first)
	}
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Fatalf("after lineCount (cache hit on same key), cache size = %d, want 1", size)
	}
	content := m.ContentForTest()
	if content == "" {
		t.Fatalf("content should be non-empty")
	}
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Errorf("after content (single message, cache hit), cache size = %d, want 1 (shared cache, no new entries)", size)
	}
	second := m.LineCountForTest(userPromptMsg("hi there", 1))
	if second != first {
		t.Errorf("lineCount should be stable: first=%d second=%d", first, second)
	}
}

func TestRenderedLinesConsistentOverManyCalls(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	msg := assistantMsg("repeatable render target\n\n```plan\n- one\n- two\n```")
	first := m.RenderedLinesForTest(msg, 80)
	for i := 0; i < 100; i++ {
		got := m.RenderedLinesForTest(msg, 80)
		if len(got) != len(first) {
			t.Fatalf("iteration %d: lines length mismatch first=%d got=%d", i, len(first), len(got))
		}
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("iteration %d: line %d mismatch", i, j)
			}
		}
	}
	if size := m.LinesCacheSizeForTest(); size != 1 {
		t.Errorf("after 100 iterations of same key, cache size = %d, want 1", size)
	}
}

func TestLineCountMissesBoundedAcrossManyCalls(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	msg := assistantMsg("bounded misses target")
	for i := 0; i < 50; i++ {
		_ = m.LineCountForTest(msg)
	}
	if misses := m.LineCountMissesForTest(); misses != 1 {
		t.Errorf("expected 1 miss across 50 lineCount calls (same key), got %d", misses)
	}
}

func TestLineCountMissesPersistAcrossSetSize(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	_ = m.LineCountForTest(assistantMsg("hello"))
	if misses := m.LineCountMissesForTest(); misses != 1 {
		t.Fatalf("expected 1 miss before SetSize, got %d", misses)
	}
	m.SetSizeForTest(120, 40)
	_ = m.LineCountForTest(assistantMsg("hello"))
	if misses := m.LineCountMissesForTest(); misses != 2 {
		t.Errorf("after SetSize + lineCount: misses should be 2 (SetSize only clears cache, does not reset counter), got %d", misses)
	}
}

func TestRenderedLinesStreamingPathInvalidatesCache(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SetSizeForTest(80, 40)
	_ = m.ContentForTest()
	if size := m.LinesCacheSizeForTest(); size != 0 {
		t.Fatalf("empty model: cache size = %d, want 0", size)
	}
	m.AppendStreamForTest("chunk 1")
	if size := m.LinesCacheSizeForTest(); size != 0 {
		t.Errorf("appendStream should trigger refresh and clear cache, got size %d", size)
	}
	_ = m.ContentForTest()
	if size := m.LinesCacheSizeForTest(); size != 0 {
		t.Errorf("empty messages + streaming: linesCache should stay empty (streaming not cached), got size %d", size)
	}
}
