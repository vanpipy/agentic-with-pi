package agentcore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

type ToolResultCache struct {
	mu       sync.Mutex
	capacity int
	entries  []toolCacheEntry
	index    map[string]int
}

type toolCacheEntry struct {
	sig    string
	result string
}

func NewToolResultCache(capacity int) *ToolResultCache {
	if capacity <= 0 {
		capacity = 20
	}
	return &ToolResultCache{
		capacity: capacity,
		entries:  make([]toolCacheEntry, 0, capacity),
		index:    make(map[string]int, capacity),
	}
}

func (c *ToolResultCache) Get(sig string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	idx, ok := c.index[sig]
	if !ok {
		return "", false
	}
	return c.entries[idx].result, true
}

func (c *ToolResultCache) Put(sig, result string) {
	if c == nil || sig == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if idx, ok := c.index[sig]; ok {
		c.entries[idx].result = result
		return
	}
	for len(c.entries) >= c.capacity {
		evict := c.entries[0]
		c.entries = c.entries[1:]
		delete(c.index, evict.sig)
		for k, v := range c.index {
			c.index[k] = v - 1
		}
	}
	c.entries = append(c.entries, toolCacheEntry{sig: sig, result: result})
	c.index[sig] = len(c.entries) - 1
}

func (c *ToolResultCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *ToolResultCache) Capacity() int {
	if c == nil {
		return 0
	}
	return c.capacity
}

func toolCallDedupKey(name, argsJSON string) string {
	h := sha256.New()
	h.Write([]byte(name))
	h.Write([]byte{0})
	var parsed any
	if err := json.Unmarshal([]byte(argsJSON), &parsed); err == nil {
		if canonical, err := json.Marshal(parsed); err == nil {
			h.Write(canonical)
			return hex.EncodeToString(h.Sum(nil))
		}
	}
	h.Write([]byte(argsJSON))
	return hex.EncodeToString(h.Sum(nil))
}

func ToolCallDedupKeyForTest(name, argsJSON string) string {
	return toolCallDedupKey(name, argsJSON)
}
