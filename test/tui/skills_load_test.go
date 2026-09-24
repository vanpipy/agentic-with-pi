package tui_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestLoadSkillsForCwdPopulatesRegistry(t *testing.T) {
	tmp := t.TempDir()

	homeSkills := filepath.Join(tmp, "skills", "review")
	if err := os.MkdirAll(homeSkills, 0o755); err != nil {
		t.Fatalf("mkdir home skill: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(homeSkills, "SKILL.md"),
		[]byte("---\nname: review\ndescription: Review a diff\n---\nreview body\n"),
		0o644,
	); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	cwdSkills := filepath.Join(tmp, "proj", ".awp", "skills", "ship")
	if err := os.MkdirAll(cwdSkills, 0o755); err != nil {
		t.Fatalf("mkdir project skill: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(cwdSkills, "SKILL.md"),
		[]byte("---\nname: ship\ndescription: Ship it\n---\nship body\n"),
		0o644,
	); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	tui.SetSkillRegistry(nil)
	t.Cleanup(func() { tui.SetSkillRegistry(nil) })

	tui.LoadSkillsForCwdForTest(filepath.Join(tmp, "proj"), tmp)

	reg := tui.SkillRegistryForTest()
	if reg == nil {
		t.Fatal("expected registry to be populated after loadSkillsForCwd")
	}
	if got, want := len(reg.Skills), 2; got != want {
		t.Fatalf("loaded skills = %d, want %d", got, want)
	}
	if _, ok := reg.Skills["review"]; !ok {
		t.Errorf("missing global skill /review")
	}
	if _, ok := reg.Skills["ship"]; !ok {
		t.Errorf("missing project skill /ship")
	}
}
