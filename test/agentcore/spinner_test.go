package agentcore_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/vanpiyp/awp/internal/tui"
)

var minidotFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func TestStreamingUsesBubbleteaSpinner(t *testing.T) {
	s := tui.NewSpinnerForTest()
	body := ansi.Strip(s.View())
	if !strings.HasPrefix(body, minidotFrames[0]) {
		t.Errorf("spinner.View() body should start with MiniDot frame, got %q", body)
	}
}

func TestSpinnerAdvancesOnTick(t *testing.T) {
	s := tui.NewSpinnerForTest()
	before := ansi.Strip(s.View())
	msg := s.Tick()
	updated, _ := s.Update(msg)
	after := ansi.Strip(updated.View())
	if before == after {
		t.Errorf("spinner frame did not advance after Tick: %q", before)
	}
}

func TestSpinnerCyclesThroughFrames(t *testing.T) {
	s := tui.NewSpinnerForTest()
	seen := map[string]bool{}
	current := s
	var cmd tea.Cmd
	for i := 0; i < len(minidotFrames)+2; i++ {
		seen[ansi.Strip(current.View())] = true
		current, cmd = current.Update(current.Tick())
		_ = cmd
	}
	if len(seen) != len(minidotFrames) {
		t.Errorf("expected %d unique frames after cycling, saw %d: %v",
			len(minidotFrames), len(seen), seen)
	}
}
