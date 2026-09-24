package tui

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func TestRegistryIncludesCompactUsageSkills(t *testing.T) {
	specs := AllCommandSpecsForTest()
	want := map[string]bool{"compact": false, "usage": false, "skills": false}
	for _, s := range specs {
		if _, ok := want[s.Name]; ok {
			want[s.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("registry missing command %q", name)
		}
	}
}

func TestParseCommandCompactWithForceFlag(t *testing.T) {
	parsed, ok := ParseCommandForTest("/compact --force")
	if !ok {
		t.Fatal("parseCommand(/compact --force) returned ok=false")
	}
	if parsed.Name != "compact" {
		t.Errorf("name = %q, want compact", parsed.Name)
	}
	if !strings.Contains(parsed.Arg, "--force") {
		t.Errorf("arg = %q, want it to contain '--force'", parsed.Arg)
	}
}

func TestParseCommandUsage(t *testing.T) {
	parsed, ok := ParseCommandForTest("/usage")
	if !ok {
		t.Fatal("parseCommand(/usage) returned ok=false")
	}
	if parsed.Name != "usage" {
		t.Errorf("name = %q, want usage", parsed.Name)
	}
}

func TestParseCommandSkills(t *testing.T) {
	parsed, ok := ParseCommandForTest("/skills")
	if !ok {
		t.Fatal("parseCommand(/skills) returned ok=false")
	}
	if parsed.Name != "skills" {
		t.Errorf("name = %q, want skills", parsed.Name)
	}
}

func TestSlashUnknownCommandHintIsDynamic(t *testing.T) {
	original := registry
	defer func() { registry = original }()

	registry = []commandSpec{
		{Name: "alpha", Description: "a", Category: "x"},
		{Name: "beta", Description: "b", Category: "x"},
	}

	m := NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, _ := ExecuteCommandForTest(m, "/nosuchcmd")
	if quit {
		t.Errorf("unknown cmd must not quit")
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("unknown command should append a system message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "/alpha") || !strings.Contains(last, "/beta") {
		t.Errorf("dynamic hint should list /alpha and /beta, got: %q", last)
	}
	if strings.Contains(last, "/quit") {
		t.Errorf("hardcoded /quit hint should not appear when registry has no quit, got: %q", last)
	}
}

func TestSlashUsageAppendsUsageHint(t *testing.T) {
	m := NewModelForTest()
	before := len(m.ChatMessagesForTest())
	quit, cmd := ExecuteCommandForTest(m, "/usage")
	if quit {
		t.Errorf("/usage must not quit")
	}
	if cmd != nil {
		t.Errorf("/usage should return nil cmd, got %T", cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/usage should append a system message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if strings.Contains(last, "unknown command") {
		t.Errorf("/usage should be a known command, got: %q", last)
	}
	if !strings.Contains(last, "Usage") {
		t.Errorf("expected 'Usage' marker in /usage output, got: %q", last)
	}
}

func TestSlashSkillsAppendsSkillList(t *testing.T) {
	m := NewModelForTest()
	SetSkillRegistry(&skills.Registry{
		Skills: map[string]*skills.Skill{
			"alpha": {Name: "alpha", Description: "first"},
			"beta":  {Name: "beta", Description: "second"},
		},
	})
	defer SetSkillRegistry(nil)

	before := len(m.ChatMessagesForTest())
	quit, cmd := ExecuteCommandForTest(m, "/skills")
	if quit {
		t.Errorf("/skills must not quit")
	}
	if cmd != nil {
		t.Errorf("/skills should return nil cmd, got %T", cmd)
	}
	after := len(m.ChatMessagesForTest())
	if after <= before {
		t.Fatalf("/skills should append a system message (before=%d, after=%d)", before, after)
	}
	last := m.LastChatMessageForTest()
	if !strings.Contains(last, "/alpha") {
		t.Errorf("expected /alpha in skills list, got: %q", last)
	}
	if !strings.Contains(last, "/beta") {
		t.Errorf("expected /beta in skills list, got: %q", last)
	}
}
