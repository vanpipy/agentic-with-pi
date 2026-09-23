package agentcore_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/skills"
)

func TestOriginString(t *testing.T) {
	cases := []struct {
		got  skills.Origin
		want string
	}{
		{skills.OriginGlobal, "global"},
		{skills.OriginProject, "project"},
		{skills.OriginAgents, "agents"},
	}
	for _, c := range cases {
		if got := c.got.String(); got != c.want {
			t.Errorf("Origin(%d).String() = %q, want %q", c.got, got, c.want)
		}
	}
}

func TestOriginUnknownString(t *testing.T) {
	if got := skills.Origin(99).String(); got != "unknown" {
		t.Errorf("Origin(99).String() = %q, want unknown", got)
	}
}

func TestSkillSourceOrigin(t *testing.T) {
	src := skills.SkillSource{Origin: skills.OriginProject, Path: "/tmp/x/SKILL.md"}
	if src.Origin != skills.OriginProject {
		t.Errorf("SkillSource.Origin = %d, want %d", src.Origin, skills.OriginProject)
	}
	if src.Path != "/tmp/x/SKILL.md" {
		t.Errorf("SkillSource.Path = %q, want /tmp/x/SKILL.md", src.Path)
	}
}
