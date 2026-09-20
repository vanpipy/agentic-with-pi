package tui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type inputModel struct {
	ti textinput.Model
}

func newInputModel() *inputModel {
	ti := textinput.New()
	ti.Placeholder = "type a prompt, press Enter to send..."
	ti.Focus()
	ti.CharLimit = 4096
	ti.SetWidth(80)
	return &inputModel{ti: ti}
}

func (i *inputModel) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := i.ti.Update(msg)
	i.ti = updated
	return cmd
}

func (i *inputModel) View() string {
	return i.ti.View()
}

func (i *inputModel) Value() string {
	return i.ti.Value()
}

func (i *inputModel) Reset() {
	i.ti.Reset()
}

func (i *inputModel) Focus() tea.Cmd {
	return i.ti.Focus()
}

func (i *inputModel) Blur() {
	i.ti.Blur()
}

func (i *inputModel) SetValue(v string) {
	i.ti.SetValue(v)
}
