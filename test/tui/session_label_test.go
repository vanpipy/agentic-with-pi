package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestSessionLabelEmpty(t *testing.T) {
	if got := tui.SessionLabelForTest("", 80); got != "(none)" {
		t.Errorf("got %q, want (none)", got)
	}
}

func TestSessionLabelFullIDWhenWidthPermits(t *testing.T) {
	id := "115845c05e9754f4f3664bc71be621e6"
	if got := tui.SessionLabelForTest(id, 80); got != id {
		t.Errorf("got %q, want full id %q", got, id)
	}
}

func TestSessionLabelShortIDUnchanged(t *testing.T) {
	if got := tui.SessionLabelForTest("abc123", 80); got != "abc123" {
		t.Errorf("got %q, want abc123", got)
	}
}

func TestSessionLabelTruncatesWithEllipsisWhenTooNarrow(t *testing.T) {
	id := "115845c05e9754f4f3664bc71be621e6"
	got := tui.SessionLabelForTest(id, 12)
	if got == id {
		t.Errorf("expected truncation at width 12, got full id %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected trailing ellipsis, got %q", got)
	}
	if !strings.HasPrefix(got, id[:8]) {
		t.Errorf("expected prefix preserved for file matching, got %q (want prefix %q)", got, id[:8])
	}
}

func TestSessionLabelMatchesFilePrefix(t *testing.T) {
	id := "115845c05e9754f4f3664bc71be621e6"
	got := tui.SessionLabelForTest(id, 80)
	fileName := got + ".jsonl"
	if !strings.HasPrefix(fileName, id) {
		t.Errorf("display %q does not match file prefix %q.jsonl", got, id)
	}
}

func TestRenderHeaderIncludesFullSessionID(t *testing.T) {
	id := "115845c05e9754f4f3664bc71be621e6"
	view := tui.RenderHeaderForTest(120, "ready", id)
	stripped := stripANSI(view)
	if !strings.Contains(stripped, id) {
		t.Errorf("rendered header should contain full session id %q, got:\n%s", id, stripped)
	}
}

func TestRenderHeaderTruncatesOnNarrowTerminal(t *testing.T) {
	id := "115845c05e9754f4f3664bc71be621e6"
	view := tui.RenderHeaderForTest(25, "ready", id)
	stripped := stripANSI(view)
	if strings.Contains(stripped, id) {
		t.Errorf("narrow terminal should truncate id, got full id in:\n%s", stripped)
	}
	if !strings.Contains(stripped, "…") {
		t.Errorf("narrow terminal should show ellipsis, got:\n%s", stripped)
	}
	if !strings.Contains(stripped, id[:8]) {
		t.Errorf("narrow terminal should preserve id prefix for matching, got:\n%s", stripped)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' || r == 'K' || r == 'H' || r == 'J' || r == 'A' || r == 'B' || r == 'C' || r == 'D' || r == 'G' {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
