package agentcore

import (
	"strconv"
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

const emergencyMarkerPrefix = "emergency truncated"

type emergencyTruncatedToolResult struct {
	OriginalLen int
	KeptLen     int
	Marker      string
}

func emergencyTruncateToolResults(msgs []llm.Message, maxChars int) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = m
		if m.Role != "tool" || maxChars <= 0 {
			continue
		}
		if len(m.Content) <= maxChars {
			continue
		}
		dropped := len(m.Content) - maxChars
		out[i].Content = m.Content[:maxChars] + "\n[... " + emergencyMarkerPrefix + " " + strconv.Itoa(dropped) + " chars]"
	}
	return out
}

func isRequestPayloadTooLargeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "request payload too large") {
		return true
	}
	if strings.Contains(msg, "request entity too large") {
		return true
	}
	if strings.Contains(msg, "context_length_exceeded") {
		return true
	}
	if strings.Contains(msg, " 413 ") || strings.HasPrefix(msg, "413 ") || strings.HasSuffix(msg, " 413") || msg == "413" {
		return true
	}
	return false
}

func stripOversizedToolResultsInMessages(msgs []llm.Message, threshold int) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "tool" && threshold > 0 && len(m.Content) > threshold {
			continue
		}
		out = append(out, m)
	}
	return out
}
