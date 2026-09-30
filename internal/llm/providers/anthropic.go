package providers

import (
	"context"
	"log/slog"
	"strings"

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
	// Unknown models default to the Anthropic standard 200K. The
	// classifier (Mode) is the source of truth for capability, but
	// the spec table is the source of truth for the actual window
	// size — when no entry matches, follow jcode's
	// DEFAULT_CONTEXT_LIMIT fallback at
	// crates/jcode-provider-anthropic-runtime/src/context_window.rs:11.
	return 200000
}

func (p *AnthropicProvider) MaxOutputTokens(model string) int {
	// The per-model spec table carries the canonical physical
	// budget. The classifier (anthropic.MaxOutputTokens) is the
	// fallback for unknown generations and matches the LARGE_OUTPUT
	// / haiku-64K / legacy-32K split jcode uses at
	// crates/jcode-provider-core/src/anthropic.rs:161-204.
	if spec, ok := p.lookupSpec(model); ok {
		return spec.maxOutputTokens
	}
	return anthropic.MaxOutputTokens(model)
}

func (p *AnthropicProvider) AvailableReasoningEfforts(model string) []string {
	// Single source of truth: the classifier at
	// anthropic_caps.AvailableReasoningEfforts walks the model
	// family/version and returns the ladder filtered by caps. The
	// spec table is no longer consulted for the effort ladder.
	return anthropic.AvailableReasoningEfforts(model)
}

func (p *AnthropicProvider) AvailableServiceTiers(model string) []string {
	return nil
}

func (p *AnthropicProvider) BetaHeaders(model string) []string {
	// Single source of truth: the classifier at
	// anthropic_caps.BetaHeaders emits the context-1m beta only
	// when the explicit `[1m]` suffix is present (jcode's
	// anthropic_is_1m_model at anthropic.rs:109-113). Native1M
	// models (opus-4-7+, sonnet-5) and standard 200K models do
	// not need the beta.
	return anthropic.BetaHeaders(model)
}

func (p *AnthropicProvider) NativeCompactMode(model string) string {
	return ""
}

func (p *AnthropicProvider) NativeCompactThreshold(model string) int {
	return 0
}

func (p *AnthropicProvider) NativeCompactCapabilities(model string) llm.NativeCompactionCapabilities {
	return llm.NativeCompactionCapabilities{Kind: llm.KindClient}
}

func (p *AnthropicProvider) NativeCompact(ctx context.Context, model string, msgs []llm.Message, summaryText, encryptedContent string) (llm.NativeCompactionResult, error) {
	// Anthropic has no server-side compaction endpoint; the decision
	// tree in NativeCompactor routes KindClient through the fallback
	// CompactRunner and never reaches this method. Return the
	// unsupported sentinel so any direct caller fails loudly.
	return llm.NativeCompactionResult{}, llm.ErrNativeCompactionUnsupported
}

func (p *AnthropicProvider) ModelCapabilities(model string) llm.ModelCapabilities {
	spec, ok := p.lookupSpec(model)
	if !ok {
		return llm.ModelCapabilities{
			ID:               model,
			ContextWindow:    200000,
			MaxOutputTokens:  anthropic.MaxOutputTokens(model),
			ContextMode:      anthropic.ContextMode(model),
			ReasoningEfforts: anthropic.AvailableReasoningEfforts(model),
			BetaHeaders:      anthropic.BetaHeaders(model),
		}
	}
	caps := anthropic.ReasoningCaps(model)
	return llm.ModelCapabilities{
		ID:                 model,
		ContextWindow:      spec.contextWindow,
		MaxOutputTokens:    spec.maxOutputTokens,
		SupportsTools:      spec.supportsTools,
		SupportsVision:     spec.supportsVision,
		SupportsCache:      spec.supportsCache,
		SupportsCacheTTL1h: spec.supportsCacheTTL1h,
		SupportsThinking:   spec.supportsThinking,
		ReasoningEfforts:   anthropic.AvailableReasoningEfforts(model),
		ServiceTiers:       spec.serviceTiers,
		BetaHeaders:        anthropic.BetaHeaders(model),
		ContextMode:        spec.contextMode,
		OutputEffort:       caps.OutputEffort,
		AdaptiveThinking:   caps.AdaptiveThinking,
		ManualThinking:     caps.ManualThinking,
	}
}

