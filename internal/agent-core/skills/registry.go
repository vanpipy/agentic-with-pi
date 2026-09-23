package skills

import (
	"fmt"
	"os"
	"sort"
)

type Registry struct {
	Skills  map[string]*Skill
	Errors  []error
	Sources []SkillSource
}

func LoadForCwd(cwd, homeDir string) (*Registry, error) {
	sources, err := DiscoverSkills(cwd, homeDir)
	if err != nil {
		return nil, err
	}

	r := &Registry{
		Skills:  make(map[string]*Skill),
		Sources: sources,
	}

	for _, src := range sources {
		data, err := os.ReadFile(src.Path)
		if err != nil {
			r.Errors = append(r.Errors, fmt.Errorf("read skill %s: %w", src.Path, err))
			continue
		}
		s, err := ParseFrontmatter(data, src.Path)
		if err != nil {
			r.Errors = append(r.Errors, err)
			continue
		}
		s.Source = src

		if existing, dup := r.Skills[s.Name]; dup {
			if existing.Source.Origin == s.Source.Origin {
				r.Errors = append(r.Errors, fmt.Errorf(
					"duplicate skill %q within %s origin (kept %s, skipped %s)",
					s.Name, s.Source.Origin, existing.Source.Path, s.Source.Path,
				))
				continue
			}
		}

		r.Skills[s.Name] = s
	}

	return r, nil
}

func (r *Registry) Get(name string) (*Skill, bool) {
	if r == nil || r.Skills == nil {
		return nil, false
	}
	s, ok := r.Skills[name]
	return s, ok
}

func (r *Registry) Names() []string {
	if r == nil || r.Skills == nil {
		return nil
	}
	out := make([]string, 0, len(r.Skills))
	for n := range r.Skills {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
