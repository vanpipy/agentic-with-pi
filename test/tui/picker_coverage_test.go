package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestSessionItemTitleReturnsID(t *testing.T) {
	if got := tui.SessionItemTitleForTest("abc-123"); got != "abc-123" {
		t.Errorf("Title = %q, want %q", got, "abc-123")
	}
}

func TestSessionItemDescriptionWithoutEventsShowsModel(t *testing.T) {
	if got := tui.SessionItemDescriptionForTest("opus-4.7", 0); got != "opus-4.7" {
		t.Errorf("Description(events=0) = %q, want %q", got, "opus-4.7")
	}
}

func TestSessionItemDescriptionWithEventsFormatsCount(t *testing.T) {
	if got := tui.SessionItemDescriptionForTest("opus-4.7", 42); got != "opus-4.7 - 42 events" {
		t.Errorf("Description(events=42) = %q, want %q", got, "opus-4.7 - 42 events")
	}
}

func TestSessionItemFilterValueCombinesIDAndModel(t *testing.T) {
	if got := tui.SessionItemFilterValueForTest("abc", "opus-4.7"); got != "abc opus-4.7" {
		t.Errorf("FilterValue = %q, want %q", got, "abc opus-4.7")
	}
}

func TestSessionItemCategoryIsEmpty(t *testing.T) {
	if got := tui.SessionItemCategoryForTest(); got != "" {
		t.Errorf("Category = %q, want empty", got)
	}
}

func TestPickerShowMakesVisibleAndSelectsFirst(t *testing.T) {
	m := tui.NewModelForTest()
	if m.PickerVisibleForTest() {
		t.Fatalf("picker should not be visible before Show()")
	}
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "sess-1", Model: "opus-4.7", Events: 3},
		{ID: "sess-2", Model: "haiku", Events: 0},
	})
	if !m.PickerVisibleForTest() {
		t.Errorf("picker should be visible after Show()")
	}
	if got := m.PickerCurrentForTest(); got != "sess-1" {
		t.Errorf("Current = %q, want %q (initial selection)", got, "sess-1")
	}
}

func TestPickerNextAndPrevNavigateSelection(t *testing.T) {
	m := tui.NewModelForTest()
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "sess-1", Model: "opus-4.7"},
		{ID: "sess-2", Model: "haiku"},
		{ID: "sess-3", Model: "sonnet"},
	})
	if got := m.PickerCurrentForTest(); got != "sess-1" {
		t.Fatalf("initial selection = %q, want sess-1", got)
	}
	m.PickerNextForTest()
	if got := m.PickerCurrentForTest(); got != "sess-2" {
		t.Errorf("after Next, current = %q, want sess-2", got)
	}
	m.PickerNextForTest()
	if got := m.PickerCurrentForTest(); got != "sess-3" {
		t.Errorf("after second Next, current = %q, want sess-3", got)
	}
	m.PickerPrevForTest()
	if got := m.PickerCurrentForTest(); got != "sess-2" {
		t.Errorf("after Prev, current = %q, want sess-2", got)
	}
}

func TestPickerNextWhenHiddenIsNoop(t *testing.T) {
	m := tui.NewModelForTest()
	if m.PickerVisibleForTest() {
		t.Fatalf("picker should start hidden")
	}
	m.PickerNextForTest()
	if m.PickerVisibleForTest() {
		t.Errorf("Next while hidden must not reveal picker")
	}
	m.PickerPrevForTest()
	if m.PickerVisibleForTest() {
		t.Errorf("Prev while hidden must not reveal picker")
	}
}

func TestPickerCurrentEmptyWhenNothingSelected(t *testing.T) {
	m := tui.NewModelForTest()
	if got := m.PickerCurrentForTest(); got != "" {
		t.Errorf("Current with no items = %q, want empty", got)
	}
}

func TestPickerHideClearsVisible(t *testing.T) {
	m := tui.NewModelForTest()
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "sess-1", Model: "opus-4.7"},
	})
	if !m.PickerVisibleForTest() {
		t.Fatalf("picker should be visible after Show()")
	}
	m.PickerHideForTest()
	if m.PickerVisibleForTest() {
		t.Errorf("picker should be hidden after Hide()")
	}
}

func TestPickerViewWhenHiddenIsEmpty(t *testing.T) {
	m := tui.NewModelForTest()
	if got := m.PickerViewForTest(); got != "" {
		t.Errorf("picker View() when hidden = %q, want empty", got)
	}
}

func TestPickerViewWhenVisibleHasContent(t *testing.T) {
	m := tui.NewModelForTest()
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "sess-xyz", Model: "opus-4.7", Events: 5},
	})
	view := m.PickerViewForTest()
	if view == "" {
		t.Errorf("picker View() when visible should not be empty")
	}
	if !strings.Contains(view, "sess-xyz") {
		t.Errorf("picker view should contain the session id 'sess-xyz', got: %q", view)
	}
}

func TestPickerShowWithEmptyItemsKeepsVisibleButNothingSelected(t *testing.T) {
	m := tui.NewModelForTest()
	m.PickerShowForTest(nil)
	if !m.PickerVisibleForTest() {
		t.Errorf("Show(nil) should still mark picker visible")
	}
	if got := m.PickerCurrentForTest(); got != "" {
		t.Errorf("Current with nil items = %q, want empty", got)
	}
}

func TestPickerReshowClearsFilterText(t *testing.T) {
	m := tui.NewModelForTest()
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "first", Model: "opus"},
		{ID: "second", Model: "haiku"},
	})
	if got := m.PickerCurrentForTest(); got != "first" {
		t.Fatalf("setup: initial current = %q, want first", got)
	}
	m.PickerShowForTest([]tui.PickerItemForTest{
		{ID: "alpha", Model: "sonnet"},
		{ID: "beta", Model: "opus"},
	})
	if got := m.PickerCurrentForTest(); got != "alpha" {
		t.Errorf("after reshow, current = %q, want alpha (first item of new list)", got)
	}
}
