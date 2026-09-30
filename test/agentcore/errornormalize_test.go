package agentcore_test

import (
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestNormalizeToolError_StripsExampleSegment(t *testing.T) {
	a := "Tool bash: missing required field(s) [file]. Pass them as a JSON object argument. Example: {\"file\": \"<value>\"}. Field descriptions: file: The path"
	b := "Tool bash: missing required field(s) [file]. Pass them as a JSON object argument. Example: {\"file\": \"\"}. Field descriptions: file: The path"
	if gotA, gotB := stream.NormalizeToolError(a), stream.NormalizeToolError(b); gotA != gotB {
		t.Errorf("normalize should strip dynamic Example placeholder, got %q vs %q", gotA, gotB)
	}
}

func TestNormalizeToolError_StripsFieldDescriptionsTrailer(t *testing.T) {
	a := "Tool bash: missing required field(s) [command]. Field descriptions: command: shell command"
	b := "Tool bash: missing required field(s) [command]. Field descriptions: command: shell; never embedded secrets"
	if gotA, gotB := stream.NormalizeToolError(a), stream.NormalizeToolError(b); gotA != gotB {
		t.Errorf("normalize should strip dynamic Field descriptions trailer, got %q vs %q", gotA, gotB)
	}
}

func TestNormalizeToolError_PreservesToolNameAndPrefix(t *testing.T) {
	in := "Tool ls failed: kaboom"
	got := stream.NormalizeToolError(in)
	if !strings.HasPrefix(got, "Tool ls failed:") {
		t.Errorf("normalize should preserve 'Tool <name> failed:' prefix, got %q", got)
	}
}

func TestNormalizeToolError_CollapsesWhitespace(t *testing.T) {
	in := "Tool  bash   failed:   kaboom\n\twith newline"
	got := stream.NormalizeToolError(in)
	if strings.Contains(got, "  ") {
		t.Errorf("normalize should collapse multiple spaces, got %q", got)
	}
	if strings.Contains(got, "\n") || strings.Contains(got, "\t") {
		t.Errorf("normalize should strip newlines and tabs, got %q", got)
	}
	if strings.TrimSpace(got) != got {
		t.Errorf("normalize should trim, got %q", got)
	}
}

func TestNormalizeToolError_NoChangeForStableText(t *testing.T) {
	in := "Tool ls failed: kaboom"
	got := stream.NormalizeToolError(in)
	if got != in {
		t.Errorf("normalize should be idempotent on already-stable text, got %q", got)
	}
}

func TestNormalizeToolError_Empty(t *testing.T) {
	if got := stream.NormalizeToolError(""); got != "" {
		t.Errorf("normalize(\"\") = %q, want \"\"", got)
	}
}

func TestReActStrategyShouldAbort_NormalizesPreflightErrors(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	makeErr := func(placeholder string) string {
		return "Tool bash: missing required field(s) [file]. Pass them as a JSON object argument. Example: {\"file\": \"" + placeholder + "\"}. Field descriptions: file: The path"
	}
	errA := makeErr("<value>")
	errB := makeErr("")
	errC := makeErr("different")
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "tool", Content: errA},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"x"}`}}}},
		{Role: "tool", Content: errB},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"y"}`}}}},
		{Role: "tool", Content: errC},
	}
	if err := strat.ShouldAbort(msgs, errC); err == nil {
		t.Error("ShouldAbort returned nil, want non-nil after normalize converges preflight errors with dynamic example placeholders")
	}
}

func TestReActStrategyShouldAbort_NormalizesDifferentMissingFields(t *testing.T) {
	strat := agentcore.NewReActStrategy(&fakeCore{}, llm.Model{ID: "m", SupportsTool: true}, func() []llm.ToolDef { return nil }, nil)
	makeErr := func(fields string, placeholder string) string {
		return "Tool bash: missing required field(s) [" + fields + "]. Pass them as a JSON object argument. Example: {\"" + strings.Split(fields, ", ")[0] + "\": \"" + placeholder + "\"}. Field descriptions: file: The path"
	}
	errA := makeErr("file", "<value>")
	errB := makeErr("file", "")
	msgs := []llm.Message{
		{Role: "user", Content: "ls"},
		{Role: "tool", Content: errA},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"x"}`}}}},
		{Role: "tool", Content: errB},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Function: llm.FunctionCall{Name: "bash", Arguments: `{"intent":"y"}`}}}},
		{Role: "tool", Content: errB},
	}
	if err := strat.ShouldAbort(msgs, errB); err == nil {
		t.Error("ShouldAbort returned nil, want non-nil after normalize converges same-missing-fields errors")
	}
}
