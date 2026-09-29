package protocol_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

func TestContextMode_KnownGenerations(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  llm.AnthropicContextMode
	}{
		// Opus / Sonnet: 4.7+ native, 4.6 opt-in, ≤4.5 standard.
		{"opus-5-native", "claude-opus-5", llm.ContextNative1M},
		{"opus-4-8-native", "claude-opus-4-8", llm.ContextNative1M},
		{"opus-4-7-native", "claude-opus-4-7", llm.ContextNative1M},
		{"opus-4-6-opt-in", "claude-opus-4-6", llm.ContextOptIn1M},
		{"opus-4-5-standard", "claude-opus-4-5", llm.ContextStandard},
		{"opus-4-1-standard", "claude-opus-4-1", llm.ContextStandard},
		{"sonnet-5-native", "claude-sonnet-5", llm.ContextNative1M},
		{"sonnet-4-6-opt-in", "claude-sonnet-4-6", llm.ContextOptIn1M},
		{"sonnet-4-5-standard", "claude-sonnet-4-5", llm.ContextStandard},
		{"sonnet-3-7-standard", "claude-3-7-sonnet", llm.ContextStandard},

		// Haiku: 4.x standard, 5.x native (optimistic).
		{"haiku-4-5-standard", "claude-haiku-4-5", llm.ContextStandard},
		{"haiku-5-native", "claude-haiku-5", llm.ContextNative1M},

		// Future / unknown families: optimistic default for ≥5.x.
		{"fable-5-1-native", "claude-fable-5-1", llm.ContextNative1M},
		{"fable-5-native", "claude-fable-5", llm.ContextNative1M},
		{"fable-4-standard", "claude-fable-4", llm.ContextStandard},
		{"mythos-native", "claude-mythos-7", llm.ContextNative1M},
		{"nova-6-native", "claude-nova-6", llm.ContextNative1M},
		{"unknown-4-standard", "claude-nova-4", llm.ContextStandard},

		// Non-Claude ids default to standard.
		{"gpt", "gpt-5", llm.ContextStandard},
		{"empty", "", llm.ContextStandard},

		// Mixed case, dotted versions, [1m] suffix, dated release — all
		// normalise to the same answer.
		{"mixed-case", "Claude-Opus-4-6", llm.ContextOptIn1M},
		{"dotted-version", "claude-opus-4.6", llm.ContextOptIn1M},
		{"with-1m-suffix", "claude-opus-4-6[1m]", llm.ContextOptIn1M},
		{"dated-release", "claude-haiku-4-5-20251001", llm.ContextStandard},
		{"dated-opus-4-6", "Claude-Opus-4-6-20251001", llm.ContextOptIn1M},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropic.ContextMode(tc.model)
			if got != tc.want {
				t.Errorf("ContextMode(%q) = %d, want %d", tc.model, got, tc.want)
			}
		})
	}
}

func TestReasoningCaps_KnownGenerations(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  anthropic.AnthropicReasoningCaps
	}{
		{"opus-5-full", "claude-opus-5", capsFull()},
		{"opus-4-8-full", "claude-opus-4-8", capsFull()},
		{"opus-4-7-full", "claude-opus-4-7", capsFull()},
		{"opus-4-6-no-xhigh", "claude-opus-4-6", capsEffortNoXhigh()},
		{"opus-4-5-manual-with-effort", "claude-opus-4-5", capsManualWithEffort()},
		{"opus-4-1-none", "claude-opus-4-1", capsNone()},
		{"sonnet-5-full", "claude-sonnet-5", capsFull()},
		{"sonnet-4-6-no-xhigh", "claude-sonnet-4-6", capsEffortNoXhigh()},
		{"sonnet-3-7-manual-only", "claude-3-7-sonnet", capsManualOnly()},
		{"sonnet-3-7-manual-only-version-last", "claude-sonnet-3-7", capsManualOnly()},
		{"sonnet-4-5-none", "claude-sonnet-4-5", capsNone()},
		{"haiku-4-5-none", "claude-haiku-4-5", capsNone()},
		{"haiku-5-full", "claude-haiku-5", capsFull()},
		{"fable-5-full", "claude-fable-5", capsFull()},
		{"fable-5-1-full", "claude-fable-5-1", capsFull()},
		{"mythos-effort-no-xhigh", "claude-mythos", capsEffortNoXhigh()},
		{"unknown-4-none", "claude-nova-4", capsNone()},
		{"unknown-5-full", "claude-nova-5", capsFull()},
		{"unknown-6-full", "claude-nova-6", capsFull()},
		{"gpt", "gpt-5", capsNone()},
		{"empty", "", capsNone()},
		{"haiku-4-5-dated", "claude-haiku-4-5-20251001", capsNone()},
		{"opus-4-6-mixed-case", "Claude-Opus-4-6", capsEffortNoXhigh()},
		{"opus-4-6-dotted", "claude-opus-4.6", capsEffortNoXhigh()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropic.ReasoningCaps(tc.model)
			if got != tc.want {
				t.Errorf("ReasoningCaps(%q) = %+v, want %+v", tc.model, got, tc.want)
			}
		})
	}
}

func TestReasoningCaps_OptimisticDefaults(t *testing.T) {
	// Every unknown 5.x+ Claude id resolves to the full modern ladder.
	full := capsFull()
	for _, model := range []string{
		"claude-sonnet-6",
		"claude-opus-5",
		"claude-haiku-5",
		"claude-fable-6",
		"claude-nova-5",
	} {
		if got := anthropic.ReasoningCaps(model); got != full {
			t.Errorf("ReasoningCaps(%q) = %+v, want full %+v", model, got, full)
		}
	}
	// Old/unversioned ids stay conservative.
	for _, model := range []string{
		"claude-haiku-4-5",
		"claude-instant",
	} {
		if got := anthropic.ReasoningCaps(model); got.SupportsReasoningEffort() {
			t.Errorf("ReasoningCaps(%q).SupportsReasoningEffort() = true, want false", model)
		}
	}
}

func TestMaxOutputTokens_PublishedLimits(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  int
	}{
		{"opus-5-128k", "claude-opus-5", 128000},
		{"opus-4-8-128k", "claude-opus-4-8", 128000},
		{"opus-4-8-dotted-128k", "claude-opus-4.8", 128000},
		{"opus-4-7-128k", "claude-opus-4-7", 128000},
		{"opus-4-6-128k", "claude-opus-4-6", 128000},
		{"opus-4-6-1m-128k", "claude-opus-4-6[1m]", 128000},
		{"opus-4-6-dated-128k", "Claude-Opus-4-6-20251001", 128000},
		{"sonnet-5-128k", "claude-sonnet-5", 128000},
		{"sonnet-4-6-128k", "claude-sonnet-4-6", 128000},
		{"sonnet-4-6-dotted-128k", "claude-sonnet-4.6", 128000},
		{"fable-5-128k", "claude-fable-5", 128000},
		{"fable-5-1-128k", "claude-fable-5-1", 128000},
		{"mythos-128k", "claude-mythos", 128000},
		{"haiku-4-5-64k", "claude-haiku-4-5", 64000},
		{"haiku-4-5-dotted-64k", "claude-haiku-4.5", 64000},
		{"haiku-4-5-dated-64k", "claude-haiku-4-5-20251001", 64000},
		{"opus-4-5-32k", "claude-opus-4-5", 32768},
		{"sonnet-4-5-32k", "claude-sonnet-4-5", 32768},
		{"sonnet-4-dated-32k", "claude-sonnet-4-20250514", 32768},
		{"instant-32k", "claude-instant", 32768},
		{"unknown-32k", "claude-nova-4", 32768},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropic.MaxOutputTokens(tc.model)
			if got != tc.want {
				t.Errorf("MaxOutputTokens(%q) = %d, want %d", tc.model, got, tc.want)
			}
		})
	}
}

func TestMaxOutputTokens_NeverUndercutsLegacyDefault(t *testing.T) {
	// Regression guard: a per-model budget must never be *smaller* than
	// the legacy 32K default Sprint 4 shipped, or turns that used to
	// fit will start truncating.
	for _, model := range []string{
		"claude-opus-5", "claude-opus-5-5", "claude-opus-5-50",
		"claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
		"claude-sonnet-5", "claude-sonnet-4-6",
		"claude-fable-5-1", "claude-fable-5", "claude-mythos",
		"claude-haiku-4-5",
	} {
		if got := anthropic.MaxOutputTokens(model); got < 32768 {
			t.Errorf("MaxOutputTokens(%q) = %d, want >= 32768", model, got)
		}
	}
}

