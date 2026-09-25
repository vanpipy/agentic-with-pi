package agentcore

import (
	"regexp"
	"strings"
)

var (
	exampleSegmentRE = regexp.MustCompile(`Example:\s*\{[^}]*\}\.`)
	fieldDescTailRE  = regexp.MustCompile(`Field descriptions:\s*.*$`)
	whitespaceRE     = regexp.MustCompile(`\s+`)
)

func NormalizeToolError(s string) string {
	if s == "" {
		return ""
	}
	out := exampleSegmentRE.ReplaceAllString(s, "")
	out = fieldDescTailRE.ReplaceAllString(out, "")
	out = whitespaceRE.ReplaceAllString(out, " ")
	return strings.TrimSpace(out)
}
