package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type commandItem struct {
	title       string
	description string
	category    string
}

func (c commandItem) FilterValue() string { return c.title }
func (c commandItem) Title() string       { return "/" + c.title }
func (c commandItem) Description() string { return c.description }
func (c commandItem) Category() string    { return c.category }

type commandDelegate struct{}

func (d commandDelegate) Height() int                         { return 1 }
func (d commandDelegate) Spacing() int                        { return 0 }
func (d commandDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d commandDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	c, ok := item.(commandItem)
	if !ok {
		return
	}
	marker := "  "
	if index == m.Index() {
		marker = autocompleteCursor.Render("▶ ")
	}
	name := autocompleteHeader.Render("/" + c.title)
	desc := helpFooter.Render(c.description)
	cat := autocompleteCategory.Render(c.category)
	fmt.Fprintf(w, "%s%s  %-7s  %s", marker, name, cat, desc)
}

type autocompleteModel struct {
	visible       bool
	query         string
	list          list.Model
	all           []commandItem
}

func newAutocompleteModel() *autocompleteModel {
	all := commandsToItems()
	const maxWidth = 80
	const maxHeight = 6
	delegate := commandDelegate{}
	l := list.New(toCommandItems(all), delegate, maxWidth, maxHeight)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(true)
	return &autocompleteModel{list: l, all: all}
}

func toCommandItems(items []commandItem) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func (a *autocompleteModel) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := a.list.Update(msg)
	a.list = updated
	return cmd
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
		a.visible = false
		return
	}
	a.query = query
	a.list.SetFilterText(query)
	visible := a.list.VisibleItems()
	a.visible = len(visible) > 0 && len(parts) == 1
	if a.visible && a.list.Index() >= len(visible) {
		a.list.Select(0)
	}
}

func (a *autocompleteModel) hide() {
	a.visible = false
}

func (a *autocompleteModel) SetSize(width, height int) {
}

func (a *autocompleteModel) next() {
	if !a.visible {
		return
	}
	a.list.CursorDown()
}

func (a *autocompleteModel) prev() {
	if !a.visible {
		return
	}
	a.list.CursorUp()
}

func (a *autocompleteModel) current() string {
	item, ok := a.list.SelectedItem().(commandItem)
	if !ok {
		return ""
	}
	return item.title
}

func (a *autocompleteModel) View() string {
	if !a.visible {
		return ""
	}
	header := autocompleteHeader.Render(" commands:") + "\n"
	return header + a.list.View()
}

func allCommands() []string {
	cmds := make([]string, 0, len(allCommandSpecs()))
	for _, c := range allCommandSpecs() {
		cmds = append(cmds, c.Name)
	}
	return cmds
}

func commandsToItems() []commandItem {
	specs := allCommandSpecs()
	out := make([]commandItem, len(specs))
	for i, s := range specs {
		out[i] = commandItem{
			title:       s.Name,
			description: s.Description,
			category:    s.Category,
		}
	}
	return out
}

func (m *Model) acceptAutocomplete() {
	cmd := m.autocomplete.current()
	if cmd == "" {
		return
	}
	m.input.Reset()
	m.input.SetValue("/" + cmd + " ")
	m.autocomplete.visible = false
}

var _ tea.Cmd