func TestBetaHeaders(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  []string
	}{
		{"opus-4-6-no-beta", "claude-opus-4-6", nil},
		{"opus-4-6-1m-beta", "claude-opus-4-6[1m]", []string{"context-1m-2025-08-07"}},
		{"opus-4-7-native-no-beta", "claude-opus-4-7", nil},
		{"opus-4-7-1m-beta", "claude-opus-4-7[1m]", []string{"context-1m-2025-08-07"}},
		{"opus-5-native-no-beta", "claude-opus-5", nil},
		{"sonnet-5-native-no-beta", "claude-sonnet-5", nil},
		{"sonnet-4-6-no-beta", "claude-sonnet-4-6", nil},
		{"sonnet-4-6-1m-beta", "claude-sonnet-4-6[1m]", []string{"context-1m-2025-08-07"}},
		{"haiku-4-5-no-beta", "claude-haiku-4-5", nil},
		{"haiku-5-native-no-beta", "claude-haiku-5", nil},
		{"non-claude-no-beta", "gpt-5", nil},
		{"unknown-no-beta", "claude-nova-4", nil},
		{"dotted-1m-beta", "claude-opus-4.6[1m]", []string{"context-1m-2025-08-07"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropic.BetaHeaders(tc.model)
			if !stringSliceEqual(got, tc.want) {
				t.Errorf("BetaHeaders(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestAvailableReasoningEfforts_FiltersLadder(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  []string
	}{
		{"opus-5-full-ladder", "claude-opus-5", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"opus-4-8-full-ladder", "claude-opus-4-8", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"opus-4-7-full-ladder", "claude-opus-4-7", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"opus-4-6-no-xhigh", "claude-opus-4-6", []string{"none", "low", "medium", "high", "max"}},
		{"sonnet-5-full-ladder", "claude-sonnet-5", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"sonnet-4-6-no-xhigh", "claude-sonnet-4-6", []string{"none", "low", "medium", "high", "max"}},
		{"sonnet-3-7-manual-ladder", "claude-3-7-sonnet", []string{"none", "low", "medium", "high"}},
		{"opus-4-5-manual-ladder", "claude-opus-4-5", []string{"none", "low", "medium", "high"}},
		{"haiku-4-5-only-none", "claude-haiku-4-5", []string{"none"}},
		{"unknown-5-full-ladder", "claude-nova-5", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"gpt", "gpt-5", []string{"none"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropic.AvailableReasoningEfforts(tc.model)
			if !stringSliceEqual(got, tc.want) {
				t.Errorf("AvailableReasoningEfforts(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestStrip1mSuffix(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"claude-opus-4-6", "claude-opus-4-6"},
		{"claude-opus-4-6[1m]", "claude-opus-4-6"},
		{"claude-haiku-5[1m]", "claude-haiku-5"},
		{"[1m]", ""},
		{"", ""},
		{"foo[1m]bar", "foo[1m]bar"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := anthropic.Strip1mSuffix(tc.in); got != tc.want {
				t.Errorf("Strip1mSuffix(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestContextMode_NormalisesDottedAndSuffixed(t *testing.T) {
	// Same canonical id in different forms must produce the same answer.
	probes := []struct {
		a, b string
	}{
		{"claude-sonnet-5", "claude-sonnet-5[1m]"},
		{"claude-opus-4-6", "claude-opus-4-6[1m]"},
		{"claude-sonnet-4-6", "Claude-Sonnet-4-6"},
		{"claude-opus-4-6", "claude-opus-4.6"},
		{"claude-haiku-4-5", "claude-haiku-4-5-20251001"},
	}
	for _, p := range probes {
		if a, b := anthropic.ContextMode(p.a), anthropic.ContextMode(p.b); a != b {
			t.Errorf("ContextMode(%q)=%d != ContextMode(%q)=%d", p.a, a, p.b, b)
		}
	}
}

func TestReasoningCaps_NormalisesDottedAndSuffixed(t *testing.T) {
	probes := []struct {
		a, b string
	}{
		{"claude-sonnet-5", "claude-sonnet-5[1m]"},
		{"claude-opus-4-6", "claude-opus-4-6[1m]"},
		{"claude-sonnet-4-6", "Claude-Sonnet-4-6"},
		{"claude-opus-4-6", "claude-opus-4.6"},
		{"claude-haiku-4-5", "claude-haiku-4-5-20251001"},
		{"claude-sonnet-3-7", "claude-3-7-sonnet"},
	}
	for _, p := range probes {
		if a, b := anthropic.ReasoningCaps(p.a), anthropic.ReasoningCaps(p.b); a != b {
			t.Errorf("ReasoningCaps(%q)=%+v != ReasoningCaps(%q)=%+v", p.a, a, p.b, b)
		}
	}
}

func capsFull() anthropic.AnthropicReasoningCaps {
	return anthropic.AnthropicReasoningCaps{
		OutputEffort: true, AdaptiveThinking: true, XHighEffort: true, MaxEffort: true,
	}
}

func capsEffortNoXhigh() anthropic.AnthropicReasoningCaps {
	return anthropic.AnthropicReasoningCaps{
		OutputEffort: true, AdaptiveThinking: true, MaxEffort: true,
	}
}

func capsManualWithEffort() anthropic.AnthropicReasoningCaps {
	return anthropic.AnthropicReasoningCaps{
		OutputEffort: true, ManualThinking: true,
	}
}

func capsManualOnly() anthropic.AnthropicReasoningCaps {
	return anthropic.AnthropicReasoningCaps{ManualThinking: true}
}

func capsNone() anthropic.AnthropicReasoningCaps {
	return anthropic.AnthropicReasoningCaps{}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
