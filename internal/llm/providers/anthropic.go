package providers

import (
	"log/slog"

	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

type AnthropicProvider struct {
	APIKey string
}

func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey: apiKey,
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) BaseURL() string {
	return "https://api.anthropic.com"
}

func (p *AnthropicProvider) Path() string {
	return "/v1/messages"
}

func (p *AnthropicProvider) Headers() map[string]string {
	return map[string]string{
		"x-api-key":         p.APIKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}
}

func (p *AnthropicProvider) Models() []llm.Model {
	out := make([]llm.Model, 0, len(anthropicModelSpecs))
	for id := range anthropicModelSpecs {
		spec := anthropicModelSpecs[id]
		out = append(out, llm.Model{
			ID:                id,
			Name:              id,
			Vendor:            p.Name(),
			MaxContextTokens:  spec.contextWindow,
			MaxOutputTokens:   spec.maxOutputTokens,
			SupportsTool:      spec.supportsTools,
			SupportsVision:    spec.supportsVision,
			SupportsStreaming: true,
			SupportsReasoning: spec.supportsThinking,
		})
	}
	return out
}

var anthropicModelSpecs = map[string]modelSpec{
	"claude-opus-4-6": {
		contextWindow:      200000,
		maxOutputTokens:    128000,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
	"claude-opus-4-6[1m]": {
		contextWindow:      1000000,
		maxOutputTokens:    128000,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		betaHeaders:        []string{"context-1m-2025-08-07"},
		contextMode:        llm.ContextOptIn1M,
	},
	"claude-sonnet-4-6": {
		contextWindow:      200000,
		maxOutputTokens:    64000,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   true,
		contextMode:        llm.ContextStandard,
	},
	"claude-haiku-4-5": {
		contextWindow:      200000,
		maxOutputTokens:    64000,
		supportsCache:      true,
		supportsCacheTTL1h: true,
		supportsTools:      true,
		supportsVision:     true,
		supportsThinking:   false,
		contextMode:        llm.ContextStandard,
	},
}

func (p *AnthropicProvider) lookupSpec(model string) (modelSpec, bool) {
	spec, ok := anthropicModelSpecs[model]
	return spec, ok
}

func (p *AnthropicProvider) SupportsCacheControl(model string) bool {
	spec, ok := p.lookupSpec(model)
	return ok && spec.supportsCache
}

func (p *AnthropicProvider) ContextWindow(model string) int {
	if spec, ok := p.lookupSpec(model); ok {
		return spec.contextWindow
	}
	return 200000
}

func (p *AnthropicProvider) MaxOutputTokens(model string) int {
	if spec, ok := p.lookupSpec(model); ok {
		return spec.maxOutputTokens
	}
	return 8192
}

func (p *AnthropicProvider) AvailableReasoningEfforts(model string) []string {
	return nil
}

func (p *AnthropicProvider) AvailableServiceTiers(model string) []string {
	return nil
}

func (p *AnthropicProvider) BetaHeaders(model string) []string {
	if spec, ok := p.lookupSpec(model); ok {
		return spec.betaHeaders
	}
	return nil
}

func (p *AnthropicProvider) SupportsNativeCompact(model string) bool {
	spec, ok := p.lookupSpec(model)
	return ok && spec.supportsNativeCompact
}

func (p *AnthropicProvider) ModelCapabilities(model string) llm.ModelCapabilities {
	spec, ok := p.lookupSpec(model)
	if !ok {
		return llm.ModelCapabilities{
			ID:              model,
			ContextWindow:   200000,
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

func (p *AnthropicProvider) ConvertRequest(req *llm.ChatRequest) ([]byte, error) {
	if len(req.Messages) == 0 {
		req.Messages = []llm.Message{{Role: "user", Content: ""}}
	}

	ar, err := anthropic.MapToAnthropicRequest(*req)
	if err != nil {
		slog.Debug("anthropic: map request failed", "model", req.Model, "err", err)
		return nil, err
	}

	if req.MaxTokens == 0 {
		ar.MaxTokens = 4096
	}

	if spec, ok := p.lookupSpec(req.Model); ok && spec.supportsThinking {
		budget := ar.MaxTokens / 2
		if budget > 16000 {
			budget = 16000
		}
		if budget < 1024 {
			budget = 1024
		}
		ar.Thinking = &anthropic.AnthropicThinking{Type: "enabled", BudgetTokens: budget}
	}

	body, err := anthropic.BuildAnthropicRequest(ar)
	if err != nil {
		slog.Debug("anthropic: marshal request failed", "model", req.Model, "err", err)
		return nil, err
	}
	slog.Debug("anthropic: marshal request ok", "model", req.Model, "bytes", len(body), "messages", len(ar.Messages))
	return body, nil
}

func (p *AnthropicProvider) ConvertResponse(data []byte) (*llm.StreamChunk, bool, error) {
	return anthropic.ConvertAnthropicEvent(data)
}

func (p *AnthropicProvider) CompleteSplit(systemPrompt string) ([]llm.ContentBlock, error) {
	return anthropic.CompleteAnthropicSystemSplit(systemPrompt)
}
