package agentcore

import (
	"context"

	"github.com/vanpiyp/awp/internal/llm"
)

const DefaultContextWindow = 128000

func ShouldCompactWithModel(msgs []llm.Message, model llm.Model, settings CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	window := model.MaxContextTokens
	if window <= 0 {
		window = DefaultContextWindow
	}
	used := estimateTotalTokens(msgs)
	return used > window-settings.ReserveTokens
}

func Compact(ctx context.Context, a *Agent) error {
	if a == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	msgs, ok := a.applyCompaction(ctx, a.currentMsgs, nil)
	if !ok {
		return ErrCompactionFailed
	}
	a.currentMsgs = msgs
	return nil
}

var ErrCompactionFailed = compactErr("compaction failed")

type compactErr string

func (e compactErr) Error() string { return string(e) }
