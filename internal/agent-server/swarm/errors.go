package swarm

import "errors"

// Sentinel errors returned by Manager methods. Per project convention
// (AGENTS.md golden rule #4), these are the behavior contract. JSON-RPC
// layer translates them to typed responses (see ADR §1 method table
// and §6 error envelope).
//
// Each sentinel wraps to a stable error code on the wire. The mapping
// is one-to-one with the codes in internal/agent-protocol/swarm/errors
// (added in S3).
var (
	// ErrUnknownSwarm indicates the requested swarm_id does not exist.
	// Wire code: swarm.unknown_swarm.
	ErrUnknownSwarm = errors.New("swarm: unknown swarm")

	// ErrUnknownMember indicates the requested session_id is not
	// (or no longer) a member of the addressed swarm. Wire code:
	// swarm.unknown_member.
	ErrUnknownMember = errors.New("swarm: unknown member")

	// ErrCapacityExceeded indicates the spawn admission cap was hit.
	// Wire code: swarm.capacity_exceeded.
	ErrCapacityExceeded = errors.New("swarm: capacity exceeded")

	// ErrRoleConflict indicates a caller requested a role that is
	// already held by another live member. Wire code:
	// swarm.role_conflict.
	ErrRoleConflict = errors.New("swarm: role conflict")

	// ErrNoPlan indicates the request addressed a DAG node but the
	// owning plan does not exist. Wire code: swarm.no_plan.
	ErrNoPlan = errors.New("swarm: no plan")

	// ErrUnknownNode indicates a DAG node id is not part of the
	// addressed plan. Wire code: swarm.unknown_node.
	ErrUnknownNode = errors.New("swarm: unknown node")

	// ErrCycleDetected indicates SeedGraph / InjectGap produced a
	// dependency cycle. Wire code: swarm.cycle_detected.
	ErrCycleDetected = errors.New("swarm: cycle detected")

	// ErrUnknownDependency indicates a SeedGraph / InjectGap request
	// referenced a `blocked_by` id that does not exist in the plan.
	// Wire code: swarm.unknown_dependency.
	ErrUnknownDependency = errors.New("swarm: unknown dependency")

	// ErrNotOwner indicates a caller tried to mutate a member they do
	// not own. Wire code: swarm.not_owner.
	ErrNotOwner = errors.New("swarm: not owner")

	// ErrInvalidTransition indicates a state-machine transition that
	// the lifecycle forbids. Wire code: swarm.invalid_transition.
	ErrInvalidTransition = errors.New("swarm: invalid transition")

	// ErrShuttingDown indicates the manager is shutting down and cannot
	// accept new work. Wire code: swarm.shutting_down.
	ErrShuttingDown = errors.New("swarm: shutting down")

	// ErrInvalidRuntimeOpts indicates the supplied RuntimeOpts is
	// missing fields or internally inconsistent. Wire code:
	// swarm.invalid_runtime_opts.
	ErrInvalidRuntimeOpts = errors.New("swarm: invalid runtime opts")

	// ErrUnknownChannel indicates the named channel does not exist on
	// the addressed swarm. Wire code: swarm.unknown_channel.
	ErrUnknownChannel = errors.New("swarm: unknown channel")

	// ErrTooLarge indicates a payload exceeded the S1 hard caps
	// (TLDR length, channel body, shared context entry, ...). Wire
	// code: swarm.too_large.
	ErrTooLarge = errors.New("swarm: too large")
)
