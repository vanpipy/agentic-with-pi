package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

type inputModel struct {
	ta textarea.Model
}

func newInputModel() *inputModel {
	ta := textarea.New()
	ta.Placeholder = "type a prompt, Enter to send, Shift+Enter for newline, / for commands, ? for help"
	ta.Focus()
	ta.CharLimit = 4096
	ta.SetWidth(80)
	ta.SetHeight(5)
	ta.MaxHeight = 5
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+j"),
		key.WithHelp("shift+enter", "newline"),
	)
	return &inputModel{ta: ta}
}

func (i *inputModel) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := i.ta.Update(msg)
	i.ta = updated
	return cmd
}

func (i *inputModel) View() string {
	return i.ta.View()
}

func (i *inputModel) Value() string {
	return strings.TrimRight(i.ta.Value(), "\n")
}

func (i *inputModel) Reset() {
	i.ta.Reset()
}

func (i *inputModel) Focus() tea.Cmd {
	return i.ta.Focus()
}

func (i *inputModel) Blur() {
	i.ta.Blur()
}

func (i *inputModel) SetValue(v string) {
	i.ta.SetValue(v)
}

func (i *inputModel) SetWidth(w int) {
	i.ta.SetWidth(w)
}