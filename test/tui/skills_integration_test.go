package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestSlashQuitUnchangedAfterSkillWiring(t *testing.T) {
	m := tui.NewModelForTest()
	quit, _ := tui.ExecuteCommandForTest(m, "/quit")
	if !quit {
		t.Errorf("/quit must set quit=true")
	}
}

func TestSlashNewUnchangedAfterSkillWiring(t *testing.T) {
	m := tui.NewModelForTest()
	quit, _ := tui.ExecuteCommandForTest(m, "/new")
	if quit {
		t.Errorf("/new must NOT quit (unchanged behavior)")
	}
}

func TestSlashUnknownCommandStillErrorsAfterSkillWiring(t *testing.T) {
	m := tui.NewModelForTest()
	tui.SetSkillRegistry(nil)

	before := len(m.ChatMessagesForTest())
	quit, _ := tui.ExecuteCommandForTest(m, "/nosuchcmd")
	if quit {
		t.Errorf("unknown cmd must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("unknown command should append an error/system message to chat (before=%d, after=%d)", before, after)
	}
}

func TestSkillMatchSubmitsRenderedPrompt(t *testing.T) {
	m := tui.NewModelForTest()
	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"optimize": {
				Name:        "optimize",
				Description: "Optimize Go code",
				Content:     "You are a code optimizer. Reply with concrete patches only.",
			},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	before := len(m.ChatMessagesForTest())

	quit, _ := tui.ExecuteCommandForTest(m, "/optimize my code")

	if quit {
		t.Errorf("skill command must NOT quit")
	}
	after := len(m.ChatMessagesForTest())
	if after != before+1 {
		t.Fatalf("skill invocation should submit exactly one new chat message (before=%d, after=%d)", before, after)
	}

	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "code optimizer") {
		t.Errorf("expected skill content in submitted prompt, got: %q", last)
	}
	if !strings.Contains(last, "my code") {
		t.Errorf("expected user arg in submitted prompt, got: %q", last)
	}
}

func TestSkillMatchQuotedPromptUnquoted(t *testing.T) {
	m := tui.NewModelForTest()
	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"foo": {Name: "foo", Description: "d", Content: "CONTENT"},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	_, _ = tui.ExecuteCommandForTest(m, `/foo "quoted prompt"`)

	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "CONTENT") {
		t.Errorf("expected skill content, got: %q", last)
	}
	if !strings.Contains(last, "quoted prompt") {
		t.Errorf("expected unquoted user arg, got: %q", last)
	}
	if strings.Contains(last, `"quoted prompt"`) {
		t.Errorf("quotes should be stripped, got: %q", last)
	}
}

func TestSkillSetsLastPromptToRendered(t *testing.T) {
	m := tui.NewModelForTest()
	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"foo": {Name: "foo", Description: "d", Content: "BODY"},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	tui.SetLastPromptForTest(m, "")

	_, _ = tui.ExecuteCommandForTest(m, "/foo whatever")

	want := "[skill: foo]\n\nBODY\n\nwhatever"
	if got := m.LastPromptForTest(); got != want {
		t.Errorf("lastPrompt = %q, want %q", got, want)
	}
}

func TestSkillSetsLastPromptWithoutUserArg(t *testing.T) {
	m := tui.NewModelForTest()
	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"foo": {Name: "foo", Description: "d", Content: "BODY"},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	tui.SetLastPromptForTest(m, "")

	_, _ = tui.ExecuteCommandForTest(m, "/foo")

	want := "[skill: foo]\n\nBODY"
	if got := m.LastPromptForTest(); got != want {
		t.Errorf("lastPrompt = %q, want %q", got, want)
	}
}

func TestSkillLongestPrefixPreferredOverBuiltinMiss(t *testing.T) {
	m := tui.NewModelForTest()
	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"foo":      {Name: "foo", Content: "SHORT"},
			"foo deep": {Name: "foo deep", Content: "LONG"},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	_, _ = tui.ExecuteCommandForTest(m, "/foo deep my prompt")

	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "LONG") {
		t.Errorf("expected longest-prefix skill (foo deep), got: %q", last)
	}
	if strings.Contains(last, "SHORT") {
		t.Errorf("short-prefix skill should not have won, got: %q", last)
	}
}

func TestSkillMatchWithoutUserArgSubmitsContentOnly(t *testing.T) {
	m := tui.NewModelForTest()
	reg := &skills.Registry{
		Skills: map[string]*skills.Skill{
			"foo": {Name: "foo", Content: "ONLY_CONTENT"},
		},
	}
	tui.SetSkillRegistry(reg)
	defer tui.SetSkillRegistry(nil)

	_, _ = tui.ExecuteCommandForTest(m, "/foo")

	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "ONLY_CONTENT") {
		t.Errorf("expected skill content even without arg, got: %q", last)
	}
}
