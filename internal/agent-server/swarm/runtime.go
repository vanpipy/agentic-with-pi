package swarm

import (
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// RuntimeOpts configures the manager's resource limits and watcher
// cadence. Defaults match jcode's agents.swarm_max_concurrent_agents and
// DEFAULT_SWARM_TASK_HEARTBEAT_SECS / DEFAULT_SWARM_STALE_AFTER_SECS
// (see ADR §15 D10-D13). Callers tune via NewWithSwarm. Tests override
// for fast timeouts.
type RuntimeOpts struct {
	// MaxMembersPerSwarm caps concurrent members per swarm. S1
	// exposes MaxSwarmMembers=1000 as the absolute ceiling. The
	// admission policy defaults to SpawnAdmissionPerSwarm=32 to match
	// jcode's default.
	MaxMembersPerSwarm int

	// AdmissionPolicy returns whether a new spawn should be admitted.
	// nil means default admission (count <= MaxMembersPerSwarm).
	AdmissionPolicy func(swarmID string, current int, requested int) error

	// HeartbeatInterval is how often the heartbeat watcher scans
	// for stale members. Mirrors jcode
	// DEFAULT_SWARM_TASK_HEARTBEAT_SECS=10.
	HeartbeatInterval time.Duration

	// StaleAfter is the threshold past which a running member
	// without a heartbeat is marked running_stale. Mirrors jcode
	// DEFAULT_SWARM_STALE_AFTER_SECS=45.
	StaleAfter time.Duration

	// IdleReapAfter is the threshold past which a headless member in
	// ready / running_stale is auto-stopped. Mirrors jcode's
	// DEFAULT_SWARM_IDLE_WORKER_REAP_SECS=30min (server/swarm.rs:264).
	IdleReapAfter time.Duration

	// PersistBatchInterval is the maximum interval between flushes
	// of the append queue to disk. On crash, up to one interval of
	// mutations may be lost. Documented as the SLA.
	PersistBatchInterval time.Duration

	// AwaitDefaultTimeout is the deadline applied when
	// comm.await_members omits timeout_secs. Matches
	// swarmproto.AwaitDefaultTimeoutSeconds.
	AwaitDefaultTimeout time.Duration

	// AwaitMaxTimeout caps caller-requested timeouts. Matches
	// swarmproto.AwaitTimeoutMaxSeconds.
	AwaitMaxTimeout time.Duration
}

// DefaultRuntimeOpts returns the production defaults documented in
// ADR §7.5 and ADR §15. Tests should clone and override, not mutate.
func DefaultRuntimeOpts() RuntimeOpts {
	return RuntimeOpts{
		MaxMembersPerSwarm:   swarmproto.SpawnAdmissionPerSwarm,
		HeartbeatInterval:    10 * time.Second,
		StaleAfter:           45 * time.Second,
		IdleReapAfter:        30 * time.Minute,
		PersistBatchInterval: 5 * time.Second,
		AwaitDefaultTimeout:  time.Duration(swarmproto.AwaitDefaultTimeoutSeconds) * time.Second,
		AwaitMaxTimeout:      time.Duration(swarmproto.AwaitTimeoutMaxSeconds) * time.Second,
	}
}

// Validate returns nil if opts is internally consistent. Empty opts
// (zero value) returns an error so callers can't accidentally run with
// silent zero-time tickers.
func (o RuntimeOpts) Validate() error {
	if o.MaxMembersPerSwarm <= 0 {
		return ErrInvalidRuntimeOpts
	}
	if o.MaxMembersPerSwarm > swarmproto.MaxSwarmMembers {
		return ErrInvalidRuntimeOpts
	}
	if o.HeartbeatInterval <= 0 {
		return ErrInvalidRuntimeOpts
	}
	if o.StaleAfter < o.HeartbeatInterval {
		return ErrInvalidRuntimeOpts
	}
	if o.PersistBatchInterval <= 0 {
		return ErrInvalidRuntimeOpts
	}
	if o.AwaitDefaultTimeout <= 0 {
		return ErrInvalidRuntimeOpts
	}
	if o.AwaitMaxTimeout < o.AwaitDefaultTimeout {
		return ErrInvalidRuntimeOpts
	}
	return nil
}

// agentEventEnvelope is the package-internal carrier between the
// dispatcher and the watchers. The wire-shape is jsonrpc.Response (the
// envelope already used for server-pushed events over the JSON-RPC
// stream). The manager reuses that exact type so callers do not need a
// second marshaling layer. The alias documents intent without adding a
// conversion step.
type agentEventEnvelope = jsonrpc.Response
