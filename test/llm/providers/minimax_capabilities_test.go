package providers_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/providers"
)

var _ llm.Provider = (*providers.MiniMaxProvider)(nil)

func TestProviderInterface_AllMethodsImplemented(t *testing.T) {
	var p llm.Provider = providers.NewMiniMaxProvider("test-key")
	if p.Name() != "minimax" {
		t.Fatalf("Name = %q, want minimax", p.Name())
	}
}

type minimaxCapabilityCase struct {
	id                    string
	contextWindow         int
	maxOutputTokens       int
	supportsCache         bool
	supportsCacheTTL1h    bool
	supportsNativeCompact bool
	supportsTools         bool
	supportsVision        bool
	supportsThinking      bool
}

var minimaxCapabilityCases = []minimaxCapabilityCase{
	{"MiniMax-M2", 200000, 8192, true, true, false, true, true, true},
	{"MiniMax-M2.1", 200000, 8192, true, true, false, true, true, true},
	{"MiniMax-M2.1-highspeed", 200000, 8192, true, true, false, true, true, false},
	{"MiniMax-M2.5", 200000, 8192, true, true, false, true, true, true},
	{"MiniMax-M2.5-highspeed", 200000, 8192, true, true, false, true, true, false},
	{"MiniMax-M2.7", 200000, 8192, true, true, false, true, true, true},
	{"MiniMax-M2.7-highspeed", 200000, 8192, true, true, false, true, true, false},
	{"MiniMax-M3", 200000, 8192, true, true, false, true, true, true},
}

func TestSupportsCacheControl(t *testing.T) {
	p := newProvider()
	for _, c := range minimaxCapabilityCases {
		if !p.SupportsCacheControl(c.id) {
			t.Errorf("SupportsCacheControl(%q) = false, want true", c.id)
		}
	}
}

func TestContextWindow(t *testing.T) {
	p := newProvider()
	for _, c := range minimaxCapabilityCases {
		got := p.ContextWindow(c.id)
		if got != c.contextWindow {
			t.Errorf("ContextWindow(%q) = %d, want %d", c.id, got, c.contextWindow)
		}
	}
}

func TestMaxOutputTokens(t *testing.T) {
	p := newProvider()
	for _, c := range minimaxCapabilityCases {
		got := p.MaxOutputTokens(c.id)
		if got != c.maxOutputTokens {
			t.Errorf("MaxOutputTokens(%q) = %d, want %d", c.id, got, c.maxOutputTokens)
		}
	}
}

func TestAvailableReasoningEfforts(t *testing.T) {
	p := newProvider()
	for _, c := range minimaxCapabilityCases {
		got := p.AvailableReasoningEfforts(c.id)
		if len(got) != 0 {
			t.Errorf("AvailableReasoningEfforts(%q) = %v, want empty slice", c.id, got)
		}
	}
}

func TestAvailableServiceTiers(t *testing.T) {
	p := newProvider()
	for _, c := range minimaxCapabilityCases {
		got := p.AvailableServiceTiers(c.id)
		if len(got) != 0 {
			t.Errorf("AvailableServiceTiers(%q) = %v, want empty slice", c.id, got)
		}
	}
}

func TestBetaHeaders(t *testing.T) {
	p := newProvider()
	for _, c := range minimaxCapabilityCases {
		got := p.BetaHeaders(c.id)
		if len(got) != 0 {
			t.Errorf("BetaHeaders(%q) = %v, want empty slice for standard context", c.id, got)
		}
	}
}

