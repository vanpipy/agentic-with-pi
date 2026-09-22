package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

type keyBindings struct {
	quit         key.Binding
	quitEmpty    key.Binding
	complete     key.Binding
	scroll       key.Binding
	submit       key.Binding
	expandToggle key.Binding
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
		submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		expandToggle: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "expand/collapse"),
		),
	}
}

func (k keyBindings) ShortHelp() []key.Binding {
	return []key.Binding{k.submit, k.complete, k.scroll, k.quit}
}

func (k keyBindings) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.submit, k.complete, k.expandToggle},
		{k.scroll},
		{k.quit, k.quitEmpty},
	}
}

var _ help.KeyMap = keyBindings{}
