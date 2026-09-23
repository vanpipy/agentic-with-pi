package skills

import (
	"strings"
)

func (r *Registry) ResolveInvocation(input string) (name string, prompt string, ok bool) {
	if r == nil || r.Skills == nil {
		return "", "", false
	}

	body, ok := stripInvocationPrefix(input)
	if !ok {
		return "", "", false
	}

	matchName, matchedLen := longestMatch(body, r.Skills)
	if matchName == "" {
		return "", "", false
	}

	rest := strings.TrimSpace(body[matchedLen:])
	if len(rest) > 0 {
		first := rest[0]
		last := rest[len(rest)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			if len(rest) >= 2 {
				rest = rest[1 : len(rest)-1]
			}
		}
	}

	return matchName, rest, true
}

func stripInvocationPrefix(input string) (string, bool) {
	if !strings.HasPrefix(input, "/") {
		return "", false
	}
	body := strings.TrimPrefix(input, "/")
	if strings.TrimSpace(body) == "" {
		return "", false
	}
	return body, true
}

func longestMatch(body string, skillsMap map[string]*Skill) (string, int) {
	bestName := ""
	bestLen := 0
	for name := range skillsMap {
		if !strings.HasPrefix(body, name) {
			continue
		}
		if len(body) == len(name) {
			return name, len(name)
		}
		next := body[len(name)]
		if next != ' ' && next != '\t' {
			continue
		}
		if len(name) > bestLen {
			bestName = name
			bestLen = len(name)
		}
	}
	return bestName, bestLen
}
