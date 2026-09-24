package agentcore

import (
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
