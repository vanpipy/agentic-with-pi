package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestCommandItemTitlePrefixesSlash(t *testing.T) {
	if got := tui.CommandItemTitleForTest("quit"); got != "/quit" {
		t.Errorf("Title = %q, want /quit", got)
	}
}

func TestCommandItemCategoryReturnsCategory(t *testing.T) {
	if got := tui.CommandItemCategoryForTest("exit"); got != "exit" {
		t.Errorf("Category = %q, want exit", got)
	}
	if got := tui.CommandItemCategoryForTest(""); got != "" {
		t.Errorf("Category(empty) = %q, want empty", got)
	}
}

func TestCommandItemDescriptionWithoutCategory(t *testing.T) {
	if got := tui.CommandItemDescriptionForTest("", "just description"); got != "just description" {
		t.Errorf("Description(empty category) = %q, want %q", got, "just description")
	}
}

func TestCommandItemDescriptionWithCategoryPrefixesIt(t *testing.T) {
	if got := tui.CommandItemDescriptionForTest("session", "clear chat"); got != "session — clear chat" {
		t.Errorf("Description(category=session) = %q, want %q", got, "session — clear chat")
	}
}

func TestCommandItemFilterValueConcatenatesFields(t *testing.T) {
	got := tui.CommandItemFilterValueForTest("quit", "exit", "exit")
	want := "quit exit exit"
	if got != want {
		t.Errorf("FilterValue = %q, want %q", got, want)
	}
}

func TestAllCommandsForTestContainsBuiltins(t *testing.T) {
	cmds := tui.AllCommandsForTest()
	want := map[string]bool{"quit": false, "new": false, "resume": false, "compact": false, "usage": false, "skills": false}
	for _, c := range cmds {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("allCommandsForTest missing %q", name)
		}
	}
}

func TestAutocompleteUpdateForTestRuns(t *testing.T) {
	m := tui.NewModelForTest()
	_ = m.AutocompleteUpdateForTest(tea.KeyPressMsg{Code: 'a'})
}

func TestAutocompleteSetSizeForTestRuns(t *testing.T) {
	m := tui.NewModelForTest()
	m.AutocompleteSetSizeForTest(80, 24)
	m.AutocompleteSetSizeForTest(0, 0)
}

func TestAutocompleteSetQueryNonSlashHides(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/qu")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after typing /qu")
	}
	m.InputAppendForTest(" more")
	m.AutocompleteRefreshForTest()
	if m.AutocompleteVisibleForTest() {
		t.Errorf("autocomplete should hide when input has trailing arg (setQuery -> !visible)")
	}
}

func TestAutocompleteSetQueryClearsOnNonSlashPrefix(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("foo")
	m.AutocompleteRefreshForTest()
	if m.AutocompleteVisibleForTest() {
		t.Errorf("autocomplete should NOT show when input does not start with /")
	}
}

func TestAutocompleteSetQueryNoOpOnRepeat(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/qu")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after /qu")
	}
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Errorf("autocomplete should remain visible after a no-op setQuery with same value")
	}
}

func TestAutocompleteViewEmptyWhenHidden(t *testing.T) {
	m := tui.NewModelForTest()
	if got := m.AutocompleteViewForTest(); got != "" {
		t.Errorf("AutocompleteView() when hidden = %q, want empty", got)
	}
}

func TestAutocompleteViewShowsItemsWhenVisible(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/qui")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after /qui")
	}
	view := m.AutocompleteViewForTest()
	if view == "" {
		t.Fatalf("autocomplete view should be non-empty when visible")
	}
	if !strings.Contains(stripANSI(view), "quit") {
		t.Errorf("autocomplete view should contain 'quit' (after stripping ANSI), got: %q", stripANSI(view))
	}
}

func TestAcceptAutocompleteNoopWhenNothingSelected(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/zzzzzz-no-match")
	m.AutocompleteRefreshForTest()
	if m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be hidden (no matches)")
	}
	m.AcceptAutocompleteForTest()
	if v := m.InputValueForTest(); v != "/zzzzzz-no-match" {
		t.Errorf("accept on no-match should leave input untouched; got %q", v)
	}
}

func TestAcceptAutocompleteFillsInputAndHides(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/qui")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after /qui")
	}
	m.AcceptAutocompleteForTest()
	if v := m.InputValueForTest(); !strings.HasPrefix(v, "/") || !strings.Contains(v, " ") {
		t.Errorf("accept should rewrite input to '/<title> ', got %q", v)
	}
	if m.AutocompleteVisibleForTest() {
		t.Errorf("accept should hide the autocomplete popup")
	}
}

func TestAutocompleteNoOpRefreshOnSameQuery(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after /")
	}
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Errorf("second refresh with same query should not hide autocomplete (early return path)")
	}
}

func TestAutocompleteNextAndPrevWhileVisible(t *testing.T) {
	m := tui.NewModelForTest()
	m.InputAppendForTest("/")
	m.AutocompleteRefreshForTest()
	if !m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should be visible after /")
	}
	m.AutocompleteNextForTest()
	m.AutocompleteNextForTest()
	m.AutocompletePrevForTest()
}

func TestAutocompleteNextAndPrevWhileHidden(t *testing.T) {
	m := tui.NewModelForTest()
	if m.AutocompleteVisibleForTest() {
		t.Fatalf("setup: autocomplete should start hidden")
	}
	m.AutocompleteNextForTest()
	m.AutocompletePrevForTest()
	if m.AutocompleteVisibleForTest() {
		t.Errorf("Next/Prev on hidden autocomplete must not reveal it")
	}
}
