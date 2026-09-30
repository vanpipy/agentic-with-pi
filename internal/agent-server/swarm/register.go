// Package swarm: member registration: Register / Unregister / Spawn /
// Stop / AssignRole / List.
//
// Mirrors jcode crates/jcode-app-core/src/server/swarm_mutation_state.rs
// (Spawn admission cap, role-conflict detection) and the lifecycle in
// ADR §5.1.
//
// Spawn admission is enforced via SwarmState.admitLock: a new member
// is admitted iff SwarmMemberCount(swarmID) + 1 <= maxMembersPerSwarm
// (or a custom AdmissionPolicy returns nil). The state mutex is held
// across the entire admit + insert so a burst of N concurrent spawns
// cannot exceed the cap.
package swarm

import (
	"time"

	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// SpawnOptions is the params payload for comm.spawn. Mirrors the
// fields documented in ADR §6.2.
type SpawnOptions struct {
	// FromSessionID is the parent session requesting the spawn. It
	// is recorded as ParentSessionID on the new MemberRecord so the
	// tree of spawned sessions is recoverable.
	FromSessionID string

	// InitialMessage is the prompt the new session starts with. Stored
	// on the MemberRecord's Detail for human-readable display.
	InitialMessage string

	// Model is the model identifier the new session should use. Empty
	// inherits from the parent. Recorded on MemberRuntime at spawn.
	Model string

	// Effort is the reasoning effort level (low/medium/high/xhigh/max).
	// Recorded on MemberRuntime at spawn.
	Effort string

	// Label is a short human-readable task label. Defaults to the
	// first 48 chars of DeriveTaskLabel(InitialMessage).
	Label string

	// SpawnMode is "visible" or "headless". Visible sessions bind to
	// a TUI connection; headless run on a dedicated goroutine.
	SpawnMode string

	// WorkingDir is the cwd the new session will operate in.
	WorkingDir string

	// Role is the role assigned to the new session. Defaults to
	// RoleAgent. The first session in a swarm is implicitly
	// RoleCoordinator; the dispatcher enforces this in S2b.
	Role swarmproto.Role

	// IsHeadless mirrors SpawnMode == "headless". Recorded on the
	// MemberRecord so persistence replays the right shape.
	IsHeadless bool

	// NewSessionID is the session id assigned to the new session. The
	// caller (dispatcher) generates a uuidv7 and passes it in so
	// Spawn does not need to depend on the uuidv7 package.
	NewSessionID string
}

// SpawnResult is the value returned by Spawn on success.
type SpawnResult struct {
	// Member is the freshly-inserted Member. Its Status is
	// StatusSpawned; callers (dispatcher) drive it through
	// ready -> running via the report handler.
	Member *Member

	// SwarmID is the swarm the new member belongs to. Equal to
	// Member.Record.SwarmID; surfaced separately for convenience.
	SwarmID string
}

// Register attaches a pre-existing MemberRecord to the state. Use
// when a session that was not born via Spawn later joins a swarm
// (e.g. the primary session that the server was started with).
//
// Returns ErrUnknownMember when rec.SessionID is empty or
// ErrRoleConflict when the session is already registered to a
// different swarm.
//
// Records are mutated in place: LastUpdatedAt and CreatedAt are
// stamped to the current wall-clock value.
func (s *SwarmState) Register(rec swarmproto.MemberRecord) (*Member, error) {
	if rec.SessionID == "" {
		return nil, ErrUnknownMember
	}
	now := s.Now()
	rec.CreatedAt = now.UTC().Format(time.RFC3339Nano)
	rec.LastUpdatedAt = rec.CreatedAt
	if rec.LastHeartbeatAt == "" {
		rec.LastHeartbeatAt = rec.CreatedAt
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return nil, ErrShuttingDown
	}
	if existing, ok := s.members[rec.SessionID]; ok {
		if existing.Record.SwarmID != rec.SwarmID {
			return nil, ErrRoleConflict
		}
		existing.Record = rec
		return existing, nil
	}
	m := newMember(rec, now)
	s.members[rec.SessionID] = m
	return m, nil
}

// Unregister removes a session from the state. Returns ErrUnknownMember
// when the session is not present. The per-member Sink is not closed
// here; Broadcaster.Unsubscribe (the conn write loop) owns that.
func (s *SwarmState) Unregister(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.members[sessionID]; !ok {
		return ErrUnknownMember
	}
	delete(s.members, sessionID)
	s.RemoveSessionLocked(sessionID)
	return nil
}

// Spawn creates a new member for opts.NewSessionID inside the swarm
// referenced by opts.FromSessionID's current membership. If the parent
// is not yet in any swarm, the new member becomes a coordinator in a
// freshly-named swarm whose id is opts.FromSessionID.
//
// Returns:
//   - ErrShuttingDown if Close has been called.
//   - ErrCapacityExceeded if admission cap would be exceeded.
//   - ErrRoleConflict if NewSessionID is already registered to a
//     different swarm.
//   - ErrUnknownMember if NewSessionID is empty.
func (s *SwarmState) Spawn(opts SpawnOptions) (*SpawnResult, error) {
	if opts.NewSessionID == "" {
		return nil, ErrUnknownMember
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return nil, ErrShuttingDown
	}
	if existing, ok := s.members[opts.NewSessionID]; ok {
		// Idempotent re-spawn of the same id within the same swarm:
		// return the existing member. A different swarm is a conflict.
		if existing.Record.SwarmID != swarmIDFor(opts) {
			return nil, ErrRoleConflict
		}
		return &SpawnResult{Member: existing, SwarmID: existing.Record.SwarmID}, nil
	}

	swarmID := swarmIDFor(opts)
	if err := s.admitLock(swarmID, 1); err != nil {
		return nil, err
	}

	now := s.nowLocked()
	nowStr := now.UTC().Format(time.RFC3339Nano)

	label := opts.Label
	if label == "" {
		label = swarmproto.DeriveTaskLabel(opts.InitialMessage)
	}
	role := opts.Role
	if role.IsOther() {
		role = swarmproto.RoleAgent
	}

	rec := swarmproto.MemberRecord{
		SessionID:       opts.NewSessionID,
		SwarmID:         swarmID,
		WorkingDir:      opts.WorkingDir,
		SwarmEnabled:    true,
		Status:          swarmproto.StatusSpawned,
		TaskLabel:       label,
		Role:            role,
		IsHeadless:      opts.IsHeadless,
		ParentSessionID: opts.FromSessionID,
		CreatedAt:       nowStr,
		LastUpdatedAt:   nowStr,
		LastHeartbeatAt: nowStr,
	}
	if opts.InitialMessage != "" {
		rec.Detail = opts.InitialMessage
	}
	if opts.FromSessionID == opts.NewSessionID {
		// Self-spawn = first member of a new swarm is the coordinator.
		rec.Role = swarmproto.RoleCoordinator
	}

	m := newMember(rec, now)
	m.Record = rec
	s.members[opts.NewSessionID] = m
	return &SpawnResult{Member: m, SwarmID: swarmID}, nil
}

// swarmIDFor resolves the swarm id for a Spawn. If the parent session
// is already a member of a swarm, the new member joins the same
// swarm; otherwise the new swarm id is the parent's session id (the
// first session is its own coordinator).
func swarmIDFor(opts SpawnOptions) string {
	return opts.FromSessionID
}

// Stop marks the member as stopped (force=false) or crashed (force=true)
// and removes it from the registry. Idempotent: stopping an unknown
// session returns ErrUnknownMember.
//
// Stopped sessions emit a terminal status transition but their
// persistent record is preserved until Unregister (caller-driven) so
// the broadcast of EventSwarmMemberUpdate can be observed by the
// conn write loop.
func (s *SwarmState) Stop(sessionID string, force bool) error {
	s.mu.Lock()
	m, ok := s.members[sessionID]
	if !ok {
		s.mu.Unlock()
		return ErrUnknownMember
	}
	target := swarmproto.StatusStopped
	if force {
		target = swarmproto.StatusCrashed
	}
	m.mu.Lock()
	if !m.Status.IsTerminal() {
		m.Status = target
		m.StoppedAt = s.now()
		m.Record.Status = target
		m.Record.LastUpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
	}
	m.mu.Unlock()
	delete(s.members, sessionID)
	s.RemoveSessionLocked(sessionID)
	s.mu.Unlock()
	return nil
}

// AssignRole updates the role on an existing member. Returns
// ErrUnknownMember if the session is not registered. Returns
// ErrRoleConflict when the requested role is already held by another
// live member of the same swarm.
func (s *SwarmState) AssignRole(sessionID string, role swarmproto.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.members[sessionID]
	if !ok {
		return ErrUnknownMember
	}
	if role == swarmproto.RoleCoordinator {
		for otherID, other := range s.members {
			if otherID == sessionID {
				continue
			}
			if other.Record.SwarmID == m.Record.SwarmID && other.Record.Role == swarmproto.RoleCoordinator {
				return ErrRoleConflict
			}
		}
	}
	m.mu.Lock()
	m.Record.Role = role
	m.Record.LastUpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
	m.mu.Unlock()
	return nil
}

// List returns the current MemberStatus snapshot for every registered
// member. The result is sorted by SessionID for deterministic output.
//
// StatusAgeSecs is the elapsed seconds since the last status
// transition; LastActivitySecs is the elapsed seconds since the last
// activity stamp. Both nil when the member is fresh (< 1s old).
func (s *SwarmState) List() []swarmproto.MemberStatus {
	return s.ListBySwarm("")
}

// ListBySwarm returns the MemberStatus snapshot for every member
// whose SwarmID matches swarmID. Empty swarmID matches all. The result
// is sorted by SessionID for deterministic output.
func (s *SwarmState) ListBySwarm(swarmID string) []swarmproto.MemberStatus {
	s.mu.RLock()
	now := s.now()
	ids := make([]string, 0, len(s.members))
	for id, m := range s.members {
		if swarmID != "" && m.Record.SwarmID != swarmID {
			continue
		}
		ids = append(ids, id)
	}
	s.mu.RUnlock()
	sortStrings(ids)

	out := make([]swarmproto.MemberStatus, 0, len(ids))
	for _, id := range ids {
		s.mu.RLock()
		m, ok := s.members[id]
		s.mu.RUnlock()
		if !ok {
			continue
		}
		out = append(out, memberStatusAt(m, now))
	}
	return out
}

// memberStatusAt snapshots the public MemberStatus from a Member.
// Holds m.mu briefly while reading; takes no SwarmState lock so it
// does not deadlock with the registry lock when callers iterate.
func memberStatusAt(m *Member, now time.Time) swarmproto.MemberStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.Status
	lastUpdate := m.Record.LastUpdatedAt
	lastActivity := m.LastActivity
	sess := m.Record.SessionID
	name := m.Record.FriendlyName
	label := m.Record.TaskLabel
	role := m.Record.Role
	headless := m.Record.IsHeadless
	latest := m.Record.LatestReport
	tldr := m.Record.LatestReportTLDR
	swarmID := m.Record.SwarmID
	parent := m.Record.ParentSessionID
	rptBack := m.Record.ReportBackToID
	statusCopy := status

	stamp := parseRFC3339NanoOrZero(lastUpdate)
	ageSecs := uint64(0)
	if !stamp.IsZero() {
		d := now.Sub(stamp)
		if d > 0 {
			ageSecs = uint64(d.Seconds())
		}
	}
	actSecs := uint64(0)
	if !lastActivity.IsZero() {
		d := now.Sub(lastActivity)
		if d > 0 {
			actSecs = uint64(d.Seconds())
		}
	}
	var agePtr *uint64
	if ageSecs > 0 {
		agePtr = &ageSecs
	}
	var actPtr *uint64
	if actSecs > 0 {
		actPtr = &actSecs
	}
	return swarmproto.MemberStatus{
		SessionID:        sess,
		FriendlyName:     name,
		Status:           statusCopy,
		TaskLabel:        label,
		Role:             role,
		IsHeadless:       headless,
		StatusAgeSecs:    agePtr,
		LastActivitySecs: actPtr,
		ReportBackToID:   rptBack,
		LatestReport:     latest,
		LatestReportTLDR: tldr,
		ParentSessionID:  parent,
		SwarmID:          swarmID,
	}
}

// parseRFC3339NanoOrZero returns the parsed time or zero on any
// failure. Used for the optional LastUpdatedAt / LastHeartbeatAt
// fields on MemberRecord.
func parseRFC3339NanoOrZero(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
