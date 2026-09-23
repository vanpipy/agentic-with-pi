package json_rpc

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUUIDv7Format(t *testing.T) {
	id := NewV7()
	if len(id) != 36 {
		t.Fatalf("expected length 36, got %d (id=%q)", len(id), id)
	}
	want := []int{8, 4, 4, 4, 12}
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 hyphen-separated groups, got %d (id=%q)", len(parts), id)
	}
	for i, p := range parts {
		if len(p) != want[i] {
			t.Fatalf("group %d expected length %d, got %d (id=%q)", i, want[i], len(p), id)
		}
		for j, r := range p {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				t.Fatalf("group %d char %d not lowercase hex (id=%q)", i, j, id)
			}
		}
	}
}

func TestUUIDv7VersionAndVariant(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := NewV7()
		b, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
		if err != nil {
			t.Fatalf("decode %q: %v", id, err)
		}
		ver := (b[6] >> 4) & 0x0f
		if ver != 0x07 {
			t.Fatalf("expected version nibble 0x07 in byte 6 high 4 bits, got 0x%x (id=%q)", ver, id)
		}
		varHi := (b[8] >> 6) & 0x03
		if varHi != 0x02 {
			t.Fatalf("expected variant bits 0b10 in byte 8 high 2 bits, got 0b%02b (id=%q)", varHi, id)
		}
	}
}

func TestUUIDv7TimestampRoundtrips(t *testing.T) {
	before := time.Now().UnixMilli()
	id := NewV7()
	after := time.Now().UnixMilli()

	b, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	if err != nil {
		t.Fatalf("decode %q: %v", id, err)
	}
	ts := int64(uint64(b[0])<<40) |
		int64(uint64(b[1])<<32) |
		int64(uint64(b[2])<<24) |
		int64(uint64(b[3])<<16) |
		int64(uint64(b[4])<<8) |
		int64(uint64(b[5]))

	if ts < before || ts > after {
		t.Fatalf("timestamp %d outside [%d, %d] window (id=%q)", ts, before, after, id)
	}
}

func TestUUIDv7UniqueUnderRapidCalls(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := NewV7()
		if _, dup := seen[id]; dup {
			t.Fatalf("collision after %d calls: id=%q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestUUIDv7ChronologicalSortAcrossCalls(t *testing.T) {
	first := NewV7()
	time.Sleep(2 * time.Millisecond)
	second := NewV7()
	if first >= second {
		t.Fatalf("expected second > first across ms boundary: first=%q second=%q", first, second)
	}
}

func TestUUIDv7MonotonicWithinSingleMs(t *testing.T) {
	time.Sleep(1 * time.Millisecond)
	const n = 100
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = NewV7()
	}
	seen := make(map[string]struct{}, n)
	for i, id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("collision at i=%d: id=%q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestUUIDv7ConcurrentSafety(t *testing.T) {
	const goroutines = 8
	const perGoroutine = 250
	ids := make(chan string, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ids <- NewV7()
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]struct{}, goroutines*perGoroutine)
	for id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("collision in concurrent calls: id=%q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != goroutines*perGoroutine {
		t.Fatalf("expected %d unique ids, got %d", goroutines*perGoroutine, len(seen))
	}
}
