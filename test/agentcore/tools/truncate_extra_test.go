package tools_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestTruncateEndZeroWidth(t *testing.T) {
	if got := tools.TruncateEnd("hello", 0); got != "" {
		t.Errorf("TruncateEnd(_, 0) = %q, want empty", got)
	}
}

func TestTruncateEndOneWidth(t *testing.T) {
	if got := tools.TruncateEnd("hello", 1); got != "…" {
		t.Errorf("TruncateEnd(_, 1) = %q, want …", got)
	}
}

func TestTruncateEndCJKOneWidth(t *testing.T) {
	got := tools.TruncateEnd("中文字符", 1)
	if got != "…" {
		t.Errorf("TruncateEnd(CJK, 1) = %q, want …", got)
	}
}

func TestTruncateCommandWidthOne(t *testing.T) {
	if got := tools.TruncateCommand("hello world", 1); got != "…" {
		t.Errorf("TruncateCommand(_, 1) = %q, want …", got)
	}
}

func TestTruncateCommandWidthZero(t *testing.T) {
	if got := tools.TruncateCommand("hello", 0); got != "…" {
		t.Errorf("TruncateCommand(_, 0) = %q, want …", got)
	}
}

func TestDisplaySuffixByWidthZero(t *testing.T) {
	if tools.TruncateCommand("hello world", 0) != "…" {
		t.Errorf("TruncateCommand width 0 should be ellipsis")
	}
}
