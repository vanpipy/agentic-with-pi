package tools

import (
	"strings"

	"github.com/rivo/uniseg"
)

func graphemes(s string) []string {
	g := uniseg.NewGraphemes(s)
	out := make([]string, 0, len(s)/2)
	for g.Next() {
		out = append(out, g.Str())
	}
	return out
}

func displayPrefixByWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	used := 0
	result := ""
	for _, c := range graphemes(s) {
		cw := uniseg.StringWidth(c)
		if used+cw > maxWidth {
			break
		}
		used += cw
		result += c
	}
	return result
}

func displaySuffixByWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	used := 0
	clusters := graphemes(s)
	result := ""
	for i := len(clusters) - 1; i >= 0; i-- {
		cw := uniseg.StringWidth(clusters[i])
		if used+cw > maxWidth {
			break
		}
		used += cw
		result = clusters[i] + result
	}
	return result
}

func TruncateMiddle(s string, maxWidth int) string {
	if uniseg.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 0 {
		return ""
	}
	if maxWidth == 1 {
		return "…"
	}
	remaining := maxWidth - 1
	head := remaining/2 + remaining%2
	tail := remaining / 2
	return displayPrefixByWidth(s, head) + "…" + displaySuffixByWidth(s, tail)
}

func TruncateEnd(s string, maxWidth int) string {
	if uniseg.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 0 {
		return ""
	}
	if maxWidth == 1 {
		return "…"
	}
	return displayPrefixByWidth(s, maxWidth-1) + "…"
}

func pathMarker(path string) string {
	switch {
	case len(path) >= 2 && path[:2] == "~/":
		return "~/…/"
	case len(path) >= 2 && path[:2] == "./":
		return "./…/"
	case len(path) >= 1 && path[0] == '/':
		return "/…/"
	default:
		return "…/"
	}
}

func TruncatePath(path string, maxWidth int) string {
	if uniseg.StringWidth(path) <= maxWidth {
		return path
	}
	if maxWidth <= 0 {
		return ""
	}

	normalized := strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(normalized, "/")
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return TruncateMiddle(path, maxWidth)
	}

	marker := pathMarker(normalized)
	markerWidth := uniseg.StringWidth(marker)

	var joined string
	kept := make([]string, 0, len(filtered))
	for i := len(filtered) - 1; i >= 0; i-- {
		var candidate string
		if joined == "" {
			candidate = filtered[i]
		} else {
			candidate = filtered[i] + "/" + joined
		}
		if markerWidth+uniseg.StringWidth(candidate) > maxWidth {
			break
		}
		joined = candidate
		kept = append(kept, filtered[i])
	}

	if joined != "" {
		return marker + joined
	}

	last := filtered[len(filtered)-1]
	suffixBudget := maxWidth - uniseg.StringWidth("…/")
	if suffixBudget > 0 {
		return "…/" + TruncateMiddle(last, suffixBudget)
	}
	return TruncateMiddle(path, maxWidth)
}

func TruncateCommand(command string, maxWidth int) string {
	if uniseg.StringWidth(command) <= maxWidth {
		return command
	}
	if maxWidth <= 1 {
		return "…"
	}

	tokens := strings.Fields(command)
	if len(tokens) >= 3 {
		candidates := []string{
			tokens[0] + " " + tokens[1] + " … " + tokens[len(tokens)-2] + " " + tokens[len(tokens)-1],
			tokens[0] + " " + tokens[1] + " … " + tokens[len(tokens)-1],
			tokens[0] + " … " + tokens[len(tokens)-1],
		}
		for _, c := range candidates {
			if uniseg.StringWidth(c) <= maxWidth {
				return c
			}
		}
	}
	return TruncateMiddle(command, maxWidth)
}
