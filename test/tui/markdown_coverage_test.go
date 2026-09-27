package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestRenderMarkdownForTest_EmptyStringReturnsEmpty(t *testing.T) {
	got := tui.RenderMarkdownForTest("", 80)
	if got != "" {
		t.Errorf("empty markdown input should return empty string, got %q", got)
	}
}

func TestRenderMarkdownForTest_WhitespaceOnlyReturnsEmpty(t *testing.T) {
	got := tui.RenderMarkdownForTest("   \n\t  ", 80)
	if got != "" {
		t.Errorf("whitespace-only markdown should return empty string, got %q", got)
	}
}

func TestRenderMarkdownForTest_PlainTextRoundtrips(t *testing.T) {
	got := tui.RenderMarkdownForTest("hello world", 80)
	if got == "" {
		t.Errorf("plain text markdown should produce non-empty output")
	}
}

func TestRenderMarkdownForTest_HeadingRenders(t *testing.T) {
	in := "# Heading\n\nbody"
	got := tui.RenderMarkdownForTest(in, 80)
	if got == "" {
		t.Errorf("heading markdown should produce non-empty output")
	}
}

func TestRenderMarkdownForTest_ListItemsRender(t *testing.T) {
	in := "- one\n- two\n- three"
	got := tui.RenderMarkdownForTest(in, 80)
	if got == "" {
		t.Errorf("list markdown should produce non-empty output")
	}
}

func TestRenderMarkdownForTest_CodeBlockRenders(t *testing.T) {
	in := "```\nfoo\nbar\n```"
	got := tui.RenderMarkdownForTest(in, 80)
	if got == "" {
		t.Errorf("code-block markdown should produce non-empty output")
	}
}

func TestRenderMarkdownForTest_WidthZeroStillRenders(t *testing.T) {
	got := tui.RenderMarkdownForTest("plain text", 0)
	if got == "" {
		t.Errorf("width=0 markdown should still produce output (no word-wrap)")
	}
}

func TestRenderMarkdownForTest_WidthChangesResult(t *testing.T) {
	in := strings.Repeat("longword ", 30)
	narrow := tui.RenderMarkdownForTest(in, 20)
	wide := tui.RenderMarkdownForTest(in, 200)
	if narrow == wide {
		t.Errorf("width=20 and width=200 should produce different outputs")
	}
}

func TestGetMdRendererForTest_ReturnsRenderer(t *testing.T) {
	r := tui.GetMdRendererForTest(80)
	if r == nil {
		t.Errorf("GetMdRendererForTest(80) should return a renderer")
	}
}

func TestGetMdRendererForTest_WidthZeroReturnsRenderer(t *testing.T) {
	r := tui.GetMdRendererForTest(0)
	if r == nil {
		t.Errorf("GetMdRendererForTest(0) should return a renderer")
	}
}

func TestGetMdRendererForTest_CachesRendererForSameWidth(t *testing.T) {
	r1 := tui.GetMdRendererForTest(80)
	r2 := tui.GetMdRendererForTest(80)
	if r1 != r2 {
		t.Errorf("same width should return the same cached renderer instance")
	}
}

func TestResetMdRenderersForTest_ClearsCache(t *testing.T) {
	_ = tui.GetMdRendererForTest(80)
	tui.ResetMdRenderersForTest()
	r := tui.GetMdRendererForTest(80)
	if r == nil {
		t.Errorf("renderer should be obtainable after reset (creates fresh)")
	}
}

func TestRenderMarkdownForTest_FollowedByResetStillWorks(t *testing.T) {
	_ = tui.RenderMarkdownForTest("first", 80)
	tui.ResetMdRenderersForTest()
	got := tui.RenderMarkdownForTest("second", 80)
	if got == "" {
		t.Errorf("render after reset should produce output")
	}
}

func TestRenderMarkdownForTest_ManyDistinctWidthsDoNotCrash(t *testing.T) {
	for w := 0; w < 200; w += 10 {
		out := tui.RenderMarkdownForTest("hi", w)
		if out == "" {
			t.Errorf("render at width=%d should produce output, got empty", w)
		}
	}
}