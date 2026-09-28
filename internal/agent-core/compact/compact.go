package compact

import (
	"context"

	"github.com/vanpiyp/awp/internal/llm"
)

type CompactionSettings struct {
	Enabled          bool
	ReserveTokens    int
	KeepRecentTurns  int
	MaxContextTokens int
	Proactive        bool
	Semantic         bool
}

const DefaultContextWindow = 128000

func ShouldCompactWithModel(msgs []llm.Message, model llm.Model, settings CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	window := model.MaxContextTokens
	if window <= 0 {
		window = DefaultContextWindow
	}
	used := EstimateTotalTokens(msgs)
	return used > window-settings.ReserveTokens
}

type CompactRunner interface {
	Apply(ctx context.Context, msgs []llm.Message) ([]llm.Message, bool)
	SetMessages(msgs []llm.Message)
}

func Compact(ctx context.Context, a CompactRunner) error {
	if a == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	msgs, ok := a.Apply(ctx, nil)
	if !ok {
		return ErrCompactionFailed
	}
	a.SetMessages(msgs)
	return nil
}

var ErrCompactionFailed = compactErr("compaction failed")

type compactErr string

func (e compactErr) Error() string { return string(e) }
