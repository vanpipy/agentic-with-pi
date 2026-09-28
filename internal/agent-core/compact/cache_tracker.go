package compact

import (
	"sync"

	"github.com/vanpiyp/awp/internal/llm"
)

type CacheTracker struct {
	mu    sync.Mutex
	stats llm.CacheStats
}

func NewCacheTracker() *CacheTracker {
	return &CacheTracker{}
}

func (c *CacheTracker) RecordUsage(usage llm.EventUsage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.RecordTurn(usage.CacheReadTokens, usage.CacheCreationTokens)
}

func (c *CacheTracker) Stats() llm.CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *CacheTracker) ShouldCompactGivenCache(used, window int) bool {
	if window <= 0 {
		return false
	}
	effective := used
	stats := c.Stats()
	if stats.CacheHitRatio > 0.8 && stats.LastTurnCacheRead > 0 {
		effective = used - stats.LastTurnCacheRead
		if effective < 0 {
			effective = 0
		}
	}
	return effective > int(float64(window)*0.9)
}
