package tools

import (
	"strconv"
	"strings"
)

const ErrorSummaryMaxWidth = 80

func parseNonzeroExitCodeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if rest, ok := strings.CutPrefix(trimmed, "Exit code:"); ok {
		code, err := strconv.Atoi(strings.TrimSpace(rest))
		return err == nil && code != 0
	}
	if rest, ok := strings.CutPrefix(trimmed, "--- Command finished with exit code:"); ok {
		cleaned := strings.TrimRight(strings.TrimSpace(rest), "-")
		code, err := strconv.Atoi(strings.TrimSpace(cleaned))
		return err == nil && code != 0
	}
	return false
}

func normalizeBacktickedIdentifier(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "`", ""))
}

func ConciseToolErrorSummary(content string) (string, bool) {
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		var detail string
		if rest, ok := strings.CutPrefix(line, "Error:"); ok {
			detail = strings.TrimSpace(rest)
		} else if rest, ok := strings.CutPrefix(line, "error:"); ok {
			detail = strings.TrimSpace(rest)
		} else if rest, ok := strings.CutPrefix(line, "Failed:"); ok {
			detail = strings.TrimSpace(rest)
		}
		if detail != "" {
			if rest, ok := strings.CutPrefix(detail, "missing field "); ok {
				return "invalid input: missing " + normalizeBacktickedIdentifier(rest), true
			}
			if strings.HasPrefix(detail, "invalid type") || strings.HasPrefix(detail, "unknown variant") {
				return "invalid input: " + TruncateMiddle(detail, ErrorSummaryMaxWidth), true
			}
			return "error: " + TruncateMiddle(detail, ErrorSummaryMaxWidth), true
		}

		if strings.Contains(line, "Compile terminated by signal") {
			return line, true
		}
		if rest, ok := strings.CutPrefix(line, "Exit code:"); ok {
			if code, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && code != 0 {
				return "exit " + strconv.Itoa(code), true
			}
		}
		if rest, ok := strings.CutPrefix(line, "--- Command finished with exit code:"); ok {
			cleaned := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(rest), "-"))
			if cleaned != "" && cleaned != "0" {
				return "exit " + cleaned, true
			}
		}
	}
	return "", false
}

func ToolOutputLooksFailed(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	normalized := trimmed
	if rest, ok := strings.CutPrefix(trimmed, "["); ok {
		if split := strings.SplitN(rest, "] ", 2); len(split) == 2 && split[0] != "" && !strings.ContainsAny(split[0], "\n\r") {
			normalized = split[1]
		}
	}
	if _, ok := ConciseToolErrorSummary(normalized); ok {
		return true
	}
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(lower, "error:") || strings.HasPrefix(lower, "failed:") {
		return true
	}
	if strings.HasPrefix(normalized, "✗") {
		return true
	}
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if parseNonzeroExitCodeLine(line) {
			return true
		}
		if strings.EqualFold(line, "Status: failed") || strings.EqualFold(line, "failed to start") || strings.EqualFold(line, "terminated") {
			return true
		}
	}
	return false
}
