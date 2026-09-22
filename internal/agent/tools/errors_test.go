package tools

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func TestConciseToolErrorSummaryMissingField(t *testing.T) {
	got, ok := ConciseToolErrorSummary("Error: missing field `command`")
	if !ok {
		t.Fatal("expected summary for missing field")
	}
	if got != "invalid input: missing command" {
		t.Errorf("got %q, want %q", got, "invalid input: missing command")
	}
}

func TestConciseToolErrorSummaryExitCode(t *testing.T) {
	got, ok := ConciseToolErrorSummary("Exit code: 1")
	if !ok {
		t.Fatal("expected summary for exit code")
	}
	if got != "exit 1" {
		t.Errorf("got %q, want exit 1", got)
	}
}

func TestConciseToolErrorSummaryFinishedExitCode(t *testing.T) {
	got, ok := ConciseToolErrorSummary("--- Command finished with exit code: 2 ---")
	if !ok {
		t.Fatal("expected summary")
	}
	if got != "exit 2" {
		t.Errorf("got %q, want exit 2", got)
	}
}

func TestConciseToolErrorSummaryNoMatch(t *testing.T) {
	got, ok := ConciseToolErrorSummary("all good\nno errors here\n")
	if ok {
		t.Errorf("expected no summary, got %q", got)
	}
}

func TestToolOutputLooksFailedDetectsStatus(t *testing.T) {
	if !ToolOutputLooksFailed("Status: failed") {
		t.Error("expected failed for Status: failed")
	}
}

func TestToolOutputLooksFailedDetectsExitCode(t *testing.T) {
	if !ToolOutputLooksFailed("Exit code: 1") {
		t.Error("expected failed for Exit code: 1")
	}
	if ToolOutputLooksFailed("Exit code: 0") {
		t.Error("expected success for Exit code: 0")
	}
}

func TestToolOutputLooksFailedDetectsCheckMark(t *testing.T) {
	if !ToolOutputLooksFailed("✗ demo.txt: failed to find expected lines") {
		t.Error("expected failed for ✗ prefix")
	}
}

func TestToolOutputLooksFailedStripsLabel(t *testing.T) {
	if !ToolOutputLooksFailed("[apply_patch] ✗ demo.txt: failed to find expected lines") {
		t.Error("expected failed for label-prefixed output")
	}
}

func TestToolOutputLooksFailedEmptyIsFalse(t *testing.T) {
	if ToolOutputLooksFailed("") {
		t.Error("empty should not be failed")
	}
	if ToolOutputLooksFailed("   \n\t  ") {
		t.Error("whitespace-only should not be failed")
	}
}

func TestToolOutputLooksFailedDetectsTerminated(t *testing.T) {
	if !ToolOutputLooksFailed("Compile terminated by signal SIGKILL") {
		t.Error("expected failed for terminated signal")
	}
}

func TestConciseToolErrorSummaryTruncatesLongDetail(t *testing.T) {
	long := strings.Repeat("x", 200)
	got, ok := ConciseToolErrorSummary("Error: " + long)
	if !ok {
		t.Fatal("expected summary")
	}
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("got %q, want error: prefix", got)
	}
	if uniseg.StringWidth(got) > ErrorSummaryMaxWidth+8 {
		t.Errorf("summary width %d > %d", uniseg.StringWidth(got), ErrorSummaryMaxWidth+8)
	}
}
