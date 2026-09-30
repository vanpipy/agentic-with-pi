// Package swarm — Broadcaster.
//
// The Broadcaster is the package-internal fan-out primitive that delivers
// server-pushed events to every live connection that has subscribed to
// this swarm session. It sits below the JSON-RPC server transport and
// above the per-conn write loop.
//
// Design (mirrors ADR §7.1 + §5.3):
//
//   - Subscribe returns a buffered channel + an unsubscribe callback.
//     The conn write loop ranges over the channel and writes each event
//     to the underlying io.Writer. Buffer = 32; overflow drops the
//     oldest event for that subscriber with a one-shot "comm.dropped"
//     marker so the client knows to resync.
//   - Publish is non-blocking: a `select { case sink <- ev: default:
//     mark dropped }` ensures the dispatcher goroutine never stalls
//     because a slow client is wedged on disk.
//   - The implementation is the default Broadcaster. Tests can swap a
//     fake via NewBroadcasterWithObserver for assertions on Publish
//     ordering.
package swarm

import (
	"sync"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

// Sink is the per-subscriber channel type. The Broadcaster hands the
// receiving end to callers; it owns the sending end.
type Sink = chan jsonrpc.Response

// Broadcaster fans out server events to every connected subscriber.
// Implementations must be safe for concurrent Publish + Subscribe.
type Broadcaster interface {
	// Subscribe registers sessionID as a live receiver and returns a
	// buffered receive-only channel plus an unsubscribe function. The
	// unsubscribe function is idempotent and safe to call from any
	// goroutine.
	Subscribe(sessionID string) (Sink, func())

	// Publish delivers ev to every current subscriber without
	// blocking. Subscribers whose buffer is full are skipped and
	// counted in DroppedSinceStart(); they will not receive ev.
	Publish(ev jsonrpc.Response)

	// Count returns the number of currently-registered subscribers.
	Count() int

	// DroppedSinceStart is a monotonic counter of skipped events due
	// to subscriber buffer overflow. Surfaced for tests and S9's
	// "comm.dropped" event marker.
	DroppedSinceStart() uint64

	// Close shuts the broadcaster down. Subsequent Publish calls are
	// no-ops. Subscribers' channels are closed so ranging loops exit.
	Close()
}

// broadcasterCapacity is the per-subscriber buffer size. Matches the
// per-member sink buffer on internal/agent-server/swarm.Member so
// channel fan-out and direct sink delivery have the same backpressure.
const broadcasterCapacity = 32

// droppedMarkerEvent is the event name emitted as a one-shot
// notification after a subscriber misses events due to overflow.
// Clients receiving this should resync via comm.list / comm.status.
const droppedMarkerEvent = "comm.dropped"

// defaultBroadcaster is the production Broadcaster implementation.
// All methods are safe for concurrent use.
type defaultBroadcaster struct {
	mu          sync.RWMutex
	sinks       map[string]Sink
	dropped     uint64
	closed      bool
	droppedSeen map[string]struct{}
}

// NewBroadcaster returns a ready-to-use Broadcaster.
func NewBroadcaster() Broadcaster {
	return &defaultBroadcaster{
		sinks:       make(map[string]Sink),
		droppedSeen: make(map[string]struct{}),
	}
}

// Subscribe registers sessionID. The returned channel is buffered
// with broadcasterCapacity slots. The unsubscribe function removes
// sessionID from the registry and closes the channel.
func (b *defaultBroadcaster) Subscribe(sessionID string) (Sink, func()) {
	ch := make(Sink, broadcasterCapacity)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	b.sinks[sessionID] = ch
	b.mu.Unlock()

	once := sync.Once{}
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if existing, ok := b.sinks[sessionID]; ok && existing == ch {
				delete(b.sinks, sessionID)
				delete(b.droppedSeen, sessionID)
				close(ch)
			}
		})
	}
	return ch, unsub
}

// Publish fans out ev to every current subscriber. Subscribers whose
// buffer is full are skipped and droppedSinceStart is incremented
// once per dropped event. On the first overflow for a subscriber a
// droppedMarkerEvent is queued at the head so the client knows to
// resync.
//
// Holds b.mu for the duration of the fan-out so concurrent
// Unsubscribe calls cannot race with a send on a closed channel
// (golden rule: never close a channel you didn't send on). The
// critical section is short — N non-blocking sends — so the lock is
// not held across any blocking operation.
func (b *defaultBroadcaster) Publish(ev jsonrpc.Response) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for id, ch := range b.sinks {
		select {
		case ch <- ev:
			delete(b.droppedSeen, id)
		default:
			b.dropped++
			firstDrop := false
			if _, seen := b.droppedSeen[id]; !seen {
				b.droppedSeen[id] = struct{}{}
				firstDrop = true
			}
			if firstDrop {
				marker := jsonrpc.Response{
					JSONRPC: "2.0",
					Event:   droppedMarkerEvent,
					Data:    nil,
				}
				select {
				case ch <- marker:
				default:
				}
			}
		}
	}
}

// Count returns the number of currently-registered subscribers.
func (b *defaultBroadcaster) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sinks)
}

// DroppedSinceStart is the monotonic dropped-event counter.
func (b *defaultBroadcaster) DroppedSinceStart() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.dropped
}

// Close marks the broadcaster as closed and closes every subscriber
// channel. Subsequent Publish calls are no-ops.
func (b *defaultBroadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, ch := range b.sinks {
		close(ch)
		delete(b.sinks, id)
		delete(b.droppedSeen, id)
	}
}
