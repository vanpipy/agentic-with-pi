// Package swarm implements the server-side state machine, dispatcher,
// persistence, and runtime watchers that back the 29-method agent
// communication surface declared in internal/agent-protocol/swarm.
//
// The package owns no wire-format types. Those types live in
// internal/agent-protocol/swarm (frozen, see ADR §15 D9). This package
// holds the *behavioral* layer: a Manager that mutates server state in
// response to JSON-RPC requests, plus three long-running watchers
// (heartbeat, idle reap, persist batcher) that keep the state in sync
// with the real world.
//
// Lifecycle is governed by the state machine in ADR §5:
//
//   - members transition through spawned -> ready -> running -> {done,
//     failed, crashed, stopped, running_stale};
//   - awaits are event-driven via per-member sinks (no poll loop);
//   - DAG nodes have their own sub-state machine (queued -> running ->
//     done / failed) layered on top.
//
// The package is consumed by internal/agent-server via the embedded
// *Server.swarm field set by NewWithSwarm (see ADR §7.3). Direct callers
// outside of agent-server are out of scope. The surface here is the
// internal state machine, not the wire API.
package swarm
