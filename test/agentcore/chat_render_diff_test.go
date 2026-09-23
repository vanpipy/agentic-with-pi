package agentcore_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestRenderMsgAssistantSingleDiffBlockHasBorderedFrame(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n+ added line\n- removed line\n  context line\n```",
	}, 80)
	if len(lines) < 5 {
		t.Fatalf("diff block should add border lines (top + content + bottom), got %d: %v",
			len(lines), lines)
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "╭") {
		t.Errorf("diff block should have a top border (╭), got %q", combined)
	}
	if !strings.Contains(combined, "╰") {
		t.Errorf("diff block should have a bottom border (╰), got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockContainsAddLineContent(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n+ added line\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "added line") {
		t.Errorf("diff block should contain inner content 'added line', got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockContainsRemoveLineContent(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n- removed line\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "removed line") {
		t.Errorf("diff block should contain inner content 'removed line', got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockContainsContextLineContent(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n  unchanged line\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "unchanged line") {
		t.Errorf("diff block should contain inner content 'unchanged line', got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockStripsFenceMarkers(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n+ added line\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "```diff") {
		t.Errorf("rendered output should not contain the opening ```diff fence marker, got %q", combined)
	}
	if strings.Contains(combined, "\n```\n") {
		t.Errorf("rendered output should not contain the closing ``` fence marker on its own line, got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockPreservesSurroundingText(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro paragraph\n\n```diff\n+ added line\n- removed line\n```\n\noutro paragraph",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "intro paragraph") {
		t.Errorf("rendered output should contain 'intro paragraph', got %q", combined)
	}
	if !strings.Contains(combined, "outro paragraph") {
		t.Errorf("rendered output should contain 'outro paragraph', got %q", combined)
	}
}

func TestRenderMsgAssistantNonDiffFenceUntouched(t *testing.T) {
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
		t.Errorf("non-diff fence (```bash) should not produce a bordered block, lines=%v", lines)
	}
}

func TestRenderMsgAssistantMultipleDiffBlocksAllBordered(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n```diff\n+ first\n```\n\nmiddle\n\n```diff\n- second\n```\n\noutro",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for multi-diff assistant text")
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
	if !strings.Contains(combined, "first") || !strings.Contains(combined, "second") {
		t.Errorf("rendered output should contain both diff contents, got %q", combined)
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
		t.Errorf("expected at least 2 top borders for 2 diff blocks, got %d, lines=%v", topBorders, lines)
	}
	if bottomBorders < 2 {
		t.Errorf("expected at least 2 bottom borders for 2 diff blocks, got %d, lines=%v", bottomBorders, lines)
	}
}

func TestRenderMsgAssistantUnmatchedDiffFenceTreatedAsMarkdown(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "before\n\n```diff\n+ never closed\n\nafter",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for unmatched ```diff fence")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "before") {
		t.Errorf("unmatched diff fence should still render surrounding text, got %q", combined)
	}
	if !strings.Contains(combined, "after") {
		t.Errorf("unmatched diff fence should still render trailing text, got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockRespectsWidthBudget(t *testing.T) {
	width := 50
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "intro\n\n```diff\n+ step one\n- step two\n  context\n```\n\noutro",
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

func TestRenderMsgAssistantDiffBlockPlusLineIsStyled(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n+ green line\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "green line") {
		t.Fatalf("diff block should contain 'green line', got %q", combined)
	}
	if !strings.Contains(combined, "\x1b[") {
		t.Errorf("rendered diff block should contain ANSI escape codes for line coloring, got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockMinusLineIsStyled(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n- red line\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "red line") {
		t.Fatalf("diff block should contain 'red line', got %q", combined)
	}
	if !strings.Contains(combined, "\x1b[") {
		t.Errorf("rendered diff block should contain ANSI escape codes for line coloring, got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockPlusAndMinusLinesHaveDifferentAnsi(t *testing.T) {
	plusLines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n+ green content\n```",
	}, 80)
	minusLines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n- red content\n```",
	}, 80)
	plusCombined := strings.Join(plusLines, "\n")
	minusCombined := strings.Join(minusLines, "\n")
	if plusCombined == minusCombined {
		t.Errorf("plus and minus diff lines should render differently (different ANSI escapes), got equal output: %q",
			plusCombined)
	}
}

func TestRenderMsgAssistantDiffAndPlanBlocksCoexist(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```plan\n- step one\n```\n\n```diff\n+ added\n- removed\n```",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for plan + diff assistant text")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "step one") {
		t.Errorf("output should contain plan content 'step one', got %q", combined)
	}
	if !strings.Contains(combined, "added") {
		t.Errorf("output should contain diff content 'added', got %q", combined)
	}
	if !strings.Contains(combined, "removed") {
		t.Errorf("output should contain diff content 'removed', got %q", combined)
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
		t.Errorf("expected at least 2 top borders for plan + diff blocks, got %d, lines=%v", topBorders, lines)
	}
	if bottomBorders < 2 {
		t.Errorf("expected at least 2 bottom borders for plan + diff blocks, got %d, lines=%v", bottomBorders, lines)
	}
}

func TestRenderMsgAssistantDiffBlockWithoutFenceHasNoBorder(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "just a plain paragraph without any fence markers",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("expected at least one line for plain assistant text")
	}
	foundBorder := false
	for _, l := range lines {
		if strings.Contains(l, "╭") || strings.Contains(l, "╰") {
			foundBorder = true
			break
		}
	}
	if foundBorder {
		t.Errorf("plain text without any fence should not produce a bordered block, lines=%v", lines)
	}
}

func TestRenderMsgAssistantDiffBlockEmptyInnerContent(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n```",
	}, 80)
	if len(lines) == 0 {
		t.Fatal("empty ```diff block should still render at least the border, got 0 lines")
	}
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "╭") {
		t.Errorf("empty ```diff block should still produce a top border, got %q", combined)
	}
	if !strings.Contains(combined, "╰") {
		t.Errorf("empty ```diff block should still produce a bottom border, got %q", combined)
	}
}

func TestRenderMsgAssistantDiffBlockNeutralLineContent(t *testing.T) {
	lines := tui.RenderMsgForTest(tui.ChatMsg{
		Role: tui.RoleAssistant,
		Text: "```diff\n@@ -1,3 +1,3 @@\n+ added\n- removed\n```",
	}, 80)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "@@") {
		t.Errorf("diff block should contain hunk header '@@', got %q", combined)
	}
	if !strings.Contains(combined, "added") {
		t.Errorf("diff block should contain 'added', got %q", combined)
	}
	if !strings.Contains(combined, "removed") {
		t.Errorf("diff block should contain 'removed', got %q", combined)
	}
}
