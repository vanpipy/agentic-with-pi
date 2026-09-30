// Package cache wires Anthropic-style prompt-cache breakpoints into
// the message list before each agent turn. The helper is the only
// place in agent-core that mutates Message.CacheControl, so the wire
// layer (protocol/anthropic) and the cache stat collector
// (compact.CacheTracker) stay in lock-step with the producer side.
package cache

import "github.com/vanpiyp/awp/internal/llm"

// SupportsCacheControl is the provider-side capability probe that gates
// cache-breakpoint insertion. The contract matches
// llm.Provider.SupportsCacheControl; declared as a function-variable so
// the helper can be unit-tested without an llm.Provider instance.
type SupportsCacheControl func(model string) bool

// Ephemeral is the cache_control payload sent on every breakpoint.
// Anthropic-compatible endpoints accept at most 4 breakpoints per
// request and the type is always "ephemeral"; the TTL variant is not
// used by the agent loop today. Keep the constant local so callers do
// not need to import llm just to reference a marker.
var Ephemeral = func() *llm.CacheControl { return llm.CacheEphemeral() }

// InjectCacheBreakpoints returns a copy of msgs with prompt-cache
// markers applied. Marking is a copy because msgs are shared across
// the strategy, the event sink, and the persistence layer —
// mutating in place would surface the marker to readers that should
// see the agent's mental state, not the wire-level annotation.
//
// Breakpoints are placed on:
//
//   - The first system message (caches the prompt prefix for the
//     whole session).
//   - The last message in the list, regardless of role (caches the
//     running transcript up to and including the last turn).
//
// Two breakpoints stay within Anthropic's 4-breakpoint ceiling while
// giving the agent loop maximum reuse: the system prompt is shared
// across all turns, and the running transcript prefix is reused as
// long as the conversation does not branch.
//
// If SupportsCacheControl returns false for the model, msgs is returned
// unchanged.
func InjectCacheControl(msgs []llm.Message, supports SupportsCacheControl, model string) []llm.Message {
	if !supports(model) {
		return msgs
	}
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	if out[0].Role == "system" {
		out[0].CacheControl = Ephemeral()
	}
	out[len(out)-1].CacheControl = Ephemeral()
	return out
}