// DefaultReasoningEffort returns the per-model default reasoning
// effort the runtime should send when the caller leaves
// ChatRequest.ReasoningEffort empty. Mirrors jcode's
// default_reasoning_effort_for_model at
// crates/jcode-provider-anthropic-runtime/src/lib.rs:788-807. Empty
// string means "no forced default" — callers that don't opt in keep
// the model's own reasoning-strength default.
func (p *AnthropicProvider) DefaultReasoningEffort(model string) string {
	key := strings.ToLower(model)
	switch {
	case strings.Contains(key, "claude-opus-5-5"):
		return "medium"
	case strings.Contains(key, "claude-opus-5"):
		return "low"
	case strings.Contains(key, "claude-opus"):
		if anthropic.ReasoningCaps(model).XHighEffort {
			return "xhigh"
		}
		return "high"
	case strings.Contains(key, "claude-fable-5"):
		return "high"
	}
	return ""
}

// ConvertRequest emits the modern Anthropic Messages wire payload.
// For models that support adaptive thinking
// (caps.AdaptiveThinking) it emits `thinking: {type: adaptive}` and
// `output_config: {effort}` when the caller requested a non-"none"
// effort. For manual-thinking generations (caps.ManualThinking) it
// falls back to the legacy `thinking: {type: enabled, budget_tokens}`
// envelope. Models that support no reasoning effort at all
// (caps.SupportsReasoningEffort() == false) emit no thinking block.
//
// The `[1m]` suffix is stripped from the wire `model` field —
// Anthropic's API expects the bare id and infers 1M-mode from the
// `anthropic-beta: context-1m-2025-08-07` header that the core
// adds via headersFor. Mirrors jcode's RetrySettings::reshape at
// crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:65-89.
func (p *AnthropicProvider) ConvertRequest(req *llm.ChatRequest) ([]byte, error) {
	if len(req.Messages) == 0 {
		req.Messages = []llm.Message{{Role: "user", Content: ""}}
	}

	ar, err := anthropic.MapToAnthropicRequest(*req)
	if err != nil {
		slog.Debug("anthropic: map request failed", "model", req.Model, "err", err)
		return nil, err
	}

	// Strip the [1m] suffix from the wire model; the beta header
	// carries the opt-in.
	ar.Model = anthropic.Strip1mSuffix(ar.Model)

	if req.MaxTokens == 0 {
		ar.MaxTokens = p.MaxOutputTokens(req.Model)
	}

	caps := anthropic.ReasoningCaps(req.Model)
	effort := req.ReasoningEffort
	if effort == "" {
		effort = p.DefaultReasoningEffort(req.Model)
	}

	// Self-heal retry flags suppress the offending reasoning fields
	// on the second attempt. RetryDropThinking drops both blocks;
	// RetryDropOutputConfig drops only the output_config block
	// (keeps the always-on thinking envelope for opus-5-5 /
	// fable-5-1).
	switch {
	case req.RetryDropThinking:
		ar.Thinking = nil
		ar.OutputConfig = nil
	case req.RetryDropOutputConfig:
		ar.OutputConfig = nil
		if caps.AdaptiveThinking {
			ar.Thinking = &anthropic.AnthropicThinking{Type: "adaptive"}
		}
	default:
		if caps.AdaptiveThinking {
			// Always emit adaptive thinking for supported generations
			// (the model picks its own budget; we do not pass one).
			// output_config.effort is sent when the caller asked for
			// one or the model default is non-empty. The "none" effort
			// value disables output_config (Claude interprets it as
			// "no effort override") but keeps the adaptive envelope.
			ar.Thinking = &anthropic.AnthropicThinking{Type: "adaptive"}
			if effort != "" && effort != "none" && caps.OutputEffort {
				ar.OutputConfig = &anthropic.AnthropicOutputConfig{Effort: effort}
			}
		} else if caps.ManualThinking && caps.SupportsReasoningEffort() {
			budgetTokens := manualThinkingBudget(effort, ar.MaxTokens)
			if budgetTokens > 0 {
				ar.Thinking = &anthropic.AnthropicThinking{
					Type:         "enabled",
					BudgetTokens: budgetTokens,
				}
			}
			if caps.OutputEffort && effort != "" && effort != "none" {
				ar.OutputConfig = &anthropic.AnthropicOutputConfig{Effort: effort}
			}
		}
	}

	// Extended/adaptive thinking is incompatible with temperature;
	// the API rejects both. Drop temperature when thinking is on so
	// the wire shape stays valid.
	if ar.Thinking != nil {
		ar.Temperature = nil
	}

	body, err := anthropic.BuildAnthropicRequest(ar)
	if err != nil {
		slog.Debug("anthropic: marshal request failed", "model", req.Model, "err", err)
		return nil, err
	}
	slog.Debug("anthropic: marshal request ok", "model", req.Model, "bytes", len(body), "messages", len(ar.Messages), "thinking", ar.Thinking != nil, "output_config", ar.OutputConfig != nil)
	return body, nil
}

