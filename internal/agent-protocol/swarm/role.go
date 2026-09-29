package swarm

import "encoding/json"

// Role is a sealed enum identifying the relationship a member has to
// its swarm. The 2 explicit values plus Other mirror jcode's
// SwarmRole enum. Constructors are unexported so callers cannot
// fabricate invalid roles.
type Role struct{ s string }

// Defined roles. Strings are frozen (ADR §15 D9).
var (
	// RoleAgent is the default role for spawned worker sessions.
	// They execute tasks and report status upward.
	RoleAgent = Role{"agent"}

	// RoleCoordinator is the role of the session that owns the
	// swarm (typically the first session in a swarm). Coordinator
	// can approve/reject proposals and broadcast assignments.
	RoleCoordinator = Role{"coordinator"}

	// RoleOther is the catch-all for forward-compat with new jcode
	// roles. Callers that want forward-compat should switch on it
	// explicitly.
	RoleOther = Role{""}
)

// ParseRole maps a wire string to its Role constant.
func ParseRole(s string) Role {
	switch s {
	case "agent":
		return RoleAgent
	case "coordinator":
		return RoleCoordinator
	default:
		return RoleOther
	}
}

// String returns the wire representation.
func (r Role) String() string { return r.s }

// IsOther reports whether this is the catch-all value.
func (r Role) IsOther() bool { return r.s == "" }

// IsCoordinator reports whether this role is the swarm coordinator.
func (r Role) IsCoordinator() bool { return r == RoleCoordinator }

// IsAgent reports whether this role is a worker (non-coordinator).
func (r Role) IsAgent() bool { return r == RoleAgent }

// MarshalJSON implements json.Marshaler.
func (r Role) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.s)
}

// UnmarshalJSON implements json.Unmarshaler. Empty strings map to
// RoleOther; unknown strings also map to RoleOther (forward compat).
func (r *Role) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*r = ParseRole(s)
	return nil
}
