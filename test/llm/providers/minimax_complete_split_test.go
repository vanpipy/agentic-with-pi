package providers_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

func TestMiniMaxProviderCompleteSplitDelegatesToSupportsCache(t *testing.T) {
	p := providers.NewMiniMaxProvider("test-key")

	t.Run("cache-supporting-model-attaches-cache-control", func(t *testing.T) {
		// "MiniMax-M3" is the production model id. If its cache support
		// changes, this test name will surface that. We assert on the
		// observable outcome: when SupportsCacheControl returns true,
		// CompleteSplit attaches CacheEphemeral to the static prefix.
		// When it returns false, the prefix is plain text. The decision
		// is delegated to SupportsCacheControl; the test verifies both
		// halves of that delegation path.
		const cacheModel = "MiniMax-M3"
		if !p.SupportsCacheControl(cacheModel) {
			t.Skipf("model %q no longer supports cache_control; update this test", cacheModel)
		}
		prefix := strings.Repeat("a", 60)
		suffix := strings.Repeat("b", 60)
		prompt := prefix + "\n" + suffix
		got := p.CompleteSplit(prompt, cacheModel)
		if len(got) != 2 {
			t.Fatalf("got %d blocks, want 2", len(got))
		}
		static, ok := got[0].(llm.ContentText)
		if !ok {
			t.Fatalf("static block is %T, want llm.ContentText", got[0])
		}
		if static.CacheControl == nil {
			t.Errorf("static block has no CacheControl; expected CacheEphemeral on cache-supporting model")
		}
		dynamic, ok := got[1].(llm.ContentText)
		if !ok {
			t.Fatalf("dynamic block is %T, want llm.ContentText", got[1])
		}
		if dynamic.CacheControl != nil {
			t.Errorf("dynamic block has CacheControl = %+v; must remain nil", dynamic.CacheControl)
		}
	})

	t.Run("unknown-model-skips-cache-control", func(t *testing.T) {
		// An unknown model id falls through to MiniMax's conservative
		// defaults. CompleteSplit should still produce split blocks but
		// without CacheControl, since SupportsCacheControl returns false.
		prefix := strings.Repeat("a", 60)
		suffix := strings.Repeat("b", 60)
		prompt := prefix + "\n" + suffix
		got := p.CompleteSplit(prompt, "unknown-model")
		if len(got) != 2 {
			t.Fatalf("got %d blocks, want 2", len(got))
		}
		static, ok := got[0].(llm.ContentText)
		if !ok {
			t.Fatalf("static block is %T, want llm.ContentText", got[0])
		}
		if static.CacheControl != nil {
			t.Errorf("static block has CacheControl = %+v on unknown model; want nil", static.CacheControl)
		}
	})

	t.Run("empty-prompt-returns-nil", func(t *testing.T) {
		got := p.CompleteSplit("", "MiniMax-M3")
		if got != nil {
			t.Errorf("got %v blocks, want nil for empty prompt", len(got))
		}
	})

	t.Run("no-newline-returns-single-block-no-cache", func(t *testing.T) {
		prompt := strings.Repeat("x", 80)
		got := p.CompleteSplit(prompt, "MiniMax-M3")
		if len(got) != 1 {
			t.Fatalf("got %d blocks, want 1 (no mid-newline)", len(got))
		}
		ct, ok := got[0].(llm.ContentText)
		if !ok {
			t.Fatalf("block is %T, want llm.ContentText", got[0])
		}
		if ct.CacheControl != nil {
			t.Errorf("single-block fallback should not attach CacheControl, got %+v", ct.CacheControl)
		}
		if ct.Text != prompt {
			t.Errorf("Text = %q, want %q", ct.Text, prompt)
		}
	})
}
