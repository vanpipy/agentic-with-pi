package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol/anthropic"
)

// TestAnthropicSSE_SurfacesCacheTokens drives the wire-format parser
// against an httptest server emitting cache_read_input_tokens +
// cache_creation_input_tokens in message_start and message_delta. The
// parser must surface those counts in llm.EventUsage so the producer
// (Core) can record them in compact.CacheTracker for ShouldCompactGivenCache
// decisions.
func TestAnthropicSSE_SurfacesCacheTokens(t *testing.T) {
	const sseBody = `event: message_start
data: {"type":"message_start","message":{"id":"msg_x","usage":{"input_tokens":300,"output_tokens":0,"cache_read_input_tokens":280,"cache_creation_input_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":300,"output_tokens":1,"cache_read_input_tokens":280,"cache_creation_input_tokens":0}}

event: message_stop
data: {"type":"message_stop"}

`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	ch, err := anthropic.ParseAnthropicSSE(ctx, resp.Body)
	if err != nil {
		t.Fatalf("ParseAnthropicSSE: %v", err)
	}

	var usage *llm.EventUsage
	for ev := range ch {
		if u, ok := ev.(llm.EventUsage); ok {
			u := u
			usage = &u
		}
	}
	if usage == nil {
		t.Fatalf("no EventUsage event observed in stream")
	}
	if usage.InputTokens != 300 {
		t.Errorf("InputTokens = %d, want 300", usage.InputTokens)
	}
	if usage.OutputTokens != 1 {
		t.Errorf("OutputTokens = %d, want 1", usage.OutputTokens)
	}
	if usage.CacheReadTokens != 280 {
		t.Errorf("CacheReadTokens = %d, want 280", usage.CacheReadTokens)
	}
	if usage.CacheCreationTokens != 0 {
		t.Errorf("CacheCreationTokens = %d, want 0", usage.CacheCreationTokens)
	}
}

// TestCacheTracker_RecordsAndGuidesCompaction drives RecordUsage across
// two turns and verifies that ShouldCompactGivenCache exempts the
// reported read tokens from the compaction threshold. With 200K context,
// 199K used, and 199K cached on the last turn, the threshold would
// otherwise fire, but the cache hit ratio (1.0) + last-turn cache read
// (199K) subtracts the cached prefix and returns false (no compact yet).
func TestCacheTracker_RecordsAndGuidesCompaction(t *testing.T) {
	tracker := compact.NewCacheTracker()

	// Turn 1: cache miss — 1000 cache creation, 0 read.
	tracker.RecordUsage(llm.EventUsage{
		InputTokens:         1000,
		OutputTokens:        50,
		CacheReadTokens:     0,
		CacheCreationTokens: 1000,
	})

	// Turn 2: full cache hit — 1000 cache read, 0 creation.
	tracker.RecordUsage(llm.EventUsage{
		InputTokens:         1000,
		OutputTokens:        80,
		CacheReadTokens:     1000,
		CacheCreationTokens: 0,
	})

	stats := tracker.Stats()
	if stats.CacheReadTokens != 1000 {
		t.Errorf("CacheReadTokens = %d, want 1000", stats.CacheReadTokens)
	}
	if stats.CacheCreationTokens != 1000 {
		t.Errorf("CacheCreationTokens = %d, want 1000", stats.CacheCreationTokens)
	}
	if stats.CacheHitRatio < 0.99 {
		t.Errorf("CacheHitRatio = %f, want ~1.0", stats.CacheHitRatio)
	}

	// used=1000, window=1000 — would compact if hit ratio were low; with hit ratio 1.0
	// and LastTurnCacheRead=1000, the effective used is 0 and ShouldCompactGivenCache
	// returns false.
	if tracker.ShouldCompactGivenCache(1000, 1000) {
		t.Error("ShouldCompactGivenCache(1000, 1000) = true, want false (full cache hit)")
	}

	// used=1000, window=1100 — still under the 90% threshold even after the subtract,
	// because effective=0. Compaction does not fire while cache is warm.
	if tracker.ShouldCompactGivenCache(1000, 1100) {
		t.Error("ShouldCompactGivenCache(1000, 1100) = true, want false (effective used = 0)")
	}

	// Without cache, the same used/window would compact: we cannot reproduce that
	// path in this tracker because RecordUsage leaves stats cached. Skip it.
	_ = stats
}

// TestCacheTracker_LowHitRatioDoesNotExempt is the inverse: with low hit
// ratio and small LastTurnCacheRead, the subtract does not move the needle.
func TestCacheTracker_LowHitRatioDoesNotExempt(t *testing.T) {
	tracker := compact.NewCacheTracker()
	tracker.RecordUsage(llm.EventUsage{
		InputTokens:         1900,
		OutputTokens:        100,
		CacheReadTokens:     100,
		CacheCreationTokens: 1900,
	})
	tracker.RecordUsage(llm.EventUsage{
		InputTokens:         1900,
		OutputTokens:        100,
		CacheReadTokens:     100,
		CacheCreationTokens: 1900,
	})
	if !tracker.ShouldCompactGivenCache(1900, 2000) {
		t.Error("low hit ratio with 1900/2000 used should compact")
	}
}

var _ = strings.TrimSpace
