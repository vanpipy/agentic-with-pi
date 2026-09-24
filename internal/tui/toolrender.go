package tui

import (
	"encoding/json"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

const (
	approxCharsPerToken = 4
	tokenWarnThreshold  = 4000
	tokenDangerThresh   = 12000
)

type tokenSeverity int

const (
	tokenNormal tokenSeverity = iota
	tokenWarning
	tokenDanger
)

func tokenSeverityFor(n int) tokenSeverity {
	switch {
	case n >= tokenDangerThresh:
		return tokenDanger
	case n >= tokenWarnThreshold:
		return tokenWarning
	default:
		return tokenNormal
	}
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	return len(s) / approxCharsPerToken
}

func formatTokenBadge(n int) string {
	switch {
	case n < 1000:
		return formatInt(n) + " tok"
	case n < 10000:
		whole := n / 1000
		tenth := (n % 1000) / 100
		if tenth == 0 {
			return formatInt(whole) + "k tok"
		}
		return formatInt(whole) + "." + formatInt(tenth) + "k tok"
	default:
		return formatInt(n/1000) + "k tok"
	}
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	out := string(digits)
	if negative {
		out = "-" + out
	}
	if len(out) <= 3 {
		return out
	}
	var b strings.Builder
	first := len(out) % 3
	if first > 0 {
		b.WriteString(out[:first])
		if first < len(out) {
			b.WriteByte(',')
		}
	}
	for i := first; i < len(out); i++ {
		if i > first && (len(out)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(out[i])
	}
	return b.String()
}

func shortenHome(path string) string {
	if path == "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+"/"); ok {
		return "~/" + rest
	}
	return path
}

type toolArgSummary struct {
	display string
	compact bool
}

func summarizeToolArg(name string, args json.RawMessage) toolArgSummary {
	if len(args) == 0 || string(args) == "null" {
		return toolArgSummary{display: ""}
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil {
		return toolArgSummary{display: tools.TruncateMiddle(string(args), 60), compact: true}
	}
	get := func(key string) string {
		if v, ok := m[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}
	getInt := func(key string) (int, bool) {
		v, ok := m[key]
		if !ok {
			return 0, false
		}
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			return 0, false
		}
		return n, true
	}

	switch name {
	case "read":
		file := shortenHome(get("file"))
		start, sOK := getInt("start_line")
		end, eOK := getInt("end_line")
		offset, oOK := getInt("offset")
		limit, lOK := getInt("limit")
		var suffix string
		switch {
		case sOK && eOK:
			suffix = ":" + formatInt(start) + "-" + formatInt(end)
		case sOK:
			suffix = ":" + formatInt(start) + "-"
		case eOK:
			suffix = ":1-" + formatInt(end)
		case oOK && lOK:
			suffix = ":" + formatInt(offset) + "-" + formatInt(offset+limit)
		case oOK:
			suffix = ":" + formatInt(offset)
		}
		if suffix != "" {
			return toolArgSummary{display: file + suffix}
		}
		return toolArgSummary{display: file}
	case "write", "edit", "edit_match":
		file := shortenHome(get("file"))
		return toolArgSummary{display: file}
	case "bash":
		cmd := get("command")
		return toolArgSummary{display: "$ " + tools.TruncateMiddle(cmd, 60), compact: true}
	case "grep", "agentgrep":
		q := get("query")
		if p := shortenHome(get("path")); p != "" {
			return toolArgSummary{display: "/" + tools.TruncateMiddle(q, 30) + "/  in  " + p}
		}
		return toolArgSummary{display: "/" + tools.TruncateMiddle(q, 40) + "/"}
	case "find":
		q := get("query")
		if p := shortenHome(get("path")); p != "" {
			return toolArgSummary{display: tools.TruncateMiddle(q, 30) + "  in  " + p}
		}
		return toolArgSummary{display: tools.TruncateMiddle(q, 40)}
	case "glob":
		pattern := get("pattern")
		return toolArgSummary{display: tools.TruncateMiddle(pattern, 50)}
	case "ls":
		if p := shortenHome(get("path")); p != "" {
			return toolArgSummary{display: p}
		}
		return toolArgSummary{display: "."}
	}
	return toolArgSummary{display: tools.TruncateMiddle(string(args), 60), compact: true}
}

type toolRowStatus int

const (
	toolRunning toolRowStatus = iota
	toolSuccess
	toolFailed
)

func renderToolRow(td *json_rpc.MessageContentPart, status toolRowStatus, resultPreview string, width int) []string {
	if td == nil {
		return nil
	}
	if width < 8 {
		width = 8
	}

	var icon string
	var iconStyle lipgloss.Style
	switch status {
	case toolSuccess:
		icon = "✓"
		iconStyle = toolSuccessIconStyle
	case toolFailed:
		icon = "✗"
		iconStyle = toolErrorIconStyle
	default:
		icon = "▸"
		iconStyle = toolRunningIconStyle
	}

	nameStyle := toolNameStyle
	if status == toolFailed {
		nameStyle = toolErrorNameStyle
	}

	summary := summarizeToolArg(td.Name, td.Arguments)
	tokens := estimateTokens(resultPreview)
	tokenBadge := formatTokenBadge(tokens)
	tokenStyle := toolTokenNormalStyle
	switch tokenSeverityFor(tokens) {
	case tokenWarning:
		tokenStyle = toolTokenWarningStyle
	case tokenDanger:
		tokenStyle = toolTokenDangerStyle
	}

	bodyWidth := width - lipgloss.Width(" ⚙ ")
	if bodyWidth < 8 {
		bodyWidth = 8
	}

	var prefix string
	if td.Intent != "" {
		prefix = td.Intent + "  "
	}

	sep := "  "
	prefixWidth := uniseg.StringWidth(prefix)
	iconWidth := 2
	nameText := td.Name
	nameWidth := uniseg.StringWidth(nameText)
	argWidth := bodyWidth - prefixWidth - iconWidth - nameWidth - 2 - 2 - uniseg.StringWidth(sep) - uniseg.StringWidth(tokenBadge)
	if argWidth < 4 {
		argWidth = 4
	}
	argText := tools.TruncateMiddle(summary.display, argWidth)

	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(icon)
	sb.WriteByte(' ')
	sb.WriteString(nameText)
	if argText != "" {
		sb.WriteString(sep)
		sb.WriteString(argText)
	}
	if status != toolRunning {
		sb.WriteString(" · ")
		sb.WriteString(tokenBadge)
	}

	styled := iconStyle.Render(icon) + " " + nameStyle.Render(nameText)
	if prefix != "" {
		styled = toolIntentStyle.Render(prefix) + styled
	}
	if argText != "" {
		styled += toolDimStyle.Render(sep) + toolDimStyle.Render(argText)
	}
	if status != toolRunning {
		styled += toolDimStyle.Render(" · ") + tokenStyle.Render(tokenBadge)
	}

	rendered := lipgloss.NewStyle().Width(width).Render(styled)
	if rendered == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(rendered, "\n"), "\n")
}

func renderToolResultPreview(text string, collapsed bool, failed bool, width int) []string {
	if text == "" {
		return nil
	}
	if width < 8 {
		width = 8
	}
	if collapsed {
		label := "result"
		if failed {
			if summary, ok := tools.ConciseToolErrorSummary(text); ok {
				label = summary
			} else {
				label = "error"
			}
		} else if tools.ToolOutputLooksFailed(text) {
			if summary, ok := tools.ConciseToolErrorSummary(text); ok {
				label = summary
			}
		}
		preview := tools.TruncateMiddle(label, width-4)
		style := toolDimStyle
		if failed {
			style = toolErrorPreviewStyle
		}
		return strings.Split(style.Render("  "+preview), "\n")
	}
	preview := tools.TruncateMiddle(text, width*4)
	if failed {
		preview = tools.TruncateMiddle(text, 400)
	}
	body := strings.Split(strings.TrimRight(preview, "\n"), "\n")
	for i, line := range body {
		body[i] = "  " + line
	}
	return body
}
