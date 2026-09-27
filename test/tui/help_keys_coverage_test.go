package tui_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestFullHelpForTestReturnsThreeRows(t *testing.T) {
	help := tui.FullHelpForTest()
	if len(help) != 3 {
		t.Fatalf("FullHelpForTest should return 3 rows, got %d", len(help))
	}
	if len(help[0]) != 3 {
		t.Errorf("row 0: expected 3 bindings (submit/complete/expandToggle), got %d", len(help[0]))
	}
	if len(help[1]) != 1 {
		t.Errorf("row 1: expected 1 binding (scroll), got %d", len(help[1]))
	}
	if len(help[2]) != 2 {
		t.Errorf("row 2: expected 2 bindings (quit/quitEmpty), got %d", len(help[2]))
	}
}

func TestFullHelpRow0ContainsExpandToggle(t *testing.T) {
	help := tui.FullHelpForTest()
	if len(help[0]) < 3 {
		t.Fatalf("row 0 too short: %d", len(help[0]))
	}
	got := help[0][2].Help().Desc
	if got != "expand/collapse" {
		t.Errorf("row 0 binding 2 Help().Desc = %q, want %q", got, "expand/collapse")
	}
}

func TestFullHelpRow2ContainsBothQuitBindings(t *testing.T) {
	help := tui.FullHelpForTest()
	if len(help[2]) != 2 {
		t.Fatalf("row 2 should have 2 bindings, got %d", len(help[2]))
	}
	d0 := help[2][0].Help().Desc
	d1 := help[2][1].Help().Desc
	if d0 != "quit" {
		t.Errorf("row 2 binding 0 desc = %q, want %q", d0, "quit")
	}
	if d1 != "quit (empty input)" {
		t.Errorf("row 2 binding 1 desc = %q, want %q", d1, "quit (empty input)")
	}
}

func TestFullHelpIncludesScrollBinding(t *testing.T) {
	help := tui.FullHelpForTest()
	found := false
	for _, row := range help {
		for _, b := range row {
			if b.Help().Desc == "scroll chat" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("FullHelpForTest missing 'scroll chat' binding")
	}
}
