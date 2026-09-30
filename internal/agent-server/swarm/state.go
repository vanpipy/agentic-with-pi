package swarm

import (
	"sync"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// Member is the in-memory representation of one spawned session inside
// a swarm. It wraps the wire-shape swarmproto.MemberRecord with the
// runtime fields the manager needs to enforce the lifecycle (see ADR
// §5.1) and the heartbeat watcher (see ADR §7.5).
//
// Members are owned by their SwarmState and must not be copied. The
// embedded mutex protects concurrent access from the dispatcher
// goroutine, the heartbeat watcher, the idle-reap watcher, and the
// persist batcher.
type Member struct {
	mu sync.Mutex

	// Record is the wire-shape swarmproto.MemberRecord this Member wraps.
	Record swarmproto.MemberRecord

	// Status is the current lifecycle value. Mutating it is a
	// state-machine operation. Callers must hold m.mu and verify the
	// transition against ADR §5.1.
	Status swarmproto.LifecycleStatus

	// LastHeartbeat is the wall-clock time of the most recent
	// heartbeat message from the underlying session. Reset by
	// HandleReport and the heartbeat watcher on tick.
	LastHeartbeat time.Time

	// LastActivity is the most recent user-visible activity
	// (comm.message in / out, comm.report, status change). Drives the
	// idle-reap watcher.
	LastActivity time.Time

	// StoppedAt is set when the member transitions to done / stopped
	// / crashed / failed. Zero while the member is live.
	StoppedAt time.Time

	// Sink is the per-member channel the broadcaster fans events to.
	// nil when no subscribers are attached (default at spawn).
	Sink chan jsonrpc.Response
}

// newMember constructs a Member from a wire record. Status starts at
// StatusSpawned. LastHeartbeat / LastActivity are set to now.
func newMember(rec swarmproto.MemberRecord, now time.Time) *Member {
	return &Member{
		Record:        rec,
		Status:        swarmproto.StatusSpawned,
		LastHeartbeat: now,
		LastActivity:  now,
		Sink:          make(chan jsonrpc.Response, 32),
	}
}

// Awaiter tracks one outstanding comm.await_members request. The
// dispatcher creates an Awaiter on receipt. The await goroutine (see
// ADR §5.3) consumes from the broadcaster and marks Completed when the
// target is met. The struct is owned by SwarmState.awaiters.
type Awaiter struct {
	mu sync.Mutex

	// ID is the unique identifier for this Awaiter, generated at
	// registration time.
	ID string

	// FromSession is the session_id of the caller that registered the
	// await. Used to scope the response back to the requesting client.
	FromSession string

	// SwarmID is the swarm this await is scoped to.
	SwarmID string

	// TargetMembers is the set of session_ids the caller is awaiting.
	// The await goroutine marks Completed when every member in this
	// set has reached a status in TargetStatuses.
	TargetMembers map[string]struct{}

	// TargetStatuses is the set of lifecycle values that satisfy the
	// await. If empty, "any terminal status" is implied (mirrors
	// jcode's AwaitedMemberStatus::default semantics).
	TargetStatuses map[swarmproto.LifecycleStatus]struct{}

	// Delivery selects the comm.DeliveryMode the response is sent
	// with when the await completes.
	Delivery swarmproto.DeliveryMode

	// Background reports whether the caller asked for the request to
	// detach (true) or block (false). Background true means the
	// response is delivered as a server event rather than as the
	// JSON-RPC reply.
	Background bool

	// Wake, when true, causes the await completion to also emit an
	// EventSwarmMemberUpdate so a sleeping caller wakes up promptly.
	Wake bool

	// Deadline is the absolute wall-clock deadline. If zero, defaults
	// to RuntimeOpts.AwaitDefaultTimeout applied at registration time.
	Deadline time.Time

	// Result holds the terminal outcome. Populated by the await
	// goroutine on completion. Readers take a.mu.
	Result *AwaitResult

	// Done is closed by the await goroutine exactly once, when Result
	// is set. Background callers range over this. Sync callers block
	// on it via AwaitMembers.
	Done chan struct{}
}

// AwaitResult is the terminal payload of an Awaiter. Members is the
// snapshot of status the awaiter observed at completion time.
type AwaitResult struct {
	// Completed is true if every TargetMember hit a TargetStatus
	// before Deadline. False on timeout or ctx cancel.
	Completed bool

	// Members is the per-target status at completion time. Always
	// populated even on timeout (mirrors jcode).
	Members []swarmproto.AwaitedMemberStatus

	// Reason is a human-readable tag ("matched", "timeout",
	// "cancelled"). Wire-shape consumers map to localized strings.
	Reason string
}

// Channel is a named pub/sub lane inside a SwarmState. Members
// subscribe via comm.subscribe_channel. Messages fan out via the
// Broadcaster (see ADR §4 broadcast.go).
type Channel struct {
	mu sync.Mutex

	// Name is the unique channel identifier within the owning swarm.
	Name string

	// Subscribers is the set of session_ids currently subscribed.
	// Membership checks are O(1).
	Subscribers map[string]struct{}

	// CreatedAt is set at registration time and never mutated.
	CreatedAt time.Time

	// LastActivity is updated on each publish or subscribe. Idle
	// channels are not reaped in S2. Deferred to S3 (per ADR §9).
	LastActivity time.Time
}

// newChannel constructs a Channel with the given name and registers
// the creator as the first subscriber.
func newChannel(name string, creatorSession string, now time.Time) *Channel {
	subs := make(map[string]struct{}, 1)
	subs[creatorSession] = struct{}{}
	return &Channel{
		Name:         name,
		Subscribers:  subs,
		CreatedAt:    now,
		LastActivity: now,
	}
}
