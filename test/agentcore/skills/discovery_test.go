package agentcore_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func skillBodyFor(name string) string {
	return "name: " + name + "\ndescription: d\n"
}

func skillBody() string {
	return skillBodyFor("x")
}

func writeSkill(t *testing.T, dir string) {
	t.Helper()
	name := filepath.Base(dir)
	writeSkillWith(t, dir, skillBodyFor(name))
}

func TestDiscoverAllThreeRoots(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	writeSkill(t, filepath.Join(home, "skills", "alpha"))
	writeSkill(t, filepath.Join(cwd, ".awp", "skills", "beta"))
	writeSkill(t, filepath.Join(cwd, "agents", "skills", "gamma"))

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}

	names := make([]string, 0, len(got))
	for _, s := range got {
		names = append(names, filepath.Base(filepath.Dir(s.Path))+"@"+s.Origin.String())
	}
	sort.Strings(names)

	want := []string{
		"alpha@global",
		"beta@project",
		"gamma@agents",
	}
	if len(names) != len(want) {
		t.Fatalf("discovered %d sources, want %d: %v", len(names), len(want), names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("discovered[%d] = %q, want %q (full list %v)", i, n, want[i], names)
		}
	}
}

func TestDiscoverMissingRootsAreSkipped(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	writeSkill(t, filepath.Join(home, "skills", "onlyglobal"))

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1", len(got))
	}
	if got[0].Origin != skills.OriginGlobal {
		t.Errorf("origin = %v, want global", got[0].Origin)
	}
}

func TestDiscoverSkipsDirWithoutSkillMD(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	dir := filepath.Join(home, "skills", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeSkill(t, filepath.Join(home, "skills", "good"))

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (broken dir should be skipped). paths=%v", len(got), pathsOf(got))
	}
}

func TestDiscoverGlobalPathUsesHomeDir(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	writeSkill(t, filepath.Join(home, "skills", "homed"))

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	want := filepath.Join(home, "skills", "homed", "SKILL.md")
	if len(got) != 1 || got[0].Path != want {
		t.Fatalf("got %v, want path %q", got, want)
	}
}

func TestDiscoverSkipsFileInsteadOfDirAtSkillRoot(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	root := filepath.Join(home, "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "loose.md"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0 (loose file at root should be skipped). paths=%v", len(got), pathsOf(got))
	}
}

func pathsOf(ss []skills.SkillSource) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Path)
	}
	return out
}
