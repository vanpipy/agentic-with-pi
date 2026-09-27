package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestStartResumeForTestReturnsNilWhenConnNil(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetConnForTest(nil)
	cmd := tui.StartResumeForTest(m, "sess-x")
	if cmd != nil {
		t.Errorf("startResume with nil conn should return nil cmd, got %v", cmd)
	}
}

func TestShowSessionPickerForTestReturnsNilWhenConnNil(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetConnForTest(nil)
	cmd := tui.ShowSessionPickerForTest(m)
	if cmd != nil {
		t.Errorf("showSessionPicker with nil conn should return nil cmd, got %v", cmd)
	}
}

func TestRunCompactForTestReturnsNilAndAppendsErrorWhenConnNil(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetConnForTest(nil)
	before := len(m.ChatMessagesForTest())
	cmd := tui.RunCompactForTest(m, true)
	if cmd != nil {
		t.Errorf("runCompact with nil conn should return nil cmd, got %v", cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("runCompact with nil conn should append an error message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "no active connection") {
		t.Errorf("runCompact error message should mention 'no active connection', got: %q", last)
	}
}

func TestRunCompactForTestWithoutForce(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetConnForTest(nil)
	cmd := tui.RunCompactForTest(m, false)
	if cmd != nil {
		t.Errorf("runCompact(force=false) with nil conn should return nil cmd, got %v", cmd)
	}
}

func TestExecuteCommandRejectsNonSlashInput(t *testing.T) {
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, cmd := tui.ExecuteCommandForTest(m, "hello world no slash")
	if quit {
		t.Errorf("non-slash input must not quit")
	}
	if cmd != nil {
		t.Errorf("non-slash input must return nil cmd, got %v", cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after != before {
		t.Errorf("non-slash input must not append a chat message (before=%d, after=%d)", before, after)
	}
}

func TestExecuteCommandEmptyStringIsNonSlash(t *testing.T) {
	m := tui.NewModelForTest()
	quit, _ := tui.ExecuteCommandForTest(m, "")
	if quit {
		t.Errorf("empty string must not quit")
	}
}

func TestExecuteCommandSlashOnlyIsNonSlash(t *testing.T) {
	m := tui.NewModelForTest()
	quit, cmd := tui.ExecuteCommandForTest(m, "/")
	if quit {
		t.Errorf("/ alone must not quit (parseCommand returns ok=false when body is empty)")
	}
	if cmd != nil {
		t.Errorf("/ alone must return nil cmd, got %v", cmd)
	}
}

func TestMatchSkillForNilRegistryReturnsFalse(t *testing.T) {
	tui.SetSkillRegistry(nil)
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, _ := tui.ExecuteCommandForTest(m, "/anything-not-builtin")
	if quit {
		t.Errorf("unknown command with nil registry must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("unknown command should append unknown-cmd hint (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "unknown command") {
		t.Errorf("expected 'unknown command' marker, got: %q", last)
	}
}

func TestMatchSkillForResolveInvocationFailsThenUnknownHint(t *testing.T) {
	tui.SetSkillRegistry(&skills.Registry{
		Skills: map[string]*skills.Skill{
			"unrelated": {Name: "unrelated", Description: "x"},
		},
	})
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })

	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, _ := tui.ExecuteCommandForTest(m, "/does-not-prefix-unrelated")
	if quit {
		t.Errorf("must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("no-match should append unknown hint (before=%d, after=%d)", before, after)
	}
}

func TestMatchSkillForGetFailsThenUnknownHint(t *testing.T) {
	tui.SetSkillRegistry(&skills.Registry{
		Skills: map[string]*skills.Skill{},
	})
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })

	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	_, _ = tui.ExecuteCommandForTest(m, "/somefakecmd")
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Errorf("empty registry should fall through to unknown hint (before=%d, after=%d)", before, after)
	}
}

func TestLoadSkillsForCwdForTestWithValidPathsPopulatesRegistry(t *testing.T) {
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })
	tui.SetSkillRegistry(nil)
	tui.LoadSkillsForCwdForTest(".", "")
	reg := tui.SkillRegistryForTest()
	if reg == nil {
		t.Errorf("LoadSkillsForCwdForTest with valid paths should produce a non-nil registry")
	}
}

func TestLoadSkillsForCwdForTestWithEmptyTempDir(t *testing.T) {
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })
	tui.SetSkillRegistry(nil)
	tmp := t.TempDir()
	tui.LoadSkillsForCwdForTest(tmp, tmp)
	reg := tui.SkillRegistryForTest()
	if reg == nil {
		t.Errorf("LoadSkillsForCwdForTest with empty temp dirs should produce a non-nil (empty) registry")
	}
}

