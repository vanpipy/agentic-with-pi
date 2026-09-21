package agent_test

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	tui "github.com/vanpiyp/awp/internal/tui"
)

func TestCopySelectionTextSkipsGlyphPrefix(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("hello world")
	m.SetSizeForTest(80, 10)

	m.BeginSelectionForTest(0, 0)
	m.ExtendSelectionForTest(0, 30)
	text := m.EndSelectionForTest()
	if text != "hello world" {
		t.Errorf("got %q, want %q", text, "hello world")
	}
}

func TestExtractSelectionTextSimpleUserPrompt(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("hello")
	m.SetSizeForTest(80, 10)

	m.BeginSelectionForTest(0, 0)
	m.ExtendSelectionForTest(0, 5)
	text := m.EndSelectionForTest()
	if text != "hello" {
		t.Errorf("want %q, got %q", "hello", text)
	}
}

func TestAnsiStringWidthIgnoresEscapes(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"\x1b[31mhello\x1b[0m", 5},
		{"hello", 5},
	}
	for _, tc := range tests {
		got := ansi.StringWidth(tc.in)
		if got != tc.want {
			t.Errorf("ansi.StringWidth(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestExtractSelectionTextMidLine(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("hello world")
	m.SetSizeForTest(80, 10)
	m.BeginSelectionForTest(0, 6)
	m.ExtendSelectionForTest(0, 11)
	got := m.EndSelectionForTest()
	want := "world"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractSelectionTextEmptyRange(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("hello")
	m.SetSizeForTest(80, 10)
	m.BeginSelectionForTest(0, 4)
	got := m.EndSelectionForTest()
	if got != "" {
		t.Errorf("click without extend should return empty, got %q", got)
	}
}

func TestExtractSelectionTextMultipleMessages(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("first message")
	m.SubmitForTest("second message")
	m.SubmitForTest("third message")
	m.SetSizeForTest(80, 30)
	m.BeginSelectionForTest(0, 0)
	m.ExtendSelectionForTest(2, 5)
	got := m.EndSelectionForTest()
	want := "first message\nsecond message\nthird"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractSelectionTextNewlinesSkipsAnsi(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("alpha bravo charlie")
	m.SetSizeForTest(80, 10)
	m.BeginSelectionForTest(0, 0)
	m.ExtendSelectionForTest(0, 5)
	got := m.EndSelectionForTest()
	if got != "alpha" {
		t.Errorf("got %q, want %q", got, "alpha")
	}
}

func TestDragEdgeForCoordTopZone(t *testing.T) {
	m := tui.NewChatModelForTest()
	for i := 0; i < 6; i++ {
		m.SubmitForTest("msg")
	}
	m.SetSizeForTest(80, 2)
	m.BeginSelectionForTest(5, 0)
	m.ExtendSelectionForTest(5, 1)
	if edge := m.DragEdgeForCoordForTest(0); edge != "top" {
		t.Errorf("top row should be edge=top, got %q (atTop=%v atBottom=%v)",
			edge, m.AtTopForTest(), m.AtBottomForTest())
	}
}

func TestDragEdgeForCoordBottomZone(t *testing.T) {
	m := tui.NewChatModelForTest()
	for i := 0; i < 6; i++ {
		m.SubmitForTest("msg")
	}
	m.SetSizeForTest(80, 2)
	m.ScrollUpForTest(2)
	m.BeginSelectionForTest(0, 0)
	m.ExtendSelectionForTest(0, 1)
	if edge := m.DragEdgeForCoordForTest(1); edge != "bottom" {
		t.Errorf("bottom row should be edge=bottom, got %q", edge)
	}
}

func TestDragEdgeForCoordMiddleIsNone(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("a")
	m.SetSizeForTest(80, 10)
	m.BeginSelectionForTest(0, 0)
	m.ExtendSelectionForTest(0, 1)
	if edge := m.DragEdgeForCoordForTest(5); edge != "none" {
		t.Errorf("middle row should be edge=none, got %q", edge)
	}
}

func TestSelectAll(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("first")
	m.SubmitForTest("second")
	m.SubmitForTest("third")
	m.SetSizeForTest(80, 10)
	m.SelectAllForTest()
	got := m.EndSelectionForTest()
	want := "first\nsecond\nthird"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHitTestForDragClampsToLastLine(t *testing.T) {
	m := tui.NewChatModelForTest()
	m.SubmitForTest("alpha")
	msgIdx, _ := m.HitTestForDragForTest(0, 999)
	if msgIdx != 0 {
		t.Errorf("overshoot y=999 should clamp to last msg idx=0, got %d", msgIdx)
	}
}

func TestExtractSelectionAcrossWrappedLines(t *testing.T) {
	m := tui.NewChatModelForTest()
	longText := "the quick brown fox jumps over the lazy dog every single day"
	m.SubmitForTest(longText)
	m.SetSizeForTest(20, 10)
	m.BeginSelectionForTest(0, 4)
	m.ExtendSelectionForTest(0, 49)
	got := m.EndSelectionForTest()
	want := "quick brown fox jumps over the lazy dog every"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}