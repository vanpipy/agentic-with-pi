// Package anthropic owns Anthropic Messages wire conversion and capability
// classification for the LLM V2 redesign. This file adds the
// family/version classifiers that the runtime, the catalogue, and (in a
// later sprint) the TUI effort cycler will share as a single source of
// truth.
//
// Mirrors jcode's crates/jcode-provider-core/src/anthropic.rs:62-381:
//   - ContextMode(model)     <- anthropic_context_mode
//   - ReasoningCaps(model)   <- anthropic_reasoning_caps
//   - MaxOutputTokens(model) <- anthropic_max_output_tokens
//   - AvailableReasoningEfforts(model) + BetaHeaders(model)
//
// The classifiers operate on a normalised form of the model id
// (trimmed, lowercased, [1m] suffix stripped, trailing -YYYYMMDD
// release-date suffix stripped, dotted versions collapsed to
// dashed). They are tolerant of mixed case, the [1m] suffix, dated
// release ids (claude-haiku-4-5-20251001), and dotted version forms
// (claude-opus-4.6). Future unknown generations are classified by
// parsed family/version and default optimistically to the modern
// ladder; the runtime self-heals on a 400 by stripping unsupported
// reasoning fields.
package anthropic

import (
	"strings"

	"github.com/vanpiyp/awp/internal/llm"
)

const (
	contextBetaHeader1M = "context-1m-2025-08-07"
	dateSuffixLength    = 8
)

// AnthropicReasoningCaps mirrors jcode's AnthropicReasoningCaps at
// crates/jcode-provider-core/src/anthropic.rs:231-243. It is the
// single source of truth for what reasoning controls a Claude model
// accepts on the live Messages API.
type AnthropicReasoningCaps struct {
	OutputEffort     bool
	AdaptiveThinking bool
	ManualThinking   bool
	XHighEffort      bool
	MaxEffort        bool
}

// SupportsReasoningEffort reports whether the model accepts any
// reasoning-effort control (either output_config.effort or a manual
// budget). Mirrors jcode at anthropic.rs:287-289.
func (c AnthropicReasoningCaps) SupportsReasoningEffort() bool {
	return c.OutputEffort || c.ManualThinking
}

// Reasoning-cap presets. Each is a complete spec returned directly
// by ReasoningCaps when its family/version pattern matches.
var (
	capsFull = AnthropicReasoningCaps{
		OutputEffort:     true,
		AdaptiveThinking: true,
		XHighEffort:      true,
		MaxEffort:        true,
	}
	capsEffortNoXhigh = AnthropicReasoningCaps{
		OutputEffort:     true,
		AdaptiveThinking: true,
		MaxEffort:        true,
	}
	capsManualWithEffort = AnthropicReasoningCaps{
		OutputEffort:   true,
		ManualThinking: true,
	}
	capsManualOnly = AnthropicReasoningCaps{
		ManualThinking: true,
	}
	capsNone = AnthropicReasoningCaps{}
)

// ContextMode reports the long-context surface for `model`. The
// returned mode is consumed by BetaHeaders (OptIn1M/Native1M emit the
// context-1m-2025-08-07 beta) and by ContextWindow fallback (unknown
// models default to 200K when Standard, the canonical Native1M
// when ≥5.x).
//
// Mirrors jcode's anthropic_context_mode at
// crates/jcode-provider-core/src/anthropic.rs:62-107.
func ContextMode(model string) llm.AnthropicContextMode {
	base := normalizeClaudeCapsKey(model)
	if !strings.HasPrefix(base, "claude") {
		return llm.ContextStandard
	}
	family, version := parseClaudeFamilyVersion(base)
	if version == nil {
		return llm.ContextStandard
	}
	switch family {
	case "opus", "sonnet":
		if cmpVersion(version, 4, 7) >= 0 {
			return llm.ContextNative1M
		}
		if version.major == 4 && version.minor == 6 {
			return llm.ContextOptIn1M
		}
		return llm.ContextStandard
	case "haiku":
		if cmpVersion(version, 5, 0) < 0 {
			return llm.ContextStandard
		}
		if cmpVersion(version, 5, 0) >= 0 {
			return llm.ContextNative1M
		}
	default:
		if cmpVersion(version, 5, 0) >= 0 {
			return llm.ContextNative1M
		}
	}
	return llm.ContextStandard
}

// ReasoningCaps reports the reasoning-effort capabilities of `model`.
// Mirrors jcode's anthropic_reasoning_caps at
// crates/jcode-provider-core/src/anthropic.rs:336-381.
func ReasoningCaps(model string) AnthropicReasoningCaps {
	base := normalizeClaudeCapsKey(model)
	if !strings.HasPrefix(base, "claude") {
		return capsNone
	}
	if strings.Contains(base, "mythos") {
		return capsEffortNoXhigh
	}
	family, version := parseClaudeFamilyVersion(base)
	if version == nil {
		return capsNone
	}
	switch family {
	case "opus":
		switch {
		case cmpVersion(version, 4, 7) >= 0:
			return capsFull
		case version.major == 4 && version.minor == 6:
			return capsEffortNoXhigh
		case version.major == 4 && version.minor == 5:
			return capsManualWithEffort
		}
	case "sonnet":
		switch {
		case cmpVersion(version, 5, 0) >= 0:
			return capsFull
		case version.major == 4 && version.minor == 6:
			return capsEffortNoXhigh
		case version.major == 3 && version.minor == 7:
			return capsManualOnly
		}
	default:
		if cmpVersion(version, 5, 0) >= 0 {
			return capsFull
		}
	}
	return capsNone
}

// MaxOutputTokens returns the published synchronous-Messages-API
// maximum output budget for `model`. Adaptive-thinking models spend
// their budget on thinking AND the visible tool call, so a budget
// that is too small truncates mid-tool-call on long agentic turns.
// The legacy flat 32K default never undercut is preserved as a
// regression guard.
//
// Mirrors jcode's anthropic_max_output_tokens at
// crates/jcode-provider-core/src/anthropic.rs:161-204.
func MaxOutputTokens(model string) int {
	base := strings.ToLower(Strip1mSuffix(strings.TrimSpace(model)))
	for _, prefix := range largeOutputPrefixes {
		if strings.HasPrefix(base, prefix) {
			return 128000
		}
	}
	for _, prefix := range haiku64kPrefixes {
		if strings.HasPrefix(base, prefix) {
			return 64000
		}
	}
	return 32768
}

// AvailableReasoningEfforts returns the reasoning-effort ladder
// filtered by the model's caps. Models that do not support any
// reasoning control return `["none"]`; the rest are filtered to drop
// `xhigh`/`max` when the model rejects them.
//
// The ladder order matches jcode's `swarm_root_reasoning_effort`
// enumeration: none, low, medium, high, xhigh, max.
func AvailableReasoningEfforts(model string) []string {
	caps := ReasoningCaps(model)
	if !caps.SupportsReasoningEffort() {
		return []string{"none"}
	}
	efforts := []string{"none", "low", "medium", "high"}
	if caps.XHighEffort {
		efforts = append(efforts, "xhigh")
	}
	if caps.MaxEffort {
		efforts = append(efforts, "max")
	}
	return efforts
}

// BetaHeaders returns the Anthropic beta headers required for `model`,
// or nil if no betas apply. The context-1m-2025-08-07 beta is only
// needed when the caller opts in via the explicit `[1m]` suffix —
// Native1M models (opus-4-7+, sonnet-5, fable-5) ship 1M as their
// default surface and do not require the beta. OptIn1M models
// (opus-4-6, sonnet-4-6) only reach 1M when the suffix is present.
// Mirrors jcode's anthropic_is_1m_model at
// crates/jcode-provider-core/src/anthropic.rs:109-113.
//
// Note: the OAuth path has a richer beta-header set
// (ANTHROPIC_OAUTH_BETA_HEADERS) — that surface ships in a later
// sprint alongside OAuth attribution headers.
func BetaHeaders(model string) []string {
	if strings.HasSuffix(model, "[1m]") {
		return []string{contextBetaHeader1M}
	}
	return nil
}

// Strip1mSuffix removes the `[1m]` long-context opt-in suffix from
// `model`, returning the bare id. Idempotent when the suffix is
// absent.
func Strip1mSuffix(model string) string {
	return strings.TrimSuffix(model, "[1m]")
}

// normalizeClaudeCapsKey returns the canonical id used by all
// classifier lookups: trimmed, ASCII-lowercased, [1m] stripped,
// trailing -YYYYMMDD release-date suffix stripped, dotted versions
// replaced with dashes. This is the same transform jcode applies at
// crates/jcode-provider-core/src/anthropic.rs:292-299.
func normalizeClaudeCapsKey(model string) string {
	base := strings.ReplaceAll(strings.ToLower(Strip1mSuffix(strings.TrimSpace(model))), ".", "-")
	return stripDateSuffix(base)
}

// stripDateSuffix removes a trailing 8-digit `-YYYYMMDD` release
// date so dated ids (`claude-haiku-4-5-20251001`) match bare
// canonical ids (`claude-haiku-4-5`). Mirrors jcode's
// strip_date_suffix at
// crates/jcode-provider-core/src/model_id.rs:43-50.
func stripDateSuffix(model string) string {
	idx := strings.LastIndex(model, "-")
	if idx < 0 {
		return model
	}
	tail := model[idx+1:]
	if len(tail) != dateSuffixLength {
		return model
	}
	for i := 0; i < len(tail); i++ {
		if tail[i] < '0' || tail[i] > '9' {
			return model
		}
	}
	return model[:idx]
}

// claudeVersion is a (major, minor) pair parsed from a Claude id.
// A single-version id (e.g. "claude-opus-5") becomes (5, 0).
type claudeVersion struct {
	major int
	minor int
}

func cmpVersion(v *claudeVersion, major, minor int) int {
	if v.major != major {
		return v.major - major
	}
	return v.minor - minor
}

// parseClaudeFamilyVersion extracts `(family, version)` from a
// normalised Claude id. Mirrors jcode at
// crates/jcode-provider-core/src/anthropic.rs:301-325. Handles both
// version-last (`claude-sonnet-4-6`) and version-first
// (`claude-3-7-sonnet`) forms. A single version segment means `.0`
// (`claude-opus-5` → 5.0).
func parseClaudeFamilyVersion(base string) (string, *claudeVersion) {
	var family string
	var nums []int
	for _, segment := range strings.Split(base, "-") {
		if segment == "claude" {
			continue
		}
		if n, ok := parseUintSegment(segment); ok {
			if len(nums) < 2 {
				nums = append(nums, n)
			}
			continue
		}
		if family == "" && isAllAsciiAlpha(segment) {
			family = segment
		}
	}
	if len(nums) == 0 {
		return family, nil
	}
	if len(nums) == 1 {
		return family, &claudeVersion{major: nums[0], minor: 0}
	}
	return family, &claudeVersion{major: nums[0], minor: nums[1]}
}

func parseUintSegment(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func isAllAsciiAlpha(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// largeOutputPrefixes lists model-name prefixes that allow 128K
// output tokens on the synchronous Messages API. Mirrors jcode's
// LARGE_OUTPUT_PREFIXES at
// crates/jcode-provider-core/src/anthropic.rs:174-189.
var largeOutputPrefixes = []string{
	"claude-opus-5",
	"claude-opus-4-8",
	"claude-opus-4.8",
	"claude-opus-4-7",
	"claude-opus-4.7",
	"claude-opus-4-6",
	"claude-opus-4.6",
	"claude-sonnet-5",
	"claude-sonnet-4-6",
	"claude-sonnet-4.6",
	"claude-fable-5-1",
	"claude-fable-5",
	"claude-fable",
	"claude-mythos",
}

var haiku64kPrefixes = []string{
	"claude-haiku-4-5",
	"claude-haiku-4.5",
}
