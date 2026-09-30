package compact

import (
	"context"
	"log/slog"

	"github.com/vanpiyp/awp/internal/llm"
)

// NativeCompactor routes compaction decisions through the provider's
// declared NativeCompactionCapabilities. It is the entry point used by
// the agent loop when the chat is about to overflow the context window.
//
// Decision tree (caps.Kind):
//
//	KindNative     -> call Provider.NativeCompact directly. On provider
//	                  error, fall through to the fallback runner so a
//	                  transient native-compaction failure never blocks
//	                  the chat.
//	KindClient     -> skip the provider entirely and route to fallback.
//	                  Anthropic uses this today; the chat continues
//	                  even when no server-side compaction exists.
//	KindUnsupported -> route to fallback. Returning the unsupported
//	                   sentinel from NativeCompact would be a
//	                   programmer error here, since the decision tree
//	                   guards the call.
type NativeCompactor struct {
	Provider llm.Provider
	Model    string
	Fallback CompactRunner
	Logger   *slog.Logger
}

func (n *NativeCompactor) Compact(ctx context.Context, msgs []llm.Message) ([]llm.Message, bool, error) {
	if n == nil || n.Provider == nil {
		return msgs, false, nil
	}
	caps := n.Provider.NativeCompactCapabilities(n.Model)
	switch caps.Kind {
	case llm.KindNative:
		result, err := n.Provider.NativeCompact(ctx, n.Model, msgs, "", "")
		if err != nil {
			n.logWarn("native compaction failed, falling back to client-side", err)
			return n.applyFallback(ctx, msgs)
		}
		summary := llm.Message{
			Role:    string(llm.RoleSystem),
			Content: result.Summary,
		}
		return []llm.Message{summary}, true, nil
	case llm.KindClient, llm.KindUnsupported:
		return n.applyFallback(ctx, msgs)
	}
	return msgs, false, nil
}

func (n *NativeCompactor) applyFallback(ctx context.Context, msgs []llm.Message) ([]llm.Message, bool, error) {
	if n.Fallback == nil {
		return msgs, false, nil
	}
	out, ok := n.Fallback.Apply(ctx, msgs)
	return out, ok, nil
}

func (n *NativeCompactor) logWarn(msg string, err error) {
	if n.Logger == nil {
		return
	}
	n.Logger.Warn(msg, "err", err, "model", n.Model)
}
