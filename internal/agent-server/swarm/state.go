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

// SwarmState is the in-memory root registry for one server process.
// It owns the per-member Member objects, the per-swarm channel index,
// the per-await Awaiter objects, and the Broadcaster that delivers
// server-pushed events to live connections.
//
// SwarmState is safe for concurrent use. Public methods take internal
// locks; callers must NOT hold Member.mu across SwarmState calls (the
// per-member mutex can deadlock with the registry mutex).
//
// Lifecycle (ADR §5 + §7.5): a SwarmState is created once at server
// startup via NewSwarmState. Background watchers (S2b) hang goroutines
// off it. Server.Shutdown calls Close to drain subscribers + mark
// shutting-down for new admit calls.
type SwarmState struct {
	mu sync.RWMutex

	// members is keyed by session_id. A session_id may belong to at
	// most one swarm at a time; reassignment is an ErrRoleConflict
	// (matches jcode swarm_mutation_state.rs).
	members map[string]*Member

	// channels is keyed first by swarm_id then by channel name.
	// A channel is created lazily on first subscribe; the creator
	// session becomes the first subscriber.
	channels map[string]map[string]*Channel

	// awaiters is keyed by await id. Populated by AwaitMembers
	// (S2b); entries here MUST keep their per-await goroutine.
	awaiters map[string]*Awaiter

	// broadcaster fans server-pushed events to live connections.
	broadcaster Broadcaster

	// admission + heartbeat knobs are snapshots of RuntimeOpts taken
	// at construction time. Changing them post-construction is not
	// supported (tests build fresh SwarmState instances).
	maxMembersPerSwarm int
	admissionPolicy    func(swarmID string, current int, requested int) error

	// now is the clock. Tests inject a fixed function; production
	// calls time.Now via the defaultClock indirection.
	now func() time.Time

	// shuttingDown is set by Close. Subsequent admit calls return
	// ErrShuttingDown; reads continue to succeed so callers can
	// drain pending reads.
	shuttingDown bool

	// closing is set when Close starts the subscriber drain.
	closing chan struct{}
}

// NewSwarmState returns a SwarmState ready for use. The opts.MaxMembersPerSwarm
// and opts.AdmissionPolicy are captured here; other RuntimeOpts fields
// (heartbeat / stale / idle reap / persist / await) are consumed by
// the watchers added in S2b.
func NewSwarmState(opts RuntimeOpts) *SwarmState {
	if opts.MaxMembersPerSwarm <= 0 {
		opts.MaxMembersPerSwarm = swarmproto.SpawnAdmissionPerSwarm
	}
	return &SwarmState{
		members:            make(map[string]*Member),
		channels:           make(map[string]map[string]*Channel),
		awaiters:           make(map[string]*Awaiter),
		broadcaster:        NewBroadcaster(),
		maxMembersPerSwarm: opts.MaxMembersPerSwarm,
		admissionPolicy:    opts.AdmissionPolicy,
		now:                defaultClock,
		closing:            make(chan struct{}),
	}
}

// defaultClock is the production clock source. Tests override
// SwarmState.now directly via SetClock.
func defaultClock() time.Time { return time.Now() }

// SetClock swaps the time source. Tests inject a function that returns
// fixed timestamps so wire-shape assertions are deterministic.
func (s *SwarmState) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now == nil {
		s.now = defaultClock
		return
	}
	s.now = now
}

// Broadcaster returns the per-state Broadcaster. Callers outside this
// package (S2b dispatcher, S2b server integration) use this to wire
// connection sinks.
func (s *SwarmState) Broadcaster() Broadcaster {
	return s.broadcaster
}

// Now returns the current wall-clock value per the injected source.
// Safe for callers that do not hold s.mu.
func (s *SwarmState) Now() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.now()
}

// nowLocked returns the current wall-clock value per the injected source.
// Caller MUST hold s.mu (read or write). Use this from inside the
// critical section to avoid recursive RLock.
func (s *SwarmState) nowLocked() time.Time {
	return s.now()
}

// Close shuts the state down. After Close:
//   - Register / Spawn return ErrShuttingDown.
//   - The Broadcaster is closed so ranging subscriber loops exit.
//   - The closing channel is closed so background watchers (S2b) can
//     observe shutdown via select.
//
// Close is idempotent and safe to call concurrently.
func (s *SwarmState) Close() {
	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return
	}
	s.shuttingDown = true
	select {
	case <-s.closing:
	default:
		close(s.closing)
	}
	bcast := s.broadcaster
	s.mu.Unlock()

	if bcast != nil {
		bcast.Close()
	}
}

// Done returns a channel that is closed when Close has been called.
// Background watchers select on this to exit their loops.
func (s *SwarmState) Done() <-chan struct{} {
	return s.closing
}

// IsShuttingDown reports whether Close has been called.
func (s *SwarmState) IsShuttingDown() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.shuttingDown
}

// MemberCount returns the total number of registered members.
func (s *SwarmState) MemberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.members)
}

// SwarmMemberCount returns the number of members in one swarm.
func (s *SwarmState) SwarmMemberCount(swarmID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, m := range s.members {
		if m.Record.SwarmID == swarmID {
			n++
		}
	}
	return n
}

// GetMember looks up a member by session_id.
func (s *SwarmState) GetMember(sessionID string) (*Member, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.members[sessionID]
	return m, ok
}

// admitLock is the per-swarm admission helper called by Spawn. It
// returns ErrCapacityExceeded when current+requested exceeds
// maxMembersPerSwarm (or AdmissionPolicy says so).
//
// Caller MUST hold s.mu (write lock) because we read SwarmMemberCount
// under that lock instead of recursively taking RLock from inside the
// write critical section.
func (s *SwarmState) admitLock(swarmID string, requested int) error {
	current := 0
	for _, m := range s.members {
		if m.Record.SwarmID == swarmID {
			current++
		}
	}
	if s.admissionPolicy != nil {
		return s.admissionPolicy(swarmID, current, requested)
	}
	if current+requested > s.maxMembersPerSwarm {
		return ErrCapacityExceeded
	}
	return nil
}
