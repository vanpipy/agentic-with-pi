package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestViewLineCount_MatchesHeight_NoContent(t *testing.T) {
	for _, h := range []int{24, 30, 50, 80} {
		m := tui.NewModelForTest()
		m.SetSizeForTest(100, h)
		v := m.ViewForTest()
		if got, want := countLines(v), h; got != want {
			t.Errorf("height=%d empty chat: view lines = %d, want %d", h, got, want)
		}
	}
}

func TestViewLineCount_MatchesHeight_LongUserMessage(t *testing.T) {
	for _, h := range []int{24, 30, 50, 80} {
		m := tui.NewModelForTest()
		m.SetSizeForTest(100, h)
		m.SubmitPromptForTest(text)

		v := m.ViewForTest()
		if got, want := countLines(v), h; got != want {
			t.Errorf("height=%d long user msg: view lines = %d, want %d", h, got, want)
		}
	}
}

func TestViewLineCount_MatchesHeight_ManyMessages(t *testing.T) {
	for _, h := range []int{24, 30, 50, 80} {
		m := tui.NewModelForTest()
		m.SetSizeForTest(100, h)

		for i := 0; i < 8; i++ {
			m.SubmitPromptForTest(text)
		}

		v := m.ViewForTest()
		if got, want := countLines(v), h; got != want {
			t.Errorf("height=%d many msgs: view lines = %d, want %d", h, got, want)
		}
	}
}

func TestView_LastLinesAreInputAndFooter(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetSizeForTest(100, 30)
	m.SubmitPromptForTest(text)

	v := m.ViewForTest()
	lines := strings.Split(v, "\n")
	last := lines[len(lines)-1]
	if last == "" {
		t.Errorf("last line should be footer, got empty: %q", last)
	}
	secondLast := lines[len(lines)-3]
	if secondLast == "" {
		t.Errorf("line before input should not be empty (input should be present)")
	}
}

func TestView_NoOffByOne_Empty(t *testing.T) {
	for _, h := range []int{24, 30, 50, 80} {
		m := tui.NewModelForTest()
		m.SetSizeForTest(100, h)

		v := m.ViewForTest()
		lines := strings.Split(v, "\n")

		if !strings.HasPrefix(v, "\n") && len(lines) > 0 && lines[0] == "" {
			t.Errorf("height=%d: leading empty line in view output", h)
		}

		if !strings.HasSuffix(v, "\n") {
			continue
		}
		if lines[len(lines)-1] != "" {
			t.Errorf("height=%d: trailing newline should produce empty last element", h)
		}
	}
}

func TestView_TextareaStaysAtBottom_LongChat(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetSizeForTest(100, 30)

	for i := 0; i < 5; i++ {
		m.SubmitPromptForTest(text)
	}

	v := m.ViewForTest()
	lines := strings.Split(v, "\n")

	if len(lines) != 30 {
		t.Fatalf("view produced %d lines, want 30", len(lines))
	}

	last := lines[len(lines)-1]
	if last == "" {
		t.Errorf("last line (footer) is empty")
	}
}

var text = strings.Repeat("This is a moderately long line that should wrap when the terminal width is around 96 columns. ", 8)

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
