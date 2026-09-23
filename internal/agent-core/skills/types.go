package skills

import (
	"errors"
	"fmt"
)

var ErrFrontmatterInvalid = errors.New("skill frontmatter invalid")

type Origin int

const (
	OriginUnknown Origin = iota
	OriginGlobal
	OriginProject
	OriginAgents
)

func (o Origin) String() string {
	switch o {
	case OriginGlobal:
		return "global"
	case OriginProject:
		return "project"
	case OriginAgents:
		return "agents"
	default:
		return "unknown"
	}
}

type SkillSource struct {
	Origin Origin
	Path   string
}

type Skill struct {
	Name         string
	Description  string
	AllowedTools []string
	Content      string
	Source       SkillSource
}

func (s *Skill) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("%w: missing name", ErrFrontmatterInvalid)
	}
	if s.Description == "" {
		return fmt.Errorf("%w: missing description", ErrFrontmatterInvalid)
	}
	return nil
}
