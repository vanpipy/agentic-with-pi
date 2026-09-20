package agent_test

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
	updated, _ := s.Update(s.TickForTest())
	after := ansi.Strip(updated.View())
	if before == after {
		t.Errorf("spinner frame did not advance after Tick: %q", before)
	}
}

func TestSpinnerTickMsgIsBubbleType(t *testing.T) {
	s := tui.NewSpinnerForTest()
	var msg tea.Msg = s.TickForTest()
	if _, ok := msg.(tui.SpinnerTickType); !ok {
		t.Errorf("spinner tick msg should be tui.SpinnerTickType, got %T", msg)
	}
}