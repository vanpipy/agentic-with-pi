package agent_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestRoleLayoutForAllRoles(t *testing.T) {
	roles := []tui.Role{tui.RoleUser, tui.RoleAssistant, tui.RoleTool, tui.RoleObserve, tui.RoleError, tui.RoleSystem, tui.RoleThinking}
	for _, r := range roles {
		layout := tui.RoleLayoutForTest(r)
		if layout.Glyph() == "" {
			t.Errorf("role %v missing glyph", r)
		}
	}
}

func TestRenderMsgWrapsByLipglossWidth(t *testing.T) {
	longText := strings.Repeat("word ", 40)
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:      tui.RoleUser,
		Text:      longText,
		PromptNum: 7,
	}, 30)

	if len(lines) < 3 {
		t.Fatalf("expected wrapping into multiple lines, got %d", len(lines))
	}
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w > 30 {
			t.Errorf("line %d exceeds width 30: width=%d %q", i, w, line)
		}
	}
	if !strings.Contains(lines[0], "7") {
		t.Errorf("first line should contain prompt number 7, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "›") {
		t.Errorf("first line should contain user prompt arrow ‹, got %q", lines[0])
	}
}

func TestRenderMsgAssistantUsesMarkdown(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "# heading\n\nbody text",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
}

func TestRenderMsgThinkingCollapsed(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:      tui.RoleThinking,
		Text:      "long thought",
		Collapsed: true,
		Duration:  5000000000,
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	if !strings.Contains(lines[0], "thought for") {
		t.Errorf("collapsed thinking should show duration summary, got %q", lines[0])
	}
}

func TestRenderMsgToolCollapsedShowsSummary(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role:      tui.RoleTool,
		Text:      "read(/tmp/foo.rs) → first 5 lines of 200",
		Collapsed: true,
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	if !strings.Contains(lines[0], "▸") {
		t.Errorf("collapsed tool line should contain ▸ glyph, got %q", lines[0])
	}
	if strings.Contains(lines[0], "→") {
		t.Errorf("collapsed tool line should hide preview (no →), got %q", lines[0])
	}
}

func TestGlyphPrefixWithPromptNumber(t *testing.T) {
	layout := tui.RoleLayoutForTest(tui.RoleUser)
	got := tui.GlyphPrefixForTest(layout, tui.RoleUser, 3)
	if !strings.Contains(got, "3") {
		t.Errorf("prompt number 3 should appear in glyph, got %q", got)
	}
	if !strings.Contains(got, "›") {
		t.Errorf("user arrow should appear in glyph, got %q", got)
	}
}

func TestIndentWrappedLinesAligns(t *testing.T) {
	lines := tui.IndentWrappedLinesForTest([]string{
		"first line here",
		"second line",
	}, 4)
	if !strings.HasPrefix(lines[0], "    ") && lipgloss.Width(lines[0])-lipgloss.Width("first line here") != 4 {
		t.Errorf("first line should have 4-char indent prefix, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "    ") {
		t.Errorf("continuation should have same indent as first, got %q", lines[1])
	}
}

func TestPromptNumAtViewportTop(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("first prompt")
	m.SubmitForTest("second prompt")
	if got := m.PromptNumAtViewportTopForTest(); got != 1 {
		t.Errorf("promptNumAtViewportTop = %d, want 1 (top message)", got)
	}
}

func TestJumpToPromptStepsToNext(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("first")
	m.SubmitForTest("second")
	m.SubmitForTest("third")
	m.SetSizeForTest(80, 1)
	m.JumpToPromptForTest(-1)
	idx := m.PromptNumAtViewportTopForTest()
	if idx != 3 {
		t.Errorf("after JumpToPrompt(-1) from 1, promptNumAtViewportTop = %d, want 3", idx)
	}
	m.JumpToPromptForTest(1)
	idx = m.PromptNumAtViewportTopForTest()
	if idx != 2 {
		t.Errorf("after JumpToPrompt(1) backward, promptNumAtViewportTop = %d, want 2", idx)
	}
	m.JumpToPromptForTest(1)
	idx = m.PromptNumAtViewportTopForTest()
	if idx != 1 {
		t.Errorf("after another JumpToPrompt(1), promptNumAtViewportTop = %d, want 1", idx)
	}
}