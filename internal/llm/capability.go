package llm

import "time"

type AnthropicContextMode int

const (
	ContextStandard AnthropicContextMode = iota
	ContextOptIn1M
	ContextNative1M
)

type ModelCapabilities struct {
	ID                    string
	ContextWindow         int
	MaxOutputTokens       int
	SupportsTools         bool
	SupportsVision        bool
	SupportsCache         bool
	SupportsCacheTTL1h    bool
	SupportsNativeCompact bool
	SupportsThinking      bool
	ReasoningEfforts      []string
	ServiceTiers          []string
	BetaHeaders           []string
	ContextMode           AnthropicContextMode
}

type RouteSelection struct {
	Model     string
	Provider  string
	APIMethod string
	Detail    string
}

type NativeCompactionResult struct {
	Summary          string
	EncryptedContent string
	PreTokens        int
	PostTokens       int
}

type Summary struct {
	Text      string
	CreatedAt time.Time
	Source    string
}
