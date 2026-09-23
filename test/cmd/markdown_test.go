package cmd_test

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestMarkdownRendersHeadings(t *testing.T) {
	out := tui.RenderMarkdownForTest("# Title\n\nSome text", 80)
	if out == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(out, "Title") {
		t.Errorf("expected 'Title' in output, got %q", out)
	}
}

func TestMarkdownRendersBold(t *testing.T) {
	out := tui.RenderMarkdownForTest("**bold text**", 80)
	if !strings.Contains(out, "bold text") {
		t.Errorf("expected 'bold text' in output, got %q", out)
	}
}

func TestMarkdownRendersCodeFence(t *testing.T) {
	out := tui.RenderMarkdownForTest("```\nfoo\nbar\n```", 80)
	if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
		t.Errorf("expected foo and bar in code fence output, got %q", out)
	}
}

func TestMarkdownEmpty(t *testing.T) {
	if out := tui.RenderMarkdownForTest("", 80); out != "" {
		t.Errorf("empty input should produce empty output, got %q", out)
	}
	if out := tui.RenderMarkdownForTest("   \n  ", 80); out != "" {
		t.Errorf("whitespace-only input should produce empty output, got %q", out)
	}
}

var _ = viewport.New
