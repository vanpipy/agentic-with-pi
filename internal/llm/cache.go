package llm

type CacheControl struct {
	Type string
	TTL  string
}

func CacheEphemeral() *CacheControl {
	return &CacheControl{Type: "ephemeral"}
}

func CacheEphemeral1h() *CacheControl {
	return &CacheControl{Type: "ephemeral", TTL: "1h"}
}

type CacheStats struct {
	CacheReadTokens       int
	CacheCreationTokens   int
	LastTurnCacheRead     int
	LastTurnCacheCreation int
	CacheHitRatio         float64
}

func (c *CacheStats) RecordTurn(read, creation int) {
	c.CacheReadTokens += read
	c.CacheCreationTokens += creation
	c.LastTurnCacheRead = read
	c.LastTurnCacheCreation = creation
	total := read + creation
	if total > 0 {
		c.CacheHitRatio = float64(read) / float64(total)
	}
}
