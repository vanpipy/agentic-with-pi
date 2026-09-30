package llm_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

// contentTextOf asserts a block is llm.ContentText and returns its value.
// Tests use it as a soft type-assertion helper to keep table rows readable.
func contentTextOf(t *testing.T, b llm.ContentBlock) llm.ContentText {
	t.Helper()
	ct, ok := b.(llm.ContentText)
	if !ok {
		t.Fatalf("block is %T, want llm.ContentText", b)
	}
	return ct
}

func TestSplitSystemPromptEmptyReturnsNil(t *testing.T) {
	got := llm.SplitSystemPrompt("", true)
	if got != nil {
		t.Fatalf("got = %v, want nil", got)
	}
}

func TestSplitSystemPromptNoMidNewlineReturnsSingleBlock(t *testing.T) {
	prompt := strings.Repeat("a", 80)
	got := llm.SplitSystemPrompt(prompt, true)
	if len(got) != 1 {
		t.Fatalf("got %d blocks, want 1", len(got))
	}
	ct := contentTextOf(t, got[0])
	if ct.Text != prompt {
		t.Errorf("Text = %q, want %q", ct.Text, prompt)
	}
	if ct.CacheControl != nil {
		t.Errorf("single-block path should not attach cache_control, got %+v", ct.CacheControl)
	}
}

func TestSplitSystemPromptSplitsAtFirstMidNewline(t *testing.T) {
	// Prompt is 40 a's + newline + 40 b's + newline + 40 c's = 122 bytes.
	// Midpoint = 61. The first newline at position >= 61 is the one
	// after the b's (position 81). The static prefix therefore includes
	// the entire a-block and b-block; the dynamic suffix is just the
	// c-block. Picking the later newline keeps the static prefix stable
	// when the user grows the dynamic tail.
	prefix := strings.Repeat("a", 40)
	mid := strings.Repeat("b", 40)
	suffix := strings.Repeat("c", 40)
	prompt := prefix + "\n" + mid + "\n" + suffix
	got := llm.SplitSystemPrompt(prompt, true)
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2", len(got))
	}
	static := contentTextOf(t, got[0])
	dynamic := contentTextOf(t, got[1])
	wantStatic := prefix + "\n" + mid + "\n"
	if static.Text != wantStatic {
		t.Errorf("static Text = %q, want %q", static.Text, wantStatic)
	}
	if dynamic.Text != suffix {
		t.Errorf("dynamic Text = %q, want %q", dynamic.Text, suffix)
	}
	if static.CacheControl == nil {
		t.Errorf("static block should carry CacheControl when supportsCache=true")
	}
	if dynamic.CacheControl != nil {
		t.Errorf("dynamic block must not carry CacheControl, got %+v", dynamic.CacheControl)
	}
}

func TestSplitSystemPromptNoCacheOmitsCacheControl(t *testing.T) {
	prefix := strings.Repeat("a", 40)
	suffix := strings.Repeat("b", 40)
	prompt := prefix + "\n" + suffix
	got := llm.SplitSystemPrompt(prompt, false)
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2", len(got))
	}
	static := contentTextOf(t, got[0])
	dynamic := contentTextOf(t, got[1])
	if static.CacheControl != nil {
		t.Errorf("static.CacheControl = %+v, want nil when supportsCache=false", static.CacheControl)
	}
	if dynamic.CacheControl != nil {
		t.Errorf("dynamic.CacheControl = %+v, want nil", dynamic.CacheControl)
	}
}

func TestSplitSystemPromptCutAtByteMidpoint(t *testing.T) {
	// 100 bytes of 'a' then newline at byte 50 then more bytes. The cut
	// should land exactly on the mid-newline (not the first newline).
	prefix := strings.Repeat("a", 50)
	tail := strings.Repeat("b", 50)
	prompt := prefix + "\n" + tail
	got := llm.SplitSystemPrompt(prompt, true)
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2", len(got))
	}
	static := contentTextOf(t, got[0])
	if static.Text != prefix+"\n" {
		t.Errorf("static Text = %q, want %q", static.Text, prefix+"\n")
	}
	if !strings.HasPrefix(static.Text, prefix) {
		t.Errorf("static.Text should start with the original prefix")
	}
}

func TestSplitSystemPromptStaticOnlyWhenSuffixEmpty(t *testing.T) {
	// Newline right at midpoint with nothing after — should yield one
	// static block (no dynamic).
	prefix := strings.Repeat("x", 40)
	prompt := prefix + "\n"
	got := llm.SplitSystemPrompt(prompt, true)
	if len(got) != 1 {
		t.Fatalf("got %d blocks, want 1 (no dynamic suffix)", len(got))
	}
	ct := contentTextOf(t, got[0])
	if ct.Text != prefix+"\n" {
		t.Errorf("Text = %q, want %q (newline stays in static)", ct.Text, prefix+"\n")
	}
}

func TestSplitSystemPromptAllCases(t *testing.T) {
	cases := []struct {
		name        string
		prompt      string
		cache       bool
		wantBlocks  int
		wantStatic  string
		wantDynamic string
		wantCache   bool
	}{
		{
			name:       "empty",
			prompt:     "",
			cache:      true,
			wantBlocks: 0,
		},
		{
			name:       "no-newline",
			prompt:     strings.Repeat("x", 100),
			cache:      true,
			wantBlocks: 1,
			wantStatic: strings.Repeat("x", 100),
			wantCache:  false,
		},
		{
			name:        "split-with-cache",
			prompt:      strings.Repeat("a", 60) + "\n" + strings.Repeat("b", 60),
			cache:       true,
			wantBlocks:  2,
			wantStatic:  strings.Repeat("a", 60) + "\n",
			wantDynamic: strings.Repeat("b", 60),
			wantCache:   true,
		},
		{
			name:        "split-no-cache",
			prompt:      strings.Repeat("a", 60) + "\n" + strings.Repeat("b", 60),
			cache:       false,
			wantBlocks:  2,
			wantStatic:  strings.Repeat("a", 60) + "\n",
			wantDynamic: strings.Repeat("b", 60),
			wantCache:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := llm.SplitSystemPrompt(tc.prompt, tc.cache)
			if len(got) != tc.wantBlocks {
				t.Fatalf("got %d blocks, want %d", len(got), tc.wantBlocks)
			}
			if tc.wantBlocks >= 1 {
				static := contentTextOf(t, got[0])
				if static.Text != tc.wantStatic {
					t.Errorf("static Text = %q, want %q", static.Text, tc.wantStatic)
				}
				if (static.CacheControl != nil) != tc.wantCache {
					t.Errorf("static.CacheControl presence = %+v, want presence=%v", static.CacheControl, tc.wantCache)
				}
			}
			if tc.wantBlocks >= 2 {
				dynamic := contentTextOf(t, got[1])
				if dynamic.Text != tc.wantDynamic {
					t.Errorf("dynamic Text = %q, want %q", dynamic.Text, tc.wantDynamic)
				}
				if dynamic.CacheControl != nil {
					t.Errorf("dynamic.CacheControl = %+v, want nil", dynamic.CacheControl)
				}
			}
		})
	}
}
