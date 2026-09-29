package providers

import (
	"log/slog"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

type MiniMaxProvider struct {
	APIKey string
}

func NewMiniMaxProvider(apiKey string) *MiniMaxProvider {
	return &MiniMaxProvider{
		APIKey: apiKey,
	}
}

func (p *MiniMaxProvider) Name() string {
	return "minimax"
}

func (p *MiniMaxProvider) BaseURL() string {
	return "https://api.minimaxi.com/anthropic"
}

func (p *MiniMaxProvider) Path() string {
	return "/v1/messages"
}

func (p *MiniMaxProvider) Headers() map[string]string {
	return map[string]string{
		"x-api-key":         p.APIKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}
}

func (p *MiniMaxProvider) Models() []llm.Model {
	return []llm.Model{
		{
			ID:                "MiniMax-M3",
			Name:              "MiniMax M3",
			Vendor:            p.Name(),
			MaxContextTokens:  200000,
			MaxOutputTokens:   4096,
			SupportsTool:      true,
			SupportsVision:    true,
			SupportsStreaming: true,
			SupportsReasoning: true,
		},
	}
}

type modelSpec struct {
	contextWindow         int
	maxOutputTokens       int
	supportsCache         bool
	supportsCacheTTL1h    bool
	supportsNativeCompact bool
	supportsTools         bool
	supportsVision        bool
	supportsThinking      bool
	reasoningEfforts      []string
	serviceTiers          []string
	betaHeaders           []string
	contextMode           llm.AnthropicContextMode
}

var minimaxModelSpecs = map[string]modelSpec{
	"MiniMax-M2": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M2.1": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M2.1-highspeed": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M2.5": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M2.5-highspeed": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M2.7": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M2.7-highspeed": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		contextMode:        llm.ContextStandard,
	},
	"MiniMax-M3": {
		contextWindow:      200000,
		maxOutputTokens:    8192,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
}

func (p *MiniMaxProvider) lookupSpec(model string) (modelSpec, bool) {
	spec, ok := minimaxModelSpecs[model]
	return spec, ok
}

func (p *MiniMaxProvider) SupportsCacheControl(model string) bool {
	spec, ok := p.lookupSpec(model)
	return ok && spec.supportsCache
}

func (p *MiniMaxProvider) ContextWindow(model string) int {
	if spec, ok := p.lookupSpec(model); ok {
		return spec.contextWindow
	}
	return 128000
}

func (p *MiniMaxProvider) MaxOutputTokens(model string) int {
	if spec, ok := p.lookupSpec(model); ok {
		return spec.maxOutputTokens
	}
	return 8192
}

func (p *MiniMaxProvider) AvailableReasoningEfforts(model string) []string {
	return nil
}

func (p *MiniMaxProvider) AvailableServiceTiers(model string) []string {
	return nil
}

func (p *MiniMaxProvider) BetaHeaders(model string) []string {
	if spec, ok := p.lookupSpec(model); ok {
		return spec.betaHeaders
	}
	return nil
}

func (p *MiniMaxProvider) SupportsNativeCompact(model string) bool {
	spec, ok := p.lookupSpec(model)
	return ok && spec.supportsNativeCompact
}

func (p *MiniMaxProvider) ModelCapabilities(model string) llm.ModelCapabilities {
	spec, ok := p.lookupSpec(model)
	if !ok {
		return llm.ModelCapabilities{
			ID:              model,
			ContextWindow:   128000,
			MaxOutputTokens: 8192,
			ContextMode:     llm.ContextStandard,
		}
	}
	return llm.ModelCapabilities{
		ID:                    model,
		ContextWindow:         spec.contextWindow,
		MaxOutputTokens:       spec.maxOutputTokens,
		SupportsTools:         spec.supportsTools,
		SupportsVision:        spec.supportsVision,
		SupportsCache:         spec.supportsCache,
		SupportsCacheTTL1h:    spec.supportsCacheTTL1h,
		SupportsNativeCompact: spec.supportsNativeCompact,
		SupportsThinking:      spec.supportsThinking,
		ReasoningEfforts:      spec.reasoningEfforts,
		ServiceTiers:          spec.serviceTiers,
		BetaHeaders:           spec.betaHeaders,
		ContextMode:           spec.contextMode,
	}
}

func (p *MiniMaxProvider) ConvertRequest(req *llm.ChatRequest) ([]byte, error) {
	if len(req.Messages) == 0 {
		req.Messages = []llm.Message{{Role: "user", Content: ""}}
	}

	ar, err := anthropic.MapToAnthropicRequest(*req)
	if err != nil {
		slog.Debug("provider: map request failed", "model", req.Model, "err", err)
		return nil, err
	}

	if req.MaxTokens == 0 {
		ar.MaxTokens = 4096
	}

	for _, m := range p.Models() {
		if m.ID == req.Model && m.SupportsReasoning {
			budget := ar.MaxTokens / 2
			if budget > 8192 {
				budget = 8192
			}
			if budget < 1 {
				budget = 1024
			}
			ar.Thinking = &anthropic.AnthropicThinking{Type: "enabled", BudgetTokens: budget}
			break
		}
	}

	body, err := anthropic.BuildAnthropicRequest(ar)
	if err != nil {
		slog.Debug("provider: marshal request failed", "model", req.Model, "err", err)
		return nil, err
	}
	slog.Debug("provider: marshal request ok", "model", req.Model, "bytes", len(body), "messages", len(ar.Messages))
	return body, nil
}

func (p *MiniMaxProvider) ConvertResponse(data []byte) (*llm.StreamChunk, bool, error) {
	return anthropic.ConvertAnthropicEvent(data)
}

// RecoverRequest is a no-op. MiniMax uses the legacy
// `thinking:{type:enabled,budget_tokens:...}` envelope with no
// `output_config` block, and MiniMax errors are surfaced to the
// caller verbatim — there is no transition-window recovery surface
// to apply here.
func (p *MiniMaxProvider) RecoverRequest(_ *llm.ChatRequest, _ error) bool {
	return false
}

func (p *MiniMaxProvider) CompleteSplit(systemPrompt string) ([]llm.ContentBlock, error) {
	return anthropic.CompleteAnthropicSystemSplit(systemPrompt)
}