// RecoverRequest inspects `err` from a failed first attempt. When
// Anthropic rejected a reasoning-related field (the API returned a
// 400 with `invalid_request_error` + `thinking`/`effort`/
// `output_config` + `not supported`/`does not support`), it sets
// one of the ChatRequest retry flags so the next ConvertRequest
// emits a compatible wire shape.
//
// Always-on-thinking generations (opus-5-5 / fable-5-1) drop only
// `output_config` and keep `thinking: {type: adaptive}`; everything
// else drops both blocks and lets the model run with the
// caller-supplied system + tools alone. Mirrors jcode's
// recover_rejected_reasoning at
// crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:95-115.
func (p *AnthropicProvider) RecoverRequest(req *llm.ChatRequest, err error) bool {
	if !isReasoningUnsupportedError(err) {
		return false
	}
	if anthropic.ThinkingAlwaysOn(req.Model) {
		req.RetryDropOutputConfig = true
	} else {
		req.RetryDropThinking = true
	}
	return true
}

// isReasoningUnsupportedError matches the Anthropic 400-error shape
// jcode classifies as recoverable at
// crates/jcode-provider-anthropic-runtime/src/lib.rs:2296-2305:
// HTTP 400 (or 4xx with `invalid_request_error`) + body mentions
// `thinking` / `effort` / `output_config` + body says
// `not supported` / `does not support`. Other 400s (auth, bad
// schema, etc.) are surfaced unchanged.
func isReasoningUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "invalid_request_error") && !strings.Contains(msg, "400") {
		return false
	}
	mentionsField := strings.Contains(msg, "thinking") ||
		strings.Contains(msg, "effort") ||
		strings.Contains(msg, "output_config")
	mentionsUnsupported := strings.Contains(msg, "not supported") ||
		strings.Contains(msg, "does not support")
	return mentionsField && mentionsUnsupported
}

// manualThinkingBudget maps a reasoning-effort string to a
// budget_tokens value for manual-thinking models. Mirrors jcode's
// manual_thinking_budget at
// crates/jcode-provider-anthropic-runtime/src/lib.rs:868-879.
func manualThinkingBudget(effort string, maxTokens int) int {
	desired := 0
	switch effort {
	case "minimal", "low":
		desired = 1024
	case "medium":
		desired = 4096
	case "high":
		desired = 8192
	case "xhigh", "max":
		desired = 16384
	default:
		return 0
	}
	if maxTokens > 0 && desired >= maxTokens {
		desired = maxTokens - 1
		if desired < 1024 {
			return 0
		}
	}
	return desired
}

func (p *AnthropicProvider) ConvertResponse(data []byte) (*llm.StreamChunk, bool, error) {
	return anthropic.ConvertAnthropicEvent(data)
}
