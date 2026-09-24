package tui_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestAutocompleteIncludesSkillsAtConstruction(t *testing.T) {
	tui.SetSkillRegistry(nil)
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })

	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"research":    {Name: "research", Description: "Investigate a question"},
			"code-review": {Name: "code-review", Description: "Review a diff"},
		},
	}
	tui.SetSkillRegistry(reg)

	m := tui.NewModelForTest()
	items := m.AutocompleteItemsForTest()

	want := map[string]string{
		"research":    "skill",
		"code-review": "skill",
	}
	for _, item := range items {
		expected, ok := want[item.Title]
		if !ok {
			continue
		}
		if item.Category != expected {
			t.Errorf("item %q: category = %q, want %q", item.Title, item.Category, expected)
		}
		if item.Description == "" {
			t.Errorf("item %q: description should not be empty", item.Title)
		}
		delete(want, item.Title)
	}
	for name := range want {
		t.Errorf("autocomplete missing skill /%s", name)
	}
}

func TestAutocompleteBuiltinsStillPresent(t *testing.T) {
	tui.SetSkillRegistry(nil)
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })

	m := tui.NewModelForTest()
	items := m.AutocompleteItemsForTest()

	builtins := map[string]string{
		"quit":   "exit",
		"new":    "session",
		"resume": "session",
	}
	for _, item := range items {
		expected, ok := builtins[item.Title]
		if !ok {
			continue
		}
		if item.Category != expected {
			t.Errorf("builtin %q: category = %q, want %q", item.Title, item.Category, expected)
		}
		delete(builtins, item.Title)
	}
	for name := range builtins {
		t.Errorf("autocomplete missing builtin /%s", name)
	}
}

func TestAutocompleteEmptyWhenRegistryNil(t *testing.T) {
	tui.SetSkillRegistry(nil)
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })

	m := tui.NewModelForTest()
	items := m.AutocompleteItemsForTest()

	skillCount := 0
	for _, item := range items {
		if item.Category == "skill" {
			skillCount++
		}
	}
	if skillCount != 0 {
		t.Errorf("expected zero skill items when registry nil, got %d", skillCount)
	}
}
