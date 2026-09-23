package skills

import (
	"bytes"
	"fmt"
	"strings"
)

const frontmatterDelimiter = "---"

func ParseFrontmatter(content []byte, sourcePath string) (*Skill, error) {
	src := SkillSource{Origin: OriginUnknown, Path: sourcePath}

	if len(bytes.TrimSpace(content)) == 0 {
		return nil, fmt.Errorf("%w: empty file at %s", ErrFrontmatterInvalid, sourcePath)
	}

	lines := splitLines(content)

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterDelimiter {
		return nil, fmt.Errorf("%w: missing opening %q at %s", ErrFrontmatterInvalid, frontmatterDelimiter, sourcePath)
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelimiter {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("%w: missing closing %q at %s", ErrFrontmatterInvalid, frontmatterDelimiter, sourcePath)
	}

	headerLines := lines[1:end]
	bodyLines := lines[end+1:]

	s := &Skill{
		Source:  src,
		Content: strings.Join(bodyLines, "\n"),
	}

	for _, raw := range headerLines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := splitKV(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "name":
			s.Name = strings.TrimSpace(value)
		case "description":
			s.Description = strings.TrimSpace(value)
		case "allowed-tools":
			s.AllowedTools = parseAllowedTools(value)
		}
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("%w at %s: %v", ErrFrontmatterInvalid, sourcePath, err)
	}

	return s, nil
}

func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	return strings.Split(string(content), "\n")
}

func splitKV(line string) (key string, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

func parseAllowedTools(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
