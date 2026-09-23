package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vanpiyp/awp/internal/paths"
)

const skillFileName = "SKILL.md"

func DiscoverSkills(cwd, homeDir string) ([]SkillSource, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("discover: getwd: %w", err)
		}
	}
	if homeDir == "" {
		homeDir = paths.Home()
	}

	roots := []struct {
		origin Origin
		path   string
	}{
		{OriginGlobal, filepath.Join(homeDir, "skills")},
		{OriginProject, filepath.Join(cwd, ".awp", "skills")},
		{OriginAgents, filepath.Join(cwd, "agents", "skills")},
	}

	var out []SkillSource
	for _, r := range roots {
		files, err := scanSkillRoot(r.path)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			out = append(out, SkillSource{Origin: r.origin, Path: f})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Origin != out[j].Origin {
			return out[i].Origin < out[j].Origin
		}
		return out[i].Path < out[j].Path
	})

	return out, nil
}

func scanSkillRoot(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}

	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(root, e.Name(), skillFileName)
		info, err := os.Stat(skillPath)
		if err != nil {
			if isNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", skillPath, err)
		}
		if info.IsDir() {
			continue
		}
		out = append(out, skillPath)
	}

	sort.Strings(out)
	return out, nil
}

func isNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
