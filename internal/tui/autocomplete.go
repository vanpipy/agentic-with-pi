package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type autocompleteModel struct {
	visible bool
	query   string
	cursor  int
	items   []string
	width   int
}

func newAutocompleteModel() *autocompleteModel {
	return &autocompleteModel{}
}

func (a *autocompleteModel) Update(msg tea.Msg) tea.Cmd {
	return nil
}

func (a *autocompleteModel) setQuery(text string) {
	if !strings.HasPrefix(text, "/") {
		a.visible = false
		return
	}
	body := strings.TrimPrefix(text, "/")
	parts := strings.SplitN(body, " ", 2)
	query := parts[0]
	if query == "" {
		a.items = allCommands()
		a.cursor = 0
		a.visible = len(a.items) > 0
		return
	}
	matches := []string{}
	for _, cmd := range allCommands() {
		if strings.HasPrefix(cmd, query) {
			matches = append(matches, cmd)
		}
	}
	a.items = matches
	a.cursor = 0
	a.visible = len(matches) > 0 && len(parts) == 1
	a.query = query
}

func (a *autocompleteModel) next() {
	if !a.visible || len(a.items) == 0 {
		return
	}
	a.cursor = (a.cursor + 1) % len(a.items)
}

func (a *autocompleteModel) prev() {
	if !a.visible || len(a.items) == 0 {
		return
	}
	a.cursor--
	if a.cursor < 0 {
		a.cursor = len(a.items) - 1
	}
}

func (a *autocompleteModel) current() string {
	if !a.visible || a.cursor >= len(a.items) {
		return ""
	}
	return a.items[a.cursor]
}

func (a *autocompleteModel) View() string {
	if !a.visible || len(a.items) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, " commands:")
	for i, cmd := range a.items {
		cursor := "  "
		if i == a.cursor {
			cursor = "▶ "
		}
		def, _ := findCommand(cmd)
		line := fmt.Sprintf("%s/%s   %s", cursor, cmd, def.description)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func allCommands() []string {
	cmds := make([]string, 0, len(builtinCommands()))
	for k := range builtinCommands() {
		cmds = append(cmds, k)
	}
	return cmds
}

func (m *Model) acceptAutocomplete() {
	cmd := m.autocomplete.current()
	if cmd == "" {
		return
	}
	m.input.Reset()
	m.input.ti.SetValue("/" + cmd + " ")
	m.autocomplete.visible = false
}

var _ tea.Cmd
