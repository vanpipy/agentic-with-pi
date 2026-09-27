package tui

import (
	"fmt"

	"charm.land/bubbles/v2/list"
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

type sessionPickerModel struct {
	visible  bool
	list     list.Model
	all      []sessionItem
	fetching bool
}

func newSessionPickerModel() *sessionPickerModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles = list.NewDefaultItemStyles(true)
	const maxWidth = 80
	maxHeight := popupItemCount * 2
	l := list.New([]list.Item{}, delegate, maxWidth, maxHeight)
	l.Styles = list.DefaultStyles(true)
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
	return p.list.View()
}

func SessionItemTitleForTest(id string) string {
	return sessionItem{id: id}.Title()
}

func SessionItemDescriptionForTest(model string, events int) string {
	return sessionItem{model: model, events: events}.Description()
}

func SessionItemFilterValueForTest(id, model string) string {
	return sessionItem{id: id, model: model}.FilterValue()
}

func SessionItemCategoryForTest() string {
	return sessionItem{}.Category()
}

type PickerItemForTest struct {
	ID        string
	Model     string
	StartedAt string
	Events    int
}

func (m *Model) PickerShowForTest(items []PickerItemForTest) {
	si := make([]sessionItem, len(items))
	for i, it := range items {
		si[i] = sessionItem{id: it.ID, model: it.Model, startedAt: it.StartedAt, events: it.Events}
	}
	m.picker.Show(si)
}

func (m *Model) PickerHideForTest() { m.picker.hide() }
func (m *Model) PickerNextForTest() { m.picker.next() }
func (m *Model) PickerPrevForTest() { m.picker.prev() }

func (m *Model) PickerCurrentForTest() string { return m.picker.current() }
