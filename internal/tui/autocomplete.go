package tui

import (
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

type commandItem struct {
	title       string
	description string
	category    string
}

const popupItemCount = 3

func (c commandItem) FilterValue() string { return c.title + " " + c.description + " " + c.category }
func (c commandItem) Title() string       { return "/" + c.title }
func (c commandItem) Description() string {
	if c.category == "" {
		return c.description
	}
	return c.category + " — " + c.description
}
func (c commandItem) Category() string { return c.category }

type autocompleteModel struct {
	visible bool
	query   string
	list    list.Model
	all     []commandItem
}

func newAutocompleteModel() *autocompleteModel {
	all := commandsToItems()
	const maxWidth = 80
	maxHeight := popupItemCount * 2
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles = list.NewDefaultItemStyles(true)
	l := list.New(toCommandItems(all), delegate, maxWidth, maxHeight)
	l.Styles = list.DefaultStyles(true)
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
	if len(parts) > 1 {
		a.visible = false
		return
	}
	newQuery := parts[0]
	if newQuery == a.query && a.visible {
		return
	}
	a.query = newQuery
	a.list.SetFilterText(a.query)
	visible := a.list.VisibleItems()
	a.visible = len(visible) > 0
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
	return a.list.View()
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
	out := make([]commandItem, 0, len(specs)+8)
	for _, s := range specs {
		out = append(out, commandItem{
			title:       s.Name,
			description: s.Description,
			category:    s.Category,
		})
	}
	if reg := skillRegistry; reg != nil {
		for _, name := range reg.Names() {
			s, ok := reg.Get(name)
			if !ok {
				continue
			}
			out = append(out, commandItem{
				title:       name,
				description: s.Description,
				category:    "skill",
			})
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