func TestModelCapabilities(t *testing.T) {
	p := newProvider()
	got := p.ModelCapabilities("MiniMax-M3")
	if got.ID != "MiniMax-M3" {
		t.Errorf("ID = %q, want MiniMax-M3", got.ID)
	}
	if got.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", got.ContextWindow)
	}
	if got.MaxOutputTokens != 8192 {
		t.Errorf("MaxOutputTokens = %d, want 8192", got.MaxOutputTokens)
	}
	if !got.SupportsTools {
		t.Errorf("SupportsTools = false, want true")
	}
	if !got.SupportsVision {
		t.Errorf("SupportsVision = false, want true")
	}
	if !got.SupportsCache {
		t.Errorf("SupportsCache = false, want true")
	}
	if !got.SupportsCacheTTL1h {
		t.Errorf("SupportsCacheTTL1h = false, want true")
	}
	if got.SupportsNativeCompact {
		t.Errorf("SupportsNativeCompact = true, want false")
	}
	if !got.SupportsThinking {
		t.Errorf("SupportsThinking = false, want true")
	}
	if len(got.ReasoningEfforts) != 0 {
		t.Errorf("ReasoningEfforts = %v, want empty", got.ReasoningEfforts)
	}
	if len(got.ServiceTiers) != 0 {
		t.Errorf("ServiceTiers = %v, want empty", got.ServiceTiers)
	}
	if len(got.BetaHeaders) != 0 {
		t.Errorf("BetaHeaders = %v, want empty", got.BetaHeaders)
	}
	if got.ContextMode != llm.ContextStandard {
		t.Errorf("ContextMode = %d, want ContextStandard (%d)", got.ContextMode, llm.ContextStandard)
	}
}

func TestUnknownModelReturnsDefaults(t *testing.T) {
	p := newProvider()
	const unknown = "MiniMax-Does-Not-Exist-9999"

	if p.SupportsCacheControl(unknown) {
		t.Errorf("SupportsCacheControl(%q) = true, want false for unknown model", unknown)
	}
	if got := p.ContextWindow(unknown); got != 128000 {
		t.Errorf("ContextWindow(%q) = %d, want 128000 default", unknown, got)
	}
	if got := p.MaxOutputTokens(unknown); got != 8192 {
		t.Errorf("MaxOutputTokens(%q) = %d, want 8192 default", unknown, got)
	}
	if got := p.AvailableReasoningEfforts(unknown); len(got) != 0 {
		t.Errorf("AvailableReasoningEfforts(%q) = %v, want empty", unknown, got)
	}
	if got := p.AvailableServiceTiers(unknown); len(got) != 0 {
		t.Errorf("AvailableServiceTiers(%q) = %v, want empty", unknown, got)
	}
	if got := p.BetaHeaders(unknown); len(got) != 0 {
		t.Errorf("BetaHeaders(%q) = %v, want empty", unknown, got)
	}
	if p.SupportsNativeCompact(unknown) {
		t.Errorf("SupportsNativeCompact(%q) = true, want false default", unknown)
	}

	caps := p.ModelCapabilities(unknown)
	if caps.ID != unknown {
		t.Errorf("ModelCapabilities.ID = %q, want %q", caps.ID, unknown)
	}
	if caps.ContextWindow != 128000 {
		t.Errorf("ModelCapabilities.ContextWindow = %d, want 128000", caps.ContextWindow)
	}
	if caps.MaxOutputTokens != 8192 {
		t.Errorf("ModelCapabilities.MaxOutputTokens = %d, want 8192", caps.MaxOutputTokens)
	}
	if caps.SupportsTools {
		t.Errorf("ModelCapabilities.SupportsTools = true, want false default")
	}
	if caps.SupportsVision {
		t.Errorf("ModelCapabilities.SupportsVision = true, want false default")
	}
	if caps.SupportsCache {
		t.Errorf("ModelCapabilities.SupportsCache = true, want false default")
	}
	if caps.SupportsCacheTTL1h {
		t.Errorf("ModelCapabilities.SupportsCacheTTL1h = true, want false default")
	}
	if caps.SupportsNativeCompact {
		t.Errorf("ModelCapabilities.SupportsNativeCompact = true, want false default")
	}
	if caps.SupportsThinking {
		t.Errorf("ModelCapabilities.SupportsThinking = true, want false default")
	}
	if caps.ContextMode != llm.ContextStandard {
		t.Errorf("ModelCapabilities.ContextMode = %d, want ContextStandard", caps.ContextMode)
	}
}
