package agent_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/vanpiyp/awp/internal/tui"
)

func pressEnter(m *tui.Model) *tui.Model {
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return updated.(*tui.Model)
}

func typeChars(m *tui.Model, text string) *tui.Model {
	for _, r := range text {
		updated, _ := m.Update(tea.KeyPressMsg{Text: string(r)})
		m = updated.(*tui.Model)
	}
	return m
}

func TestSlashHelpSetsShowHelp(t *testing.T) {
	m := tui.NewModelForTest()
	m = typeChars(m, "/help")
	m = pressEnter(m)
	if !m.ShowHelpForTest() {
		t.Fatal("/help did not set showHelp")
	}
}

func TestSlashHelpClosesAutocomplete(t *testing.T) {
	m := tui.NewModelForTest()
	m = typeChars(m, "/help")
	if !m.AutocompleteVisibleForTest() {
		t.Fatal("autocomplete should be visible while typing /help")
	}
	m = pressEnter(m)
	if m.AutocompleteVisibleForTest() {
		t.Fatal("autocomplete should hide after Enter executes /help")
	}
}

func TestSlashHelpRendersFullHelpBlock(t *testing.T) {
	m := tui.NewModelForTest()
	m = typeChars(m, "/help")
	m = pressEnter(m)
	v := m.View()
	plain := ansi.Strip(v.Content)
	if !strings.Contains(plain, "ctrl+c") || !strings.Contains(plain, "quit") {
		t.Errorf("FullHelpView missing ctrl+c/quit. Got:\n%s", plain)
	}
}

func TestSlashHelpDoubleRenderFix(t *testing.T) {
	m := tui.NewModelForTest()
	m = typeChars(m, "/help")
	m = pressEnter(m)
	v := m.View()
	plain := ansi.Strip(v.Content)
	if strings.Count(plain, "ctrl+c") > 1 {
		t.Errorf("ctrl+c rendered %d times - help block duplicated", strings.Count(plain, "ctrl+c"))
	}
}

func TestQuestionMarkTogglesHelp(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.KeyPressMsg{Text: "?"})
	m = out.(*tui.Model)
	if !m.ShowHelpForTest() {
		t.Fatal("? did not set showHelp")
	}
}

func TestQuestionMarkTwiceClosesHelp(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.KeyPressMsg{Text: "?"})
	m = out.(*tui.Model)
	out, _ = m.Update(tea.KeyPressMsg{Text: "?"})
	m = out.(*tui.Model)
	if m.ShowHelpForTest() {
		t.Fatal("double ? should close help")
	}
}

func TestEscClosesHelp(t *testing.T) {
	m := tui.NewModelForTest()
	m = typeChars(m, "/help")
	m = pressEnter(m)
	if !m.ShowHelpForTest() {
		t.Fatal("/help should set showHelp")
	}
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = out.(*tui.Model)
	if m.ShowHelpForTest() {
		t.Fatal("esc should close help")
	}
}