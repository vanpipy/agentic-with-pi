package agentcore_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestAutocompleteFuzzyFiltersSlashCommands(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = out.(*tui.Model)

	for _, r := range "/ne" {
		out, _ := m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	if !m.AutocompleteVisibleForTest() {
		t.Fatal("/ne should show autocomplete popup")
	}
	v := m.View()
	plain := stripANSI(v.Content)
	if !strings.Contains(plain, "/new") {
		t.Errorf("autocomplete should list /new. Got:\n%s", plain)
	}
	if strings.Contains(plain, "/quit") {
		t.Errorf("autocomplete should NOT list /quit for query 'ne'. Got:\n%s", plain)
	}
}

func TestAutocompleteAcceptsTabCompletion(t *testing.T) {
	m := tui.NewModelForTest()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = out.(*tui.Model)

	for _, r := range "/ne" {
		out, _ := m.Update(tea.KeyPressMsg{Text: string(r)})
		m = out.(*tui.Model)
	}
	out, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = out.(*tui.Model)
	value := m.InputValueForTest()
	if value != "/new " {
		t.Errorf("tab should accept /new completion, got %q", value)
	}
	if m.AutocompleteVisibleForTest() {
		t.Errorf("autocomplete should hide after tab accept")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if r == 'm' || r == 'K' || r == 'H' {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
