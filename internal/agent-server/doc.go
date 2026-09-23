// Package server runs the awp server process: it accepts client
// connections on a Unix domain socket, dispatches JSON-RPC requests
// from internal/protocol/json_rpc to handler methods, drives the
// agent, and persists session state across prompts.
//
// # Layout
//
//   - agentserver.go    — Server struct, New, and the per-session JSONL
//     store registry (stores + storesMu). Also
//     SocketPath metadata access lives on the Server.
//
//   - serve.go     — Server lifecycle on the wire: Serve (accept
//     loop), Shutdown (graceful), handleConn (per
//     connection read loop), and the per-connection
//     cancel registry (registerConnCancel /
//     popConnCancel). SocketPath() moved here from
//     the old ipc.go.
//
//   - handler.go   — JSON-RPC method dispatch: ping, prompt, resume,
//     cancel, list_sessions, plus the
//     agentcore.Event ↔ wire event mapping and helpers
//     such as loadResumeHistory and
//     collectSessionSummaries.
//
//   - sessions.go  — Two layers of session state: Store (JSONL on
//     disk, for durable history and resume) and
//     sessionStateStore (in-memory map, for cross-
//     request message snapshots within a session).
//
// agentserver.go is what the server IS; serve.go is what it DOES at
// runtime. This mirrors the Kubernetes / etcd / Consul convention
// of keeping the struct definition separate from the lifecycle
// entry points.
package agentserver
