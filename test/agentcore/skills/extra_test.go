package agentcore_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func TestDiscoverDefaultsHomeFromPathsHome(t *testing.T) {
	t.Setenv("AWP_HOME", "")
	t.Setenv("AWP_CONFIG_HOME", "")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cwd := t.TempDir()
	writeSkill(t, filepath.Join(home, ".awp", "skills", "fromDefaultHome"))

	got, err := skills.DiscoverSkills(cwd, "")
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("got 0 sources, want ≥1")
	}
	var found bool
	for _, s := range got {
		if strings.Contains(s.Path, filepath.Join(".awp", "skills", "fromDefaultHome", "SKILL.md")) {
			if s.Origin != skills.OriginGlobal {
				t.Errorf("Origin = %v, want OriginGlobal", s.Origin)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find skill from default home; got %v", pathsOf(got))
	}
}

func TestDiscoverDefaultsCwdFromOSWhenEmpty(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/awp/home")

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalCwd) })

	writeSkill(t, filepath.Join(dir, ".awp", "skills", "alpha"))

	got, err := skills.DiscoverSkills("", "/custom/awp/home")
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1; paths=%v", len(got), pathsOf(got))
	}
	if got[0].Origin != skills.OriginProject {
		t.Errorf("Origin = %v, want OriginProject", got[0].Origin)
	}
}

func TestDiscoverGetwdError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate CWD deletion when running as root")
	}

	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	subdir := filepath.Join(dir, "soon-gone")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(subdir); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(originalCwd)
	})

	if _, err := skills.DiscoverSkills("", "/home"); err == nil {
		t.Fatal("expected DiscoverSkills to fail when CWD is invalid")
	}
}

func TestDiscoverScanSkillRootNonNotExistError(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "regular"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o000); err != nil {
		t.Skipf("cannot remove perms (likely running as root): %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := skills.DiscoverSkills(t.TempDir(), home); err == nil {
		t.Fatal("expected error when skill root is unreadable")
	}
}

func TestDiscoverSkipsDirWhereSKILLPathIsDir(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	weird := filepath.Join(home, "skills", "weird")
	if err := os.MkdirAll(filepath.Join(weird, "SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d, want 0 (SKILL.md as dir should be skipped); paths=%v", len(got), pathsOf(got))
	}
}

func TestDiscoverSortedOutputWithinSameOrigin(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	for _, n := range []string{"zeta", "alpha", "mu"} {
		writeSkill(t, filepath.Join(home, "skills", n))
	}

	got, err := skills.DiscoverSkills(cwd, home)
	if err != nil {
		t.Fatalf("DiscoverSkills: %v", err)
	}

	var fromHome []string
	for _, s := range got {
		if s.Origin == skills.OriginGlobal {
			fromHome = append(fromHome, filepath.Base(filepath.Dir(s.Path)))
		}
	}
	want := []string{"alpha", "mu", "zeta"}
	if len(fromHome) != len(want) {
		t.Fatalf("got %v, want %v", fromHome, want)
	}
	for i, n := range fromHome {
		if n != want[i] {
			t.Errorf("fromHome[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestRegistryErrorFromDiscoverPropagates(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o000); err != nil {
		t.Skipf("cannot remove perms (likely running as root): %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if _, err := skills.LoadForCwd(t.TempDir(), home); err == nil {
		t.Fatal("expected LoadForCwd to fail when skill root is unreadable")
	}
}

func TestRegistryNilReceiverGet(t *testing.T) {
	var r *skills.Registry
	s, ok := r.Get("anything")
	if ok || s != nil {
		t.Errorf("nil registry Get should return (nil, false); got (%v, %v)", s, ok)
	}
}

func TestRegistryNilReceiverNames(t *testing.T) {
	var r *skills.Registry
	if names := r.Names(); names != nil {
		t.Errorf("nil registry Names should return nil; got %v", names)
	}
}

func TestRegistryReadFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate unreadable file when running as root")
	}

	cwd := t.TempDir()
	home := t.TempDir()

	dir := filepath.Join(home, "skills", "unreadable")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(target, []byte("---\nname: x\ndescription: y\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o000); err != nil {
		t.Skipf("cannot chmod 0: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(target, 0o644) })

	reg, err := skills.LoadForCwd(cwd, home)
	if err != nil {
		t.Fatalf("LoadForCwd: %v", err)
	}
	if len(reg.Errors) == 0 {
		t.Fatal("expected read error to be aggregated")
	}

	found := false
	for _, e := range reg.Errors {
		if strings.Contains(e.Error(), "read skill") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'read skill' error; got %v", reg.Errors)
	}
}

func TestParseFrontmatterMissingClosingDelimiter(t *testing.T) {
	input := "---\nname: x\ndescription: y\n"
	_, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err == nil {
		t.Fatal("expected error for missing closing delimiter")
	}
	if !errors.Is(err, skills.ErrFrontmatterInvalid) {
		t.Errorf("err = %v, want ErrFrontmatterInvalid", err)
	}
}

func TestParseFrontmatterBlankLineInHeader(t *testing.T) {
	input := "---\nname: x\n\ndescription: y\n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if s.Name != "x" {
		t.Errorf("Name = %q, want x", s.Name)
	}
	if s.Description != "y" {
		t.Errorf("Description = %q, want y", s.Description)
	}
}

func TestParseFrontmatterLineWithoutColon(t *testing.T) {
	input := "---\nname: x\ndescription: y\nno-colon-line\n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if s.Name != "x" {
		t.Errorf("Name = %q, want x", s.Name)
	}
}

func TestParseFrontmatterEmptyAllowedTools(t *testing.T) {
	input := "---\nname: x\ndescription: y\nallowed-tools: \n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if len(s.AllowedTools) != 0 {
		t.Errorf("AllowedTools = %v, want empty/nil", s.AllowedTools)
	}
}

func TestParseFrontmatterWhitespaceOnlyAllowedTools(t *testing.T) {
	input := "---\nname: x\ndescription: y\nallowed-tools:   ,  , \n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if len(s.AllowedTools) != 0 {
		t.Errorf("AllowedTools = %v, want empty/nil", s.AllowedTools)
	}
}

func TestParseFrontmatterTrimsWhitespaceInValues(t *testing.T) {
	input := "---\nname:    spaced   \ndescription:  multi  word  desc  \n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if s.Name != "spaced" {
		t.Errorf("Name = %q, want 'spaced' (trimmed)", s.Name)
	}
	if s.Description != "multi  word  desc" {
		t.Errorf("Description = %q, want 'multi  word  desc'", s.Description)
	}
}

func TestParseFrontmatterOriginUnknownInSyntheticPath(t *testing.T) {
	input := "---\nname: x\ndescription: y\n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/synthetic/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if s.Source.Origin != skills.OriginUnknown {
		t.Errorf("Source.Origin = %v, want OriginUnknown for synthetic path", s.Source.Origin)
	}
	if s.Source.Path != "/synthetic/SKILL.md" {
		t.Errorf("Source.Path = %q, want /synthetic/SKILL.md", s.Source.Path)
	}
}
