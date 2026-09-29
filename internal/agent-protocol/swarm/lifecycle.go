package swarm

import "encoding/json"

// LifecycleStatus is a sealed enum representing a member's lifecycle
// state. The 13 explicit values mirror ~/Project/jcode's
// SwarmLifecycleStatus (crates/jcode-swarm-core/src/lib.rs:136-151)
// exactly; StatusOther is the catch-all for forward-compat.
//
// All callers must use the exported package vars (StatusRunning,
// StatusCompleted, ...) or ParseLifecycleStatus to obtain a value;
// constructor functions are unexported so no external code can
// fabricate invalid lifecycle states. Type-switches over
// LifecycleStatus are exhaustive by construction.
type LifecycleStatus struct{ s string }

// Defined lifecycle values. Strings are frozen (see ADR §15 D9).
var (
	StatusSpawned      = LifecycleStatus{"spawned"}
	StatusReady        = LifecycleStatus{"ready"}
	StatusRunning      = LifecycleStatus{"running"}
	StatusRunningStale = LifecycleStatus{"running_stale"}
	StatusCompleted    = LifecycleStatus{"completed"}
	StatusDone         = LifecycleStatus{"done"}
	StatusFailed       = LifecycleStatus{"failed"}
	StatusStopped      = LifecycleStatus{"stopped"}
	StatusCrashed      = LifecycleStatus{"crashed"}
	StatusQueued       = LifecycleStatus{"queued"}
	StatusBlocked      = LifecycleStatus{"blocked"}
	StatusPending      = LifecycleStatus{"pending"}
	StatusTodo         = LifecycleStatus{"todo"}

	// StatusOther is returned by ParseLifecycleStatus for any unknown
	// string. Callers that want forward-compat should switch on it
	// explicitly to handle new lifecycle values jcode might introduce.
	StatusOther = LifecycleStatus{""}
)

// ParseLifecycleStatus maps a wire string to its LifecycleStatus
// constant. Unknown strings (forward-compat with jcode) map to
// StatusOther; callers may treat it as opaque.
func ParseLifecycleStatus(s string) LifecycleStatus {
	switch s {
	case "spawned":
		return StatusSpawned
	case "ready":
		return StatusReady
	case "running":
		return StatusRunning
	case "running_stale":
		return StatusRunningStale
	case "completed":
		return StatusCompleted
	case "done":
		return StatusDone
	case "failed":
		return StatusFailed
	case "stopped":
		return StatusStopped
	case "crashed":
		return StatusCrashed
	case "queued":
		return StatusQueued
	case "blocked":
		return StatusBlocked
	case "pending":
		return StatusPending
	case "todo":
		return StatusTodo
	default:
		return StatusOther
	}
}

// String returns the wire representation. StatusOther returns "".
func (l LifecycleStatus) String() string { return l.s }

// IsOther reports whether this is the catch-all value (unknown wire
// string or empty input).
func (l LifecycleStatus) IsOther() bool { return l.s == "" }

// IsTerminal reports whether the status represents a final state
// (no further automatic transitions). Terminal statuses: completed,
// done, failed, stopped, crashed.
func (l LifecycleStatus) IsTerminal() bool {
	switch l {
	case StatusCompleted, StatusDone, StatusFailed, StatusStopped, StatusCrashed:
		return true
	}
	return false
}

// IsCompleted reports whether the status represents successful
// completion (completed or done).
func (l LifecycleStatus) IsCompleted() bool {
	return l == StatusCompleted || l == StatusDone
}

// IsFailed reports whether the status represents an unsuccessful
// terminal state (terminal but not completed).
func (l LifecycleStatus) IsFailed() bool {
	return l.IsTerminal() && !l.IsCompleted()
}

// IsActive reports whether the member is currently executing (running
// or running_stale). Used by heartbeat watcher to skip idle members.
func (l LifecycleStatus) IsActive() bool {
	return l == StatusRunning || l == StatusRunningStale
}

// IsRunnable reports whether the member is eligible to receive task
// assignments (queued, ready, pending, todo).
func (l LifecycleStatus) IsRunnable() bool {
	switch l {
	case StatusQueued, StatusReady, StatusPending, StatusTodo:
		return true
	}
	return false
}

// IsBlocked reports whether the member is blocked on a dependency.
func (l LifecycleStatus) IsBlocked() bool { return l == StatusBlocked }

// MarshalJSON implements json.Marshaler so LifecycleStatus round-trips
// over the wire. StatusOther marshals to "".
func (l LifecycleStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.s)
}

// UnmarshalJSON implements json.Unmarshaler. Empty strings map to
// StatusOther; unknown strings also map to StatusOther (forward
// compat).
func (l *LifecycleStatus) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*l = ParseLifecycleStatus(s)
	return nil
}
