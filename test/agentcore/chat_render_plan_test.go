package agentcore_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestRenderMsgAssistantWithPlanBlockProducesMoreLines(t *testing.T) {
	plain := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro paragraph\n\n- bullet one\n- bullet two",
	}, 80)
	withPlan := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro paragraph\n\n```plan\n- step one\n- step two\n```\n\noutro paragraph",
	}, 80)
	if len(withPlan) <= len(plain) {
		t.Errorf("plan-blocked render should produce more lines than plain: plain=%d plan=%d",
			len(plain), len(withPlan))
	}
}

func TestRenderMsgAssistantSinglePlanBlockHasBorderedFrame(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```plan\n- step one\n- step two\n```",
	}, 80)
	if len(lines) < 5 {
		t.Fatalf("plan block should add border lines (top + content + bottom), got %d: %v",
			len(lines), lines)
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "╭") {
		t.Errorf("plan block should have a top border (╭), got %q", combined)
	}
	if !strings.Contains(combined, "╰") {
		t.Errorf("plan block should have a bottom border (╰), got %q", combined)
	}
}

func TestRenderMsgAssistantPlanBlockContainsInnerContent(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n```plan\n- step one\n- step two\n```\n\noutro",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "step one") {
		t.Errorf("plan block should contain inner content 'step one', got %q", combined)
	}
	if !strings.Contains(combined, "step two") {
		t.Errorf("plan block should contain inner content 'step two', got %q", combined)
	}
}

func TestRenderMsgAssistantPlanBlockStripsFenceMarkers(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```plan\n- step one\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "```plan") {
		t.Errorf("rendered output should not contain the opening ```plan fence marker, got %q", combined)
	}
	if strings.Contains(combined, "\n```\n") {
		t.Errorf("rendered output should not contain the closing ``` fence marker on its own line, got %q", combined)
	}
}

func TestRenderMsgAssistantPreservesSurroundingText(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro paragraph\n\n```plan\n- step one\n```\n\noutro paragraph",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "intro paragraph") {
		t.Errorf("rendered output should contain 'intro paragraph', got %q", combined)
	}
	if !strings.Contains(combined, "outro paragraph") {
		t.Errorf("rendered output should contain 'outro paragraph', got %q", combined)
	}
}

func TestRenderMsgAssistantWithoutPlanBlockUnchanged(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "just a normal reply with **bold** and a list:\n\n- a\n- b",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for plain assistant text")
	}
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "```") {
		t.Errorf("rendered plain text should not leak any fence markers, got %q", combined)
	}
}

func TestRenderMsgAssistantNonPlanFenceUntouched(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "before\n\n```bash\necho hello\n```\n\nafter",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for ```bash fenced assistant text")
	}
	foundBorder := false
	for _, l := range lines {
		if strings.Contains(l, "╭") || strings.Contains(l, "╰") {
			foundBorder = true
			break
		}
	}
	if foundBorder {
		t.Errorf("non-plan fence (```bash) should not produce a bordered block, lines=%v", lines)
	}
}

func TestRenderMsgAssistantMultiplePlanBlocksAllBordered(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n```plan\n- a\n```\n\nmiddle\n\n```plan\n- b\n```\n\noutro",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for multi-plan assistant text")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "intro") {
		t.Errorf("rendered output should contain 'intro', got %q", combined)
	}
	if !strings.Contains(combined, "middle") {
		t.Errorf("rendered output should contain 'middle', got %q", combined)
	}
	if !strings.Contains(combined, "outro") {
		t.Errorf("rendered output should contain 'outro', got %q", combined)
	}
	if !strings.Contains(combined, "a") || !strings.Contains(combined, "b") {
		t.Errorf("rendered output should contain both plan contents, got %q", combined)
	}
	topBorders := 0
	bottomBorders := 0
	for _, l := range lines {
		if strings.Contains(l, "╭") {
			topBorders++
		}
		if strings.Contains(l, "╰") {
			bottomBorders++
		}
	}
	if topBorders < 2 {
		t.Errorf("expected at least 2 top borders for 2 plan blocks, got %d, lines=%v", topBorders, lines)
	}
	if bottomBorders < 2 {
		t.Errorf("expected at least 2 bottom borders for 2 plan blocks, got %d, lines=%v", bottomBorders, lines)
	}
}

func TestRenderMsgAssistantUnmatchedPlanFenceTreatedAsMarkdown(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "before\n\n```plan\n- never closed\n\nafter",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for unmatched ```plan fence")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "before") {
		t.Errorf("unmatched plan fence should still render surrounding text, got %q", combined)
	}
	if !strings.Contains(combined, "after") {
		t.Errorf("unmatched plan fence should still render trailing text, got %q", combined)
	}
}

func TestRenderMsgAssistantPlanBlockRespectsWidthBudget(t *testing.T) {
	width := 50
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n```plan\n- step one\n- step two\n```\n\noutro",
	}, width)
	if len(lines) == 0 {
		t.Fatal("expected at least one line")
	}
	for i, l := range lines {
		w := lipgloss.Width(l)
		if w > width {
			t.Errorf("line %d exceeds width %d: width=%d %q", i, width, w, l)
		}
	}
}
