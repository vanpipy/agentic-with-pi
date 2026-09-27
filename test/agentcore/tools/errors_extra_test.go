package tools_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestParseNonzeroExitCodeLineReachedViaToolOutputLooksFailed(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"exit code 1", "Exit code: 1", true},
		{"exit code 0", "Exit code: 0", false},
		{"exit code -1", "Exit code: -1", true},
		{"exit code unparseable", "Exit code:abc", false},
		{"finished exit code 1", "--- Command finished with exit code: 1 ---", true},
		{"finished exit code 0", "--- Command finished with exit code: 0 ---", false},
		{"finished exit code unparseable", "--- Command finished with exit code:abc", true},
		{"random text", "random text", false},
		{"empty line", "", false},
	}
	for _, c := range cases {
		got := tools.ToolOutputLooksFailed(c.input)
		if got != c.want {
			t.Errorf("ToolOutputLooksFailed(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestConciseToolErrorSummaryErrorPrefix(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Error: something broke")
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("got %q, want error: prefix", got)
	}
}

func TestConciseToolErrorSummaryLowercaseError(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("error: lowercase error")
	if !ok {
		t.Fatal("expected ok")
	}
	if got != "error: lowercase error" {
		t.Errorf("got %q", got)
	}
}

func TestConciseToolErrorSummaryFailedPrefix(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Failed: did not compile")
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("got %q, want error: prefix", got)
	}
}

func TestConciseToolErrorSummaryInvalidType(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Error: invalid type: expected u32 got string")
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.HasPrefix(got, "invalid input:") {
		t.Errorf("got %q, want invalid input: prefix", got)
	}
}

func TestConciseToolErrorSummaryUnknownVariant(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Error: unknown variant `frobnicate`")
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.HasPrefix(got, "invalid input:") {
		t.Errorf("got %q, want invalid input: prefix", got)
	}
}

func TestConciseToolErrorSummaryMissingFieldPlain(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Error: missing field command")
	if !ok {
		t.Fatal("expected ok")
	}
	if !strings.HasPrefix(got, "invalid input: missing") {
		t.Errorf("got %q, want invalid input: missing prefix", got)
	}
}

func TestConciseToolErrorSummaryCompileTerminated(t *testing.T) {
	in := "warning: blah\nCompile terminated by signal SIGTERM"
	got, ok := tools.ConciseToolErrorSummary(in)
	if !ok {
		t.Fatal("expected ok")
	}
	if got != "Compile terminated by signal SIGTERM" {
		t.Errorf("got %q", got)
	}
}

func TestConciseToolErrorSummaryExitCodeZeroIgnored(t *testing.T) {
	if _, ok := tools.ConciseToolErrorSummary("Exit code: 0"); ok {
		t.Error("Exit code 0 should not produce a summary")
	}
}

func TestToolOutputLooksFailedLowercaseErrorPrefix(t *testing.T) {
	if !tools.ToolOutputLooksFailed("error: nope") {
		t.Error("lowercase 'error:' should be detected")
	}
}

func TestToolOutputLooksFailedLowercaseFailedPrefix(t *testing.T) {
	if !tools.ToolOutputLooksFailed("failed: nope") {
		t.Error("lowercase 'failed:' should be detected")
	}
}

func TestToolOutputLooksFailedFailedToStart(t *testing.T) {
	if !tools.ToolOutputLooksFailed("failed to start") {
		t.Error("'failed to start' should be detected")
	}
}

func TestToolOutputLooksFailedLabelOnly(t *testing.T) {
	if !tools.ToolOutputLooksFailed("[run_tests] Exit code: 1") {
		t.Error("labeled Exit code should be detected")
	}
}

func TestToolOutputLooksFailedNoFalsePositive(t *testing.T) {
	if tools.ToolOutputLooksFailed("all good\nstatus: ok\nresult: passed\n") {
		t.Error("successful output should not be marked failed")
	}
}

func TestToolOutputLooksFailedIgnoresEmptyLabel(t *testing.T) {
	if tools.ToolOutputLooksFailed("[] all good") {
		t.Error("empty label should not produce a false positive")
	}
}
