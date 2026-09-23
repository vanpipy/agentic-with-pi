package agentcore_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func TestParseFrontmatterValidAllFields(t *testing.T) {
	input := "---\nname: optimize\ndescription: Optimize Go code\nallowed-tools: read, edit, shell\n---\n# body here\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter valid: %v", err)
	}
	if s.Name != "optimize" {
		t.Errorf("Name = %q, want optimize", s.Name)
	}
	if s.Description != "Optimize Go code" {
		t.Errorf("Description = %q, want %q", s.Description, "Optimize Go code")
	}
	if !reflect.DeepEqual(s.AllowedTools, []string{"read", "edit", "shell"}) {
		t.Errorf("AllowedTools = %v, want [read edit shell]", s.AllowedTools)
	}
	if s.Content != "# body here\n" {
		t.Errorf("Content = %q, want %q", s.Content, "# body here\n")
	}
	if s.Source.Path != "/x/SKILL.md" {
		t.Errorf("Source.Path = %q, want /x/SKILL.md", s.Source.Path)
	}
}

func TestParseFrontmatterMissingName(t *testing.T) {
	input := "---\ndescription: only desc\n---\nbody\n"
	_, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err == nil {
		t.Fatalf("expected error for missing name")
	}
	if !errors.Is(err, skills.ErrFrontmatterInvalid) {
		t.Errorf("err = %v, want ErrFrontmatterInvalid", err)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("err msg should mention name, got %q", err.Error())
	}
}

func TestParseFrontmatterMissingDescription(t *testing.T) {
	input := "---\nname: x\n---\nbody\n"
	_, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err == nil {
		t.Fatalf("expected error for missing description")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("err msg should mention description, got %q", err.Error())
	}
}

func TestParseFrontmatterEmptyFile(t *testing.T) {
	_, err := skills.ParseFrontmatter([]byte(""), "/x/SKILL.md")
	if err == nil {
		t.Fatalf("expected error for empty file")
	}
}

func TestParseFrontmatterNoDelimiters(t *testing.T) {
	_, err := skills.ParseFrontmatter([]byte("just a body with no frontmatter\n"), "/x/SKILL.md")
	if err == nil {
		t.Fatalf("expected error for no opening delimiter")
	}
}

func TestParseFrontmatterNoBodyAfterFrontmatter(t *testing.T) {
	input := "---\nname: x\ndescription: y\n---\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if s.Content != "" {
		t.Errorf("Content = %q, want empty", s.Content)
	}
	if s.Name != "x" {
		t.Errorf("Name = %q, want x", s.Name)
	}
}

func TestParseFrontmatterAllowedToolsTrimsSpacesAndDropsEmpty(t *testing.T) {
	input := "---\nname: x\ndescription: y\nallowed-tools: read , , edit,  shell \n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	want := []string{"read", "edit", "shell"}
	if !reflect.DeepEqual(s.AllowedTools, want) {
		t.Errorf("AllowedTools = %v, want %v", s.AllowedTools, want)
	}
}

func TestParseFrontmatterNoAllowedToolsYieldsNilSlice(t *testing.T) {
	input := "---\nname: x\ndescription: y\n---\nbody\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	if len(s.AllowedTools) != 0 {
		t.Errorf("AllowedTools = %v, want nil/empty", s.AllowedTools)
	}
}

func TestParseFrontmatterBlankLineAfterBody(t *testing.T) {
	input := "---\nname: x\ndescription: y\n---\nfirst line\nsecond line\n"
	s, err := skills.ParseFrontmatter([]byte(input), "/x/SKILL.md")
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	want := "first line\nsecond line\n"
	if s.Content != want {
		t.Errorf("Content = %q, want %q", s.Content, want)
	}
}
