package llm_test

import (
	"sync"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-core/compact"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestCacheTracker_RecordUsageAddsToTotals(t *testing.T) {
	ct := compact.NewCacheTracker()
	ct.RecordUsage(llm.EventUsage{CacheReadTokens: 100, CacheCreationTokens: 50})
	ct.RecordUsage(llm.EventUsage{CacheReadTokens: 200, CacheCreationTokens: 25})

	stats := ct.Stats()
	if stats.CacheReadTokens != 300 {
		t.Errorf("CacheReadTokens = %d, want 300", stats.CacheReadTokens)
	}
	if stats.CacheCreationTokens != 75 {
		t.Errorf("CacheCreationTokens = %d, want 75", stats.CacheCreationTokens)
	}
}

func TestCacheTracker_RecordTurnUpdatesLastTurn(t *testing.T) {
	ct := compact.NewCacheTracker()
	ct.RecordUsage(llm.EventUsage{CacheReadTokens: 1000, CacheCreationTokens: 500})
	stats1 := ct.Stats()
	if stats1.LastTurnCacheRead != 1000 {
		t.Errorf("LastTurnCacheRead (after first turn) = %d, want 1000", stats1.LastTurnCacheRead)
	}
	if stats1.LastTurnCacheCreation != 500 {
		t.Errorf("LastTurnCacheCreation (after first turn) = %d, want 500", stats1.LastTurnCacheCreation)
	}

	ct.RecordUsage(llm.EventUsage{CacheReadTokens: 400, CacheCreationTokens: 100})
	stats2 := ct.Stats()
	if stats2.LastTurnCacheRead != 400 {
		t.Errorf("LastTurnCacheRead (after second turn) = %d, want 400", stats2.LastTurnCacheRead)
	}
	if stats2.LastTurnCacheCreation != 100 {
		t.Errorf("LastTurnCacheCreation (after second turn) = %d, want 100", stats2.LastTurnCacheCreation)
	}
	if stats2.CacheReadTokens != 1400 {
		t.Errorf("CacheReadTokens cumulative = %d, want 1400", stats2.CacheReadTokens)
	}
}

func TestCacheStats_CacheHitRatioEdgeCases(t *testing.T) {
	stats := llm.CacheStats{}
	ratio := stats.CacheHitRatio
	if ratio != 0 {
		t.Errorf("zero-value CacheHitRatio = %f, want 0", ratio)
	}

	stats.RecordTurn(0, 0)
	if stats.CacheHitRatio != 0 {
		t.Errorf("CacheHitRatio after (0,0) = %f, want 0 (no division by zero)", stats.CacheHitRatio)
	}

	stats.RecordTurn(800, 200)
	if stats.CacheHitRatio < 0.79 || stats.CacheHitRatio > 0.81 {
		t.Errorf("CacheHitRatio = %f, want ~0.8", stats.CacheHitRatio)
	}

	stats.RecordTurn(1000, 0)
	if stats.CacheHitRatio < 0.99 {
		t.Errorf("CacheHitRatio after all-read turn = %f, want 1.0", stats.CacheHitRatio)
	}
}

func TestCacheTracker_ShouldCompactGivenCacheHighHitRatio(t *testing.T) {
	ct := compact.NewCacheTracker()
	ct.RecordUsage(llm.EventUsage{CacheReadTokens: 950, CacheCreationTokens: 50})

	used := 180000
	window := 200000
	if ct.ShouldCompactGivenCache(used, window) {
		t.Errorf("ShouldCompactGivenCache(%d, %d) = true with high cache hit ratio; want false (cached prefix is free)", used, window)
	}

	ct2 := compact.NewCacheTracker()
	if !ct2.ShouldCompactGivenCache(180001, window) {
		t.Errorf("ShouldCompactGivenCache without any recorded usage = false past 90%% usage; want true")
	}
}

func TestCacheTracker_ConcurrentRecordUsageIsRaceSafe(t *testing.T) {
	ct := compact.NewCacheTracker()
	const goroutines = 32
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				ct.RecordUsage(llm.EventUsage{CacheReadTokens: 10, CacheCreationTokens: 5})
				_ = ct.Stats()
			}
		}()
	}
	wg.Wait()

	stats := ct.Stats()
	want := goroutines * perGoroutine * 10
	if stats.CacheReadTokens != want {
		t.Errorf("CacheReadTokens = %d, want %d (race condition)", stats.CacheReadTokens, want)
	}
	wantCreation := goroutines * perGoroutine * 5
	if stats.CacheCreationTokens != wantCreation {
		t.Errorf("CacheCreationTokens = %d, want %d (race condition)", stats.CacheCreationTokens, wantCreation)
	}
}
