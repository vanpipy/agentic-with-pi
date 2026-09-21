package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type sessionItem struct {
	id        string
	model     string
	startedAt string
	events    int
}

func (s sessionItem) FilterValue() string { return s.id + " " + s.model }
func (s sessionItem) Title() string       { return s.id }
func (s sessionItem) Description() string {
	if s.events == 0 {
		return s.model
	}
	return fmt.Sprintf("%s - %d events", s.model, s.events)
}
func (s sessionItem) Category() string { return "" }

type sessionDelegate struct{}

func (d sessionDelegate) Height() int                         { return 1 }
func (d sessionDelegate) Spacing() int                        { return 0 }
func (d sessionDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d sessionDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	s, ok := item.(sessionItem)
	if !ok {
		return
	}
	marker := "  "
	if index == m.Index() {
		marker = autocompleteCursor.Render("\u25b6 ")
	}
	id := autocompleteHeader.Render(s.id)
	desc := helpFooter.Render(s.Description())
	fmt.Fprintf(w, "%s%s  %s", marker, id, desc)
}

type sessionPickerModel struct {
	visible  bool
	query    string
	list     list.Model
	all      []sessionItem
	fetching bool
}

func newSessionPickerModel() *sessionPickerModel {
	delegate := sessionDelegate{}
	const maxWidth = 80
	const maxHeight = 6
	l := list.New([]list.Item{}, delegate, maxWidth, maxHeight)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(true)
	return &sessionPickerModel{list: l}
}

func (p *sessionPickerModel) Show(sessions []sessionItem) {
	p.all = sessions
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = s
	}
	p.list.SetItems(items)
	p.list.SetFilterText("")
	p.visible = true
	p.list.Select(0)
}

func (p *sessionPickerModel) hide() {
	p.visible = false
}

func (p *sessionPickerModel) setQuery(text string) {
	if !p.visible {
		return
	}
	newQuery := strings.TrimSpace(text)
	if newQuery == p.query {
		return
	}
	p.query = newQuery
	p.list.SetFilterText(newQuery)
	if p.list.Index() >= len(p.list.VisibleItems()) {
		p.list.Select(0)
	}
}

func (p *sessionPickerModel) next() {
	if !p.visible {
		return
	}
	p.list.CursorDown()
}

func (p *sessionPickerModel) prev() {
	if !p.visible {
		return
	}
	p.list.CursorUp()
}

func (p *sessionPickerModel) current() string {
	item, ok := p.list.SelectedItem().(sessionItem)
	if !ok {
		return ""
	}
	return item.id
}

func (p *sessionPickerModel) View() string {
	if !p.visible {
		return ""
	}
	header := autocompleteHeader.Render(" sessions (esc to cancel):") + "\n"
	return header + p.list.View()
}

var _ tea.Cmd