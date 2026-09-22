package tools

import "github.com/rivo/uniseg"

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
