package llm

import "strings"

// SplitSystemPrompt divides a system prompt into a cacheable static prefix
// and a dynamic suffix so providers with prompt-cache support can attach
// cache_control to the prefix and avoid re-tokenizing it on every turn.
//
// The split point is the first '\n' at or after the byte midpoint of the
// prompt. Splitting at the midpoint keeps the static prefix stable across
// user-supplied dynamic tails while still leaving room for suffixes that
// grow longer than the prefix.
//
// The separator newline is included in the static prefix so that
// concatenating static.Text + dynamic.Text reproduces the original prompt
// byte-for-byte (the cache hash must be invariant under whether the split
// is applied or not).
//
// When supportsCache is true, the static prefix carries CacheEphemeral.
// Callers should pass SupportsCacheControl(model) for the target model.
//
// Returns nil for an empty prompt, a single block when no mid-point
// newline exists, or two blocks when a split point is found.
func SplitSystemPrompt(prompt string, supportsCache bool) []ContentBlock {
	if prompt == "" {
		return nil
	}
	mid := len(prompt) / 2
	nl := strings.Index(prompt[mid:], "\n")
	if nl < 0 {
		return []ContentBlock{ContentText{Text: prompt}}
	}
	cut := mid + nl + 1
	static := prompt[:cut]
	dynamic := prompt[cut:]
	blocks := make([]ContentBlock, 0, 2)
	if static != "" {
		block := ContentText{Text: static}
		if supportsCache {
			block.CacheControl = CacheEphemeral()
		}
		blocks = append(blocks, block)
	}
	if dynamic != "" {
		blocks = append(blocks, ContentText{Text: dynamic})
	}
	return blocks
}
