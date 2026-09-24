package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestSkillsCommandVisualListsEachLoadedSkill(t *testing.T) {
	m := tui.NewModelForTest()

	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"optimize": {
				Name:        "optimize",
				Description: "Optimize Go code",
				Content:     "BODY",
			},
			"review": {
				Name:        "review",
				Description: "Review a change",
				Content:     "REVIEW_BODY",
			},
			"refactor": {
				Name:        "refactor",
				Description: "Refactor a module",
				Content:     "REFACTOR_BODY",
			},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	before := len(m.ChatMessagesForTest())
	quit, cmd := tui.ExecuteCommandForTest(m, "/skills")
	if quit {
		t.Errorf("/skills must not quit")
	}
	if cmd != nil {
		t.Errorf("/skills should return nil cmd (visual), got %T", cmd)
	}

	after := len(m.ChatMessagesForTest())
	if after != before+1 {
		t.Fatalf("/skills should append exactly one chat message (before=%d, after=%d)", before, after)
	}

	visual := m.LastChatMessageForTest()
	for _, want := range []string{"/optimize", "/review", "/refactor"} {
		if !strings.Contains(visual, want) {
			t.Errorf("visual /skills output missing %q, got: %q", want, visual)
		}
	}
}

func TestSkillsCommandVisualMentionsNoSkillsWhenRegistryEmpty(t *testing.T) {
	m := tui.NewModelForTest()
	tui.SetSkillRegistry(nil)
	defer tui.SetSkillRegistry(nil)

	before := len(m.ChatMessagesForTest())
	_, _ = tui.ExecuteCommandForTest(m, "/skills")
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/skills should still append a message when registry is empty (before=%d, after=%d)", before, after)
	}
	visual := m.LastChatMessageForTest()
	if !strings.Contains(strings.ToLower(visual), "no skills") {
		t.Errorf("expected 'no skills' marker when registry is empty, got: %q", visual)
	}
}

func TestSkillsCommandVisualDoesNotSubmitPrompt(t *testing.T) {
	m := tui.NewModelForTest()

	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"foo": {Name: "foo", Description: "d", Content: "BODY"},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	before := len(m.ChatMessagesForTest())
	_, _ = tui.ExecuteCommandForTest(m, "/skills")
	after := len(m.ChatMessagesForTest())

	if after != before+1 {
		t.Fatalf("/skills should append exactly one informational message, got %d", after-before)
	}
	visual := m.LastChatMessageForTest()
	if strings.Contains(visual, "BODY") {
		t.Errorf("/skills is informational and must NOT submit skill body to prompt, got: %q", visual)
	}
}
