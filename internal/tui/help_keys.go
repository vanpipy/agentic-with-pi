package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

type keyBindings struct {
	quit        key.Binding
	quitEmpty   key.Binding
	complete    key.Binding
	scroll      key.Binding
	showHelp    key.Binding
	submit      key.Binding
}

func defaultKeys() keyBindings {
	return keyBindings{
		quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		quitEmpty: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "quit (empty input)"),
		),
		complete: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "complete /command"),
		),
		scroll: key.NewBinding(
			key.WithKeys("up", "down"),
			key.WithHelp("↑↓", "scroll chat"),
		),
		showHelp: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle full help"),
		),
		submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
	}
}

func (k keyBindings) ShortHelp() []key.Binding {
	return []key.Binding{k.submit, k.complete, k.scroll, k.showHelp, k.quit}
}

func (k keyBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.submit, k.complete},
		{k.scroll, k.showHelp},
		{k.quit, k.quitEmpty},
	}
}

var _ help.KeyMap = keyBindings{}