// Package swarm defines the wire types for multi-session agent
// coordination over the existing JSON-RPC 2.0 UDS transport.
//
// Scope:
//
//   - 29 request methods (comm.spawn, comm.message, comm.await_members,
//     comm.seed_graph, comm.complete_node, ...) corresponding 1:1 to
//     ~/Project/jcode's Request::Comm* enum.
//   - 25 ServerEvent constants for swarm.* / comm.*_response streams.
//   - Sealed-type pattern for every enum (LifecycleStatus, Role,
//     PlanMode, NodeKind, NodeStatus, NodeOrigin, TaskControlAction,
//     DeliveryMode, AwaitMode, ConfidenceLevel): package-private struct
//     field + exported package vars + ParseXxx constructor with Other
//     fallback. Type-switches are exhaustive by construction.
//   - Algorithmic helpers (cycle detect, ready-set, TLDR validation,
//     terminal-status set, summarize, action-allows-status, etc.)
//     consumed by internal/agent-server/swarm in S2+.
//
// This package is the wire contract. Field names, method names, and
// event names are stable: clients in ~/Project/awp/cmd/awp,
// internal/agent-client, internal/agent-core, and external tools
// depend on them. See ADR docs/architecture/swarm-design.md §15 (D9).
//
// Server-internal helpers (state machine, DAG engine, broadcast) live
// in internal/agent-server/swarm and may be refactored freely under
// AGENTS.md golden rule #1 (no backward compatibility).
package swarm

// stable: this package is the JSON-RPC wire contract for swarm methods.
// Field tags, method constants, event constants, and the sealed-type
// enum values (LifecycleStatus / Role / PlanMode / NodeKind /
// NodeStatus / NodeOrigin / TaskControlAction / DeliveryMode /
// AwaitMode / ConfidenceLevel) are frozen at ship time. Additive
// changes (new fields, new enum values) are allowed but renaming or
// removing anything below breaks the wire.
const _ = "swarm stable:wire-contract"
