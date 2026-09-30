package swarm_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarm "github.com/vanpiyp/awp/internal/agent-server/swarm"
)

func TestBroadcasterSubscribePublish(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()

	ch, unsub := b.Subscribe("s1")
	defer unsub()

	ev := jsonrpc.Response{JSONRPC: "2.0", Event: "ping", ID: "1"}
	b.Publish(ev)

	select {
	case got := <-ch:
		if got.Event != "ping" {
			t.Errorf("event = %q, want ping", got.Event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("did not receive published event")
	}
}

func TestBroadcasterFanOutToAllSubscribers(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()

	const subscribers = 5
	chans := make([]swarm.Sink, subscribers)
	for i := range chans {
		c, unsub := b.Subscribe(string(rune('a' + i)))
		t.Cleanup(unsub)
		chans[i] = c
	}

	if got := b.Count(); got != subscribers {
		t.Fatalf("Count = %d, want %d", got, subscribers)
	}

	ev := jsonrpc.Response{JSONRPC: "2.0", Event: "broadcast", ID: "x"}
	b.Publish(ev)

	for i, c := range chans {
		select {
		case got := <-c:
			if got.Event != "broadcast" {
				t.Errorf("subscriber %d: event = %q, want broadcast", i, got.Event)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d did not receive event", i)
		}
	}
}

func TestBroadcasterUnsubscribeClosesChannel(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()
	ch, unsub := b.Subscribe("s1")

	unsub()
	if got := b.Count(); got != 0 {
		t.Errorf("Count after unsubscribe = %d, want 0", got)
	}
	// Reading from a closed channel returns zero value immediately.
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("channel should be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("unsubscribed channel did not close")
	}

	// Idempotent unsubscribe must not panic.
	unsub()
}

func TestBroadcasterDropOnFullBuffer(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()
	ch, unsub := b.Subscribe("s1")
	defer unsub()

	// Fill buffer and overflow without draining. Subscriber must see
	// the dropped counter increment. The droppedMarkerEvent is
	// best-effort and may be lost if the buffer stays full; the
	// counter is the source of truth.
	for i := 0; i < 64; i++ {
		b.Publish(jsonrpc.Response{JSONRPC: "2.0", Event: "flood", ID: "x"})
	}
	if dropped := b.DroppedSinceStart(); dropped == 0 {
		t.Errorf("DroppedSinceStart = 0, want > 0 after overflowing subscriber")
	}

	// Verify the subscriber still has buffered events (at least one
	// made it through before the buffer filled).
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Errorf("subscriber received no events before overflow")
	}
}

func TestBroadcasterPublishAfterCloseIsNoop(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()
	ch, unsub := b.Subscribe("s1")
	defer unsub()

	b.Close()

	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("channel should be closed after Broadcaster.Close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("channel did not close after Close")
	}

	// Publish after close must not panic.
	b.Publish(jsonrpc.Response{JSONRPC: "2.0", Event: "after_close"})

	// Close is idempotent.
	b.Close()
}

func TestBroadcasterConcurrentSubscribePublish(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()

	var wg sync.WaitGroup
	const goroutines = 16
	const ops = 50

	var received atomic.Uint64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				sid := "s" + string(rune('a'+id%goroutines))
				ch, unsub := b.Subscribe(sid)
				b.Publish(jsonrpc.Response{JSONRPC: "2.0", Event: "e"})
				select {
				case <-ch:
					received.Add(1)
				case <-time.After(50 * time.Millisecond):
				}
				unsub()
			}
		}(g)
	}
	wg.Wait()
	if received.Load() == 0 {
		t.Errorf("no events received across %d goroutines", goroutines)
	}
}

func TestBroadcasterSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()
	b.Close()

	ch, unsub := b.Subscribe("s1")
	defer unsub()

	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("Subscribe after Close should return a closed channel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Subscribe after Close did not return closed channel")
	}
}

func TestBroadcasterUnsubscribeTwiceIdempotent(t *testing.T) {
	t.Parallel()
	b := swarm.NewBroadcaster()
	_, unsub := b.Subscribe("s1")
	unsub()
	// Must not panic on double-close.
	unsub()
}
