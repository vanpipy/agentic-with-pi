package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func cmdContainsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	switch m := msg.(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, sub := range m {
			if cmdContainsQuit(sub) {
				return true
			}
		}
	}
	return false
}

func cmdContainsNonNil(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if msg == nil {
		return false
	}
	if bm, ok := msg.(tea.BatchMsg); ok {
		return len(bm) > 0
	}
	return true
}

func newQuitSafetyModel(t *testing.T) *tui.Model {
	t.Helper()
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return out.(*tui.Model)
}

func typeQuitAndEnter(m *tui.Model) tea.Cmd {
	m.InputAppendForTest("/quit")
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return cmd
}

func TestQuitSafetyHatchMidStreaming_TriggersQuit(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)

	cmd := typeQuitAndEnter(m)

	if m.StateForTest() != tui.StateCancelling {
		t.Fatalf("expected state to transition to StateCancelling after /quit mid-stream, got %v", m.StateForTest())
	}
	if !cmdContainsQuit(cmd) {
		t.Fatalf("expected returned cmd to contain tea.Quit after /quit mid-stream, got: %v", cmd)
	}
}

func TestQuitSafetyHatchMidCancelling_TriggersQuit(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateCancelling)

	cmd := typeQuitAndEnter(m)

	if m.StateForTest() != tui.StateCancelling {
		t.Fatalf("expected state to remain StateCancelling, got %v", m.StateForTest())
	}
	if !cmdContainsQuit(cmd) {
		t.Fatalf("expected returned cmd to contain tea.Quit during /quit in StateCancelling, got: %v", cmd)
	}
}

func TestQuitSafetyHatchReady_RegressionStillQuits(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateReady)

	cmd := typeQuitAndEnter(m)

	if !cmdContainsQuit(cmd) {
		t.Fatalf("regression: /quit in StateReady must still produce tea.Quit, got: %v", cmd)
	}
}

func TestPlainTextMidStreaming_StillBlocked(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("hello world")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateStreaming {
		t.Fatalf("plain text mid-stream must not change state, got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("plain text mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if m.InputValueForTest() != "hello world" {
		t.Fatalf("plain text mid-stream must remain in input box, got %q", m.InputValueForTest())
	}
}

func TestNewSafetyHatchMidStreaming_CancelsAndStartsFresh(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/new")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateCancelling {
		t.Fatalf("/new mid-stream must cancel (state -> StateCancelling), got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/new mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("/new mid-stream must clear input box, got %q", v)
	}
}

func TestResumeSafetyHatchMidStreaming_CancelsAndSwitches(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/resume")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateCancelling {
		t.Fatalf("/resume mid-stream must cancel (state -> StateCancelling), got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/resume mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("/resume mid-stream must clear input box, got %q", v)
	}
}

func TestCompactSafetyHatchMidStreaming_CancelsAndCompresses(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/compact")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateCancelling {
		t.Fatalf("/compact mid-stream must cancel (state -> StateCancelling), got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/compact mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("/compact mid-stream must clear input box, got %q", v)
	}
}

func TestUsageSafetyHatchMidStreaming_NoCancelExecutes(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/usage")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateStreaming {
		t.Fatalf("/usage mid-stream is read-only and must not cancel; state must remain %v, got %v", tui.StateStreaming, m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/usage mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("/usage mid-stream must clear input box, got %q", v)
	}
}

func TestSkillsSafetyHatchMidStreaming_NoCancelExecutes(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/skills")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateStreaming {
		t.Fatalf("/skills mid-stream is read-only and must not cancel; state must remain %v, got %v", tui.StateStreaming, m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/skills mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("/skills mid-stream must clear input box, got %q", v)
	}
}

func TestUnknownSlashCommandMidStreaming_StillExecutesViaBypass(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/totally-not-a-real-command")

	beforeView := m.ViewForTest()

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateCancelling {
		t.Fatalf("other slash commands mid-stream must cancel first; expected state -> StateCancelling, got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("other slash commands mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("other slash commands mid-stream must clear input box, got %q", v)
	}
	if strings.Contains(beforeView, "totally-not-a-real-command") == false {
		t.Fatalf("sanity: chat view should not have contained the unknown command before Enter; view=%q", beforeView)
	}
}

func TestUnknownSlashCommandReady_NoCancelStillExecutes(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateReady)
	m.InputAppendForTest("/still-not-a-real-command")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if cmdContainsQuit(cmd) {
		t.Fatalf("unknown slash command in StateReady must not produce tea.Quit, got: %v", cmd)
	}
	if m.StateForTest() != tui.StateReady {
		t.Fatalf("unknown slash command in StateReady must leave state at StateReady, got %v", m.StateForTest())
	}
	if v := m.InputValueForTest(); v != "" {
		t.Fatalf("unknown slash command must clear input box, got %q", v)
	}
}

func TestSafetyHatchHelperCoversCancelFirstSet(t *testing.T) {
	cases := map[string]bool{
		"/quit":    true,
		"/new":     true,
		"/resume":  true,
		"/compact": true,
		"/usage":   false,
		"/skills":  false,
		"/foo":     false,
		"/":        false,
		"hello":    false,
	}
	for in, want := range cases {
		if got := tui.IsCancelFirstCommandForTest(in); got != want {
			t.Fatalf("IsCancelFirstCommandForTest(%q) = %v, want %v", in, got, want)
		}
	}
}
