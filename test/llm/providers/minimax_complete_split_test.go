package providers_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

func TestCompleteSplit_EmptyPromptReturnsNil(t *testing.T) {
	p := newProvider()
	blocks, err := p.CompleteSplit("")
	if err != nil {
		t.Fatalf("CompleteSplit(\"\") err = %v, want nil", err)
	}
	if len(blocks) != 0 {
		t.Errorf("CompleteSplit(\"\") len(blocks) = %d, want 0", len(blocks))
	}
}

func TestCompleteSplit_SingleLineReturnsWholeAsCached(t *testing.T) {
	p := newProvider()
	prompt := "no newlines here at all just one long single-line system prompt"
	blocks, err := p.CompleteSplit(prompt)
	if err != nil {
		t.Fatalf("CompleteSplit err = %v, want nil", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1 for single-line prompt", len(blocks))
	}
	tb, ok := blocks[0].(llm.ContentText)
	if !ok {
		t.Fatalf("blocks[0] type = %T, want llm.ContentText", blocks[0])
	}
	if tb.Text != prompt {
		t.Errorf("Text = %q, want %q", tb.Text, prompt)
	}
	if tb.CacheControl == nil {
		t.Fatalf("CacheControl = nil, want non-nil (whole prompt should be cached)")
	}
	if tb.CacheControl.Type != "ephemeral" {
		t.Errorf("CacheControl.Type = %q, want ephemeral", tb.CacheControl.Type)
	}
	if tb.CacheControl.TTL != "1h" {
		t.Errorf("CacheControl.TTL = %q, want 1h", tb.CacheControl.TTL)
	}
}

func TestCompleteSplit_MultiLineSplitsAtMidpointNewline(t *testing.T) {
	p := newProvider()
	first := strings.Repeat("a", 80)
	second := strings.Repeat("b", 80)
	third := strings.Repeat("c", 80)
	prompt := first + second + "\n" + third

	blocks, err := p.CompleteSplit(prompt)
	if err != nil {
		t.Fatalf("CompleteSplit err = %v, want nil", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2 (prefix cached, suffix not)", len(blocks))
	}

	prefix, ok := blocks[0].(llm.ContentText)
	if !ok {
		t.Fatalf("blocks[0] type = %T, want llm.ContentText", blocks[0])
	}
	if prefix.CacheControl == nil {
		t.Fatalf("prefix.CacheControl = nil, want non-nil")
	}
	if prefix.CacheControl.TTL != "1h" {
		t.Errorf("prefix.CacheControl.TTL = %q, want 1h", prefix.CacheControl.TTL)
	}

	suffix, ok := blocks[1].(llm.ContentText)
	if !ok {
		t.Fatalf("blocks[1] type = %T, want llm.ContentText", blocks[1])
	}
	if suffix.CacheControl != nil {
		t.Errorf("suffix.CacheControl = %+v, want nil (suffix should not be cached)", suffix.CacheControl)
	}

	if prefix.Text+suffix.Text != prompt {
		t.Errorf("prefix+suffix != original prompt\nprefix=%q\nsuffix=%q\nwant=%q",
			prefix.Text, suffix.Text, prompt)
	}

	if !strings.HasPrefix(prompt, prefix.Text) {
		t.Errorf("prefix not a prefix of prompt")
	}
	if strings.HasSuffix(prefix.Text, "\n") {
		t.Errorf("prefix %q unexpectedly ends with newline; split should exclude the newline", prefix.Text)
	}
	if !strings.HasPrefix(suffix.Text, "\n") {
		t.Errorf("suffix %q does not start with the split newline", suffix.Text)
	}
	if !strings.HasPrefix(suffix.Text, "\nc") {
		t.Errorf("suffix %q does not start with newline + c's", suffix.Text)
	}
	midpoint := len(prompt) / 2
	if len(prefix.Text) < midpoint {
		t.Errorf("prefix length = %d, want >= midpoint (%d) so the cached prefix covers the bulk", len(prefix.Text), midpoint)
	}
}

func TestCompleteSplit_CustomCacheControlApplied(t *testing.T) {
	_ = providers.NewMiniMaxProvider("k")
	oneH := llm.CacheEphemeral1h()
	if oneH == nil || oneH.Type != "ephemeral" || oneH.TTL != "1h" {
		t.Errorf("CacheEphemeral1h = %+v, want ephemeral/1h", oneH)
	}
	fiveMin := llm.CacheEphemeral()
	if fiveMin == nil || fiveMin.Type != "ephemeral" || fiveMin.TTL != "" {
		t.Errorf("CacheEphemeral = %+v, want ephemeral/empty", fiveMin)
	}

	prompt := "alpha\nbeta\ngamma\ndelta\nepsilon"
	blocks, err := newProvider().CompleteSplit(prompt)
	if err != nil {
		t.Fatalf("CompleteSplit err = %v", err)
	}
	if len(blocks) == 0 {
		t.Fatalf("len(blocks) = 0, want >0")
	}
	first, ok := blocks[0].(llm.ContentText)
	if !ok {
		t.Fatalf("blocks[0] type = %T", blocks[0])
	}
	if first.CacheControl == nil {
		t.Fatalf("first.CacheControl = nil")
	}
	if first.CacheControl.Type != "ephemeral" {
		t.Errorf("Type = %q, want ephemeral", first.CacheControl.Type)
	}
}