func TestRenderSkillsListForNilRegistryViaSkillsCmd(t *testing.T) {
	tui.SetSkillRegistry(nil)
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, cmd := tui.ExecuteCommandForTest(m, "/skills")
	if quit {
		t.Errorf("/skills must not quit")
	}
	if cmd != nil {
		t.Errorf("/skills should return nil cmd, got %v", cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/skills should append a system message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "no skills") {
		t.Errorf("nil registry should produce 'no skills' marker, got: %q", last)
	}
}

func TestRenderSkillsListForEmptyRegistryViaSkillsCmd(t *testing.T) {
	tui.SetSkillRegistry(&skills.Registry{Skills: map[string]*skills.Skill{}})
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, _ := tui.ExecuteCommandForTest(m, "/skills")
	if quit {
		t.Errorf("/skills must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/skills should append a system message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "no skills") {
		t.Errorf("empty registry should produce 'no skills' marker, got: %q", last)
	}
}

func TestRenderSkillsListFallsBackToNameWhenDescriptionEmpty(t *testing.T) {
	tui.SetSkillRegistry(&skills.Registry{
		Skills: map[string]*skills.Skill{
			"no-desc": {Name: "no-desc", Description: ""},
		},
	})
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	_, _ = tui.ExecuteCommandForTest(m, "/skills")
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/skills should append a system message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "/no-desc") {
		t.Errorf("skill list should include /no-desc entry, got: %q", last)
	}
	if strings.Contains(last, "—") {
		t.Errorf("skill list should not include em-dash for description-less skills, got: %q", last)
	}
}

func TestLastChatMessageForTestOnEmptyChat(t *testing.T) {
	m := tui.NewModelForTest()
	if got := m.LastChatMessageForTest(); got != "" {
		t.Errorf("LastChatMessageForTest on empty chat = %q, want empty", got)
	}
}

func TestLastPromptForTestOnNilModel(t *testing.T) {
	tui.SetLastPromptForTest(nil, "ignored")
	if got := (*tui.Model)(nil).LastPromptForTest(); got != "" {
		t.Errorf("LastPromptForTest on nil receiver = %q, want empty", got)
	}
}

func TestChatMessagesForTestOnNilModel(t *testing.T) {
	if msgs := tui.ChatMessagesForTest(nil); msgs != nil {
		t.Errorf("ChatMessagesForTest(nil) should return nil, got %v", msgs)
	}
}

func TestShowUsageAppendsNoSessionHint(t *testing.T) {
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, cmd := tui.ExecuteCommandForTest(m, "/usage")
	if quit || cmd != nil {
		t.Errorf("/usage should not quit and return nil cmd, got quit=%v cmd=%v", quit, cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/usage should append (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "no active session") {
		t.Errorf("/usage with no session should mention 'no active session', got: %q", last)
	}
}

func TestShowUsageAppendsSessionSpecificHint(t *testing.T) {
	m := tui.NewModelForTest()
	tui.SetSessionForTest(m, "deadbeef-1234")
	before := len(m.ChatMessagesForTest())
	quit, cmd := tui.ExecuteCommandForTest(m, "/usage")
	if quit || cmd != nil {
		t.Errorf("/usage should not quit and return nil cmd, got quit=%v cmd=%v", quit, cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/usage should append (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "deadbeef-1234") {
		t.Errorf("/usage with session should include session id, got: %q", last)
	}
	if strings.Contains(last, "no active session") {
		t.Errorf("/usage with session should NOT show 'no active session', got: %q", last)
	}
}

func TestSetSessionForTestNilModelSafe(t *testing.T) {
	tui.SetSessionForTest(nil, "anything")
}

func TestExecuteSkillRendersContentAndPrompt(t *testing.T) {
	tui.SetSkillRegistry(&skills.Registry{
		Skills: map[string]*skills.Skill{
			"with-prompt": {Name: "with-prompt", Description: "x", Content: "BODY"},
		},
	})
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, _ := tui.ExecuteCommandForTest(m, "/with-prompt arg")
	if quit {
		t.Errorf("skill invocation must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after != before+1 {
		t.Errorf("skill invocation should append exactly one message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "BODY") {
		t.Errorf("skill content missing in chat, got: %q", last)
	}
	if !strings.Contains(last, "arg") {
		t.Errorf("user arg missing in chat, got: %q", last)
	}
}

func TestExecuteSkillEmptyRenderedIsNoop(t *testing.T) {
	tui.SetSkillRegistry(&skills.Registry{
		Skills: map[string]*skills.Skill{
			"empty": {Name: "empty", Description: "x", Content: ""},
		},
	})
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })
	m := tui.NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, _ := tui.ExecuteCommandForTest(m, "/empty ")
	if quit {
		t.Errorf("must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after != before {
		t.Errorf("empty rendered prompt must not append chat (before=%d, after=%d)", before, after)
	}
}
