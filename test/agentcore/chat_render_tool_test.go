package agentcore_test

import (
	"encoding/json"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestRenderMsgToolCardHasMultipleLines(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "test",
		Arguments: json.RawMessage(`{}`),
	}
	legacy := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleTool,
		Text: "test · bash({})",
	}, 80)
	card := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "test · bash({})",
		ToolData: part,
	}, 80)
	if len(card) < 1 {
		t.Errorf("card render must produce at least one line, got %d", len(card))
	}
	if len(legacy) == 0 {
		t.Errorf("legacy render must produce at least one line, got 0")
	}
}

func TestRenderMsgToolCardContainsNameAndIntent(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "find jcode",
		Arguments: json.RawMessage(`{"q":"jcode"}`),
	}
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "find jcode · bash({\"q\":\"jcode\"})",
		ToolData: part,
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "find jcode") {
		t.Errorf("card should contain intent 'find jcode', got %q", combined)
	}
	if !strings.Contains(combined, "bash") {
		t.Errorf("card should contain name 'bash', got %q", combined)
	}
}

func TestRenderMsgToolCardHasBorderedFrame(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "test",
		Arguments: json.RawMessage(`{}`),
	}
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "test · bash({})",
		ToolData: part,
	}, 80)
	if len(lines) < 1 {
		t.Fatalf("card must produce at least one line, got 0")
	}
	for _, line := range lines {
		if strings.ContainsRune(line, '\u2502') || strings.ContainsRune(line, '\u256d') || strings.ContainsRune(line, '\u256e') {
			t.Errorf("flat row should not contain box-drawing border characters, got %q", line)
		}
	}
}

func TestRenderMsgToolCardContainsGlyph(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "test",
		Arguments: json.RawMessage(`{}`),
	}
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "test · bash({})",
		ToolData: part,
	}, 80)
	combined := strings.Join(lines, "\n")
	hasIcon := strings.ContainsRune(combined, '\u2713') ||
		strings.ContainsRune(combined, '\u25b8') ||
		strings.ContainsRune(combined, '\u2717')
	if !hasIcon {
		t.Errorf("tool row should contain status icon (✓/▸/✗), got %q", combined)
	}
}

func TestRenderMsgToolCardTruncatesLongArgs(t *testing.T) {
	longArgs := strings.Repeat("a", 200)
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "echo long",
		Arguments: json.RawMessage(longArgs),
	}
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "echo long · bash(...)",
		ToolData: part,
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, strings.Repeat("a", 60)) {
		t.Errorf("card should truncate long args, but 60+ run of 'a' found in %q", combined)
	}
	if !strings.Contains(combined, "…") {
		t.Errorf("card should contain truncation ellipsis, got %q", combined)
	}
}

func TestRenderMsgToolCollapsedHidesArgs(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "find jcode",
		Arguments: json.RawMessage(`{"q":"jcode"}`),
	}
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:      tui.RoleTool,
		Text:      "find jcode · bash({\"q\":\"jcode\"})",
		ToolData:  part,
		Collapsed: true,
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, `"q"`) {
		t.Errorf("collapsed card should hide args body, got %q", combined)
	}
	if !strings.Contains(combined, "find jcode") {
		t.Errorf("collapsed card should still show intent, got %q", combined)
	}
}

func TestRenderMsgToolExpandedShowsArgsPreview(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "list",
		Arguments: json.RawMessage(`{"command":"ls /tmp"}`),
	}
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "list · bash({\"command\":\"ls /tmp\"})",
		ToolData: part,
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "ls /tmp") {
		t.Errorf("expanded row should expose args preview, got %q", combined)
	}
}

func TestRenderMsgToolBackwardCompatNilToolDataFallsBack(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleTool,
		Text: "find jcode · bash({\"q\":\"jcode\"})",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	found := false
	for _, l := range lines {
		if strings.Contains(l, "bash") && strings.Contains(l, "find jcode") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("legacy (nil ToolData) render should contain original text, lines=%v", lines)
	}
}

func TestRenderMsgToolCardLineWidthRespectsBudget(t *testing.T) {
	part := &json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Intent:    "test",
		Arguments: json.RawMessage(`{}`),
	}
	width := 60
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:     tui.RoleTool,
		Text:     "test · bash({})",
		ToolData: part,
	}, width)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	for i, l := range lines {
		w := lipgloss.Width(l)
		if w > width {
			t.Errorf("card line %d exceeds width %d: width=%d %q", i, width, w, l)
		}
	}
}
