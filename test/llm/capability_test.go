package llm_test

import (
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestAnthropicContextModeValues(t *testing.T) {
	if llm.ContextStandard != 0 {
		t.Fatalf("ContextStandard = %d, want 0", llm.ContextStandard)
	}
	if llm.ContextOptIn1M != 1 {
		t.Fatalf("ContextOptIn1M = %d, want 1", llm.ContextOptIn1M)
	}
	if llm.ContextNative1M != 2 {
		t.Fatalf("ContextNative1M = %d, want 2", llm.ContextNative1M)
	}
}

func TestModelCapabilitiesPayload(t *testing.T) {
	caps := llm.ModelCapabilities{
		ID:                    "claude-opus-4-7",
		ContextWindow:         200000,
		MaxOutputTokens:       8192,
		SupportsTools:         true,
		SupportsVision:        true,
		SupportsCache:         true,
		SupportsCacheTTL1h:    true,
		SupportsNativeCompact: true,
		SupportsThinking:      true,
		ReasoningEfforts:      []string{"low", "medium", "high"},
		ServiceTiers:          []string{"standard", "priority"},
		BetaHeaders:           []string{"prompt-caching-2024-07-31"},
		ContextMode:           llm.ContextNative1M,
	}
	if caps.ID != "claude-opus-4-7" {
		t.Fatalf("ID = %q, want %q", caps.ID, "claude-opus-4-7")
	}
	if caps.ContextWindow != 200000 {
		t.Fatalf("ContextWindow = %d, want 200000", caps.ContextWindow)
	}
	if caps.MaxOutputTokens != 8192 {
		t.Fatalf("MaxOutputTokens = %d, want 8192", caps.MaxOutputTokens)
	}
	if !caps.SupportsTools {
		t.Fatalf("SupportsTools = false, want true")
	}
	if !caps.SupportsVision {
		t.Fatalf("SupportsVision = false, want true")
	}
	if !caps.SupportsCache {
		t.Fatalf("SupportsCache = false, want true")
	}
	if !caps.SupportsCacheTTL1h {
		t.Fatalf("SupportsCacheTTL1h = false, want true")
	}
	if !caps.SupportsNativeCompact {
		t.Fatalf("SupportsNativeCompact = false, want true")
	}
	if !caps.SupportsThinking {
		t.Fatalf("SupportsThinking = false, want true")
	}
	if len(caps.ReasoningEfforts) != 3 {
		t.Fatalf("len(ReasoningEfforts) = %d, want 3", len(caps.ReasoningEfforts))
	}
	if len(caps.ServiceTiers) != 2 {
		t.Fatalf("len(ServiceTiers) = %d, want 2", len(caps.ServiceTiers))
	}
	if len(caps.BetaHeaders) != 1 {
		t.Fatalf("len(BetaHeaders) = %d, want 1", len(caps.BetaHeaders))
	}
	if caps.ContextMode != llm.ContextNative1M {
		t.Fatalf("ContextMode = %d, want %d", caps.ContextMode, llm.ContextNative1M)
	}
}

func TestRouteSelectionPayload(t *testing.T) {
	rs := llm.RouteSelection{
		Model:     "claude-opus-4-7",
		Provider:  "anthropic",
		APIMethod: "messages",
		Detail:    "native-1m",
	}
	if rs.Model != "claude-opus-4-7" {
		t.Fatalf("Model = %q, want %q", rs.Model, "claude-opus-4-7")
	}
	if rs.Provider != "anthropic" {
		t.Fatalf("Provider = %q, want %q", rs.Provider, "anthropic")
	}
	if rs.APIMethod != "messages" {
		t.Fatalf("APIMethod = %q, want %q", rs.APIMethod, "messages")
	}
	if rs.Detail != "native-1m" {
		t.Fatalf("Detail = %q, want %q", rs.Detail, "native-1m")
	}
}

func TestNativeCompactionResultPayload(t *testing.T) {
	r := llm.NativeCompactionResult{
		Summary:          "session so far",
		EncryptedContent: "enc-blob",
		PreTokens:        180000,
		PostTokens:       4500,
	}
	if r.Summary != "session so far" {
		t.Fatalf("Summary = %q, want %q", r.Summary, "session so far")
	}
	if r.EncryptedContent != "enc-blob" {
		t.Fatalf("EncryptedContent = %q, want %q", r.EncryptedContent, "enc-blob")
	}
	if r.PreTokens != 180000 {
		t.Fatalf("PreTokens = %d, want 180000", r.PreTokens)
	}
	if r.PostTokens != 4500 {
		t.Fatalf("PostTokens = %d, want 4500", r.PostTokens)
	}
}

func TestSummaryPayload(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)
	s := llm.Summary{
		Text:      "user wants to refactor auth",
		CreatedAt: now,
		Source:    "native",
	}
	if s.Text != "user wants to refactor auth" {
		t.Fatalf("Text = %q, want %q", s.Text, "user wants to refactor auth")
	}
	if !s.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", s.CreatedAt, now)
	}
	if s.Source != "native" {
		t.Fatalf("Source = %q, want %q", s.Source, "native")
	}
}

func TestAnthropicContextModeDistinct(t *testing.T) {
	modes := []llm.AnthropicContextMode{
		llm.ContextStandard,
		llm.ContextOptIn1M,
		llm.ContextNative1M,
	}
	seen := make(map[llm.AnthropicContextMode]bool, len(modes))
	for _, m := range modes {
		if seen[m] {
			t.Fatalf("duplicate mode value: %d", m)
		}
		seen[m] = true
	}
	if len(seen) != 3 {
		t.Fatalf("distinct mode count = %d, want 3", len(seen))
	}
}
