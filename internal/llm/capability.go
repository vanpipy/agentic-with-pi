package llm

import (
	"errors"
	"time"
)

type AnthropicContextMode int

const (
	ContextStandard AnthropicContextMode = iota
	ContextOptIn1M
	ContextNative1M
)

type ModelCapabilities struct {
	ID                 string
	ContextWindow      int
	MaxOutputTokens    int
	SupportsTools      bool
	SupportsVision     bool
	SupportsCache      bool
	SupportsCacheTTL1h bool
	SupportsThinking   bool
	ReasoningEfforts   []string
	ServiceTiers       []string
	BetaHeaders        []string
	ContextMode        AnthropicContextMode
	OutputEffort       bool
	AdaptiveThinking   bool
	ManualThinking     bool
}

type RouteSelection struct {
	Model     string
	Provider  string
	APIMethod string
	Detail    string
}

// NativeCompactionKind reports how a provider wants the runtime to handle
// context compaction when the chat is about to overflow.
type NativeCompactionKind int

const (
	// KindUnsupported: the provider has no native compaction surface and
	// the client must fall back to summarization.
	KindUnsupported NativeCompactionKind = iota
	// KindNative: the provider offers a server-side compaction API; the
	// runtime may call Provider.NativeCompact directly.
	KindNative
	// KindClient: the provider explicitly opts into client-side
	// compaction (e.g. Anthropic, which has no server-side compact
	// endpoint); the runtime skips NativeCompact and routes to the
	// fallback CompactRunner.
	KindClient
)

// NativeCompactionCapabilities reports how a provider wants the runtime
// to handle compaction for a given model. Mode and Threshold are
// optional hints (mode = "auto" / threshold tokens); the Kind is the
// dispatching decision.
type NativeCompactionCapabilities struct {
	Kind      NativeCompactionKind
	Mode      string
	Threshold int
}

type NativeCompactionResult struct {
	Summary          string
	EncryptedContent string
	PreTokens        int
	PostTokens       int
}

// ErrNativeCompactionUnsupported is returned by Provider.NativeCompact
// when the provider does not implement server-side native compaction.
// Callers should treat this as a routing signal, not a fatal error,
// and fall back to client-side summarization.
var ErrNativeCompactionUnsupported = errors.New("native compaction unsupported by provider")

type Summary struct {
	Text      string
	CreatedAt time.Time
	Source    string
}
