package tools_test

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
	"github.com/vanpiyp/awp/internal/agent/tools"
)

func TestConciseToolErrorSummaryMissingField(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Error: missing field `command`")
	if !ok {
		t.Fatal("expected summary for missing field")
	}
	if got != "invalid input: missing command" {
		t.Errorf("got %q, want %q", got, "invalid input: missing command")
	}
}

func TestConciseToolErrorSummaryExitCode(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("Exit code: 1")
	if !ok {
		t.Fatal("expected summary for exit code")
	}
	if got != "exit 1" {
		t.Errorf("got %q, want exit 1", got)
	}
}

func TestConciseToolErrorSummaryFinishedExitCode(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("--- Command finished with exit code: 2 ---")
	if !ok {
		t.Fatal("expected summary")
	}
	if got != "exit 2" {
		t.Errorf("got %q, want exit 2", got)
	}
}

func TestConciseToolErrorSummaryNoMatch(t *testing.T) {
	got, ok := tools.ConciseToolErrorSummary("all good\nno errors here\n")
	if ok {
		t.Errorf("expected no summary, got %q", got)
	}
}

func TestToolOutputLooksFailedDetectsStatus(t *testing.T) {
	if !tools.ToolOutputLooksFailed("Status: failed") {
		t.Error("expected failed for Status: failed")
	}
}

func TestToolOutputLooksFailedDetectsExitCode(t *testing.T) {
	if !tools.ToolOutputLooksFailed("Exit code: 1") {
		t.Error("expected failed for Exit code: 1")
	}
	if tools.ToolOutputLooksFailed("Exit code: 0") {
		t.Error("expected success for Exit code: 0")
	}
}

func TestToolOutputLooksFailedDetectsCheckMark(t *testing.T) {
	if !tools.ToolOutputLooksFailed("✗ demo.txt: failed to find expected lines") {
		t.Error("expected failed for ✗ prefix")
	}
}

func TestToolOutputLooksFailedStripsLabel(t *testing.T) {
	if !tools.ToolOutputLooksFailed("[apply_patch] ✗ demo.txt: failed to find expected lines") {
		t.Error("expected failed for label-prefixed output")
	}
}

func TestToolOutputLooksFailedEmptyIsFalse(t *testing.T) {
	if tools.ToolOutputLooksFailed("") {
		t.Error("empty should not be failed")
	}
	if tools.ToolOutputLooksFailed("   \n\t  ") {
		t.Error("whitespace-only should not be failed")
	}
}

func TestToolOutputLooksFailedDetectsTerminated(t *testing.T) {
	if !tools.ToolOutputLooksFailed("Compile terminated by signal SIGKILL") {
		t.Error("expected failed for terminated signal")
	}
}

func TestConciseToolErrorSummaryTruncatesLongDetail(t *testing.T) {
	long := strings.Repeat("x", 200)
	got, ok := tools.ConciseToolErrorSummary("Error: " + long)
	if !ok {
		t.Fatal("expected summary")
	}
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("got %q, want error: prefix", got)
	}
	if uniseg.StringWidth(got) > tools.ErrorSummaryMaxWidth+8 {
		t.Errorf("summary width %d > %d", uniseg.StringWidth(got), tools.ErrorSummaryMaxWidth+8)
	}
}
