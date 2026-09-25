package tui_test

import (
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

func TestSlashNewMidStreaming_StillBlocked(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/new")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateStreaming {
		t.Fatalf("/new mid-stream must not change state, got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/new mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if m.InputValueForTest() != "/new" {
		t.Fatalf("/new mid-stream must remain in input box, got %q", m.InputValueForTest())
	}
}

func TestSlashCompactMidStreaming_StillBlocked(t *testing.T) {
	m := newQuitSafetyModel(t)
	m.SetStateForTest(tui.StateStreaming)
	m.InputAppendForTest("/compact")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.StateForTest() != tui.StateStreaming {
		t.Fatalf("/compact mid-stream must not change state, got %v", m.StateForTest())
	}
	if cmdContainsQuit(cmd) {
		t.Fatalf("/compact mid-stream must not produce tea.Quit, got: %v", cmd)
	}
	if m.InputValueForTest() != "/compact" {
		t.Fatalf("/compact mid-stream must remain in input box, got %q", m.InputValueForTest())
	}
}
