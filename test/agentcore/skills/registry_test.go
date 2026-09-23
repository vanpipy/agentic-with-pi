package agentcore_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func TestRegistryProjectOverridesGlobal(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	// global: alpha = "global alpha"
	writeSkillWith(t, filepath.Join(home, "skills", "alpha"),
		"name: alpha\ndescription: global alpha\n")
	// project: alpha = "project alpha"
	writeSkillWith(t, filepath.Join(cwd, ".awp", "skills", "alpha"),
		"name: alpha\ndescription: project alpha\n")

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	s, ok := reg.Get("alpha")
	if !ok {
		t.Fatalf("alpha not in registry")
	}
	if s.Description != "project alpha" {
		t.Errorf("Description = %q, want project alpha (project must win)", s.Description)
	}
	if s.Source.Origin != skills.OriginProject {
		t.Errorf("Source.Origin = %v, want OriginProject", s.Source.Origin)
	}
}

func TestRegistryCoexistence(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	writeSkill(t, filepath.Join(home, "skills", "global-only"))
	writeSkill(t, filepath.Join(cwd, ".awp", "skills", "project-only"))
	writeSkill(t, filepath.Join(cwd, "agents", "skills", "agents-only"))

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	names := reg.Names()
	sort.Strings(names)
	want := []string{"agents-only", "global-only", "project-only"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestRegistryInvalidFrontmatterAggregated(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	good := filepath.Join(home, "skills", "good")
	writeSkill(t, good)

	bad := filepath.Join(home, "skills", "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\ndescription: only desc\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	if _, ok := reg.Get("good"); !ok {
		t.Errorf("good skill should be loaded")
	}
	if _, ok := reg.Get("bad"); ok {
		t.Errorf("bad skill should NOT be loaded")
	}
	if len(reg.Errors) == 0 {
		t.Fatalf("expected at least one error, got 0")
	}
	found := false
	for _, e := range reg.Errors {
		if errors.Is(e, skills.ErrFrontmatterInvalid) {
			found = true
		}
	}
	if !found {
		t.Errorf("errors = %v, want one wrapping ErrFrontmatterInvalid", reg.Errors)
	}
}

func TestRegistryDuplicateWithinSameOriginKeepsFirst(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	dirA := filepath.Join(home, "skills", "alpha")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: first\n---\nfirst\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirB := filepath.Join(home, "skills", "beta")
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: second\n---\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	s, ok := reg.Get("alpha")
	if !ok {
		t.Fatalf("alpha not loaded")
	}
	if s.Description != "first" {
		t.Errorf("Description = %q, want first (first-wins in same origin)", s.Description)
	}
	if len(reg.Errors) == 0 {
		t.Fatalf("expected duplicate-in-origin error, got 0")
	}
}

func TestRegistryGlobalAndProjectSameNameProjectWinsNoError(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	writeSkillWith(t, filepath.Join(home, "skills", "shared"),
		"name: shared\ndescription: from global\n")
	writeSkillWith(t, filepath.Join(cwd, ".awp", "skills", "shared"),
		"name: shared\ndescription: from project\n")

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	s, _ := reg.Get("shared")
	if s.Description != "from project" {
		t.Errorf("Description = %q, want from project", s.Description)
	}
	if len(reg.Errors) != 0 {
		t.Errorf("project-overriding-global should NOT add errors, got %v", reg.Errors)
	}
}

func TestRegistryNamesIsSortedAndStable(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	for _, n := range []string{"zeta", "alpha", "mu"} {
		writeSkill(t, filepath.Join(home, "skills", n))
	}

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	names := reg.Names()
	want := []string{"alpha", "mu", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestRegistryEmptyWhenNoRoots(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	if len(reg.Names()) != 0 {
		t.Errorf("Names = %v, want empty", reg.Names())
	}
}

func TestRegistryErrorMessageMentionsSkillName(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	dir := filepath.Join(home, "skills", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\ndescription: only desc\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	if len(reg.Errors) == 0 {
		t.Fatalf("no errors")
	}
	hasName := false
	for _, e := range reg.Errors {
		if strings.Contains(e.Error(), "broken") {
			hasName = true
		}
	}
	if !hasName {
		t.Errorf("expected error to mention 'broken', got %v", reg.Errors)
	}
}

func writeSkillWith(t *testing.T, dir, fmBody string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	front := "---\n" + fmBody + "---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(front), 0o644); err != nil {
		t.Fatal(err)
	}
}
