// Package ipc wraps the awp process-boundary IPC.
//
// This package is not a protocol implementation. It sits below
// internal/agent-protocol/json_rpc/ in the stack: ipc decides how
// bytes get from the server process to the TUI client process
// (Unix Domain Socket per POSIX), while json_rpc decides how those
// bytes are framed into messages and what those messages mean.
//
// # Capabilities
//
// Listen / Dial / IsRunning all take and return socket file paths
// (no protocol-level state). Listen returns net.Listener; callers
// that need the original path assert it via interface{ Path() string }
// — the same pattern the stdlib uses for *net.UnixListener extensions.
//
// # Layout
//
//   - unix_socket.go — POSIX Unix Domain Socket implementation:
//     Listen, Dial, IsRunning, and the unixListener
//     wrapper that adds Path() and on-close cleanup
//     of the socket file.
//   - pid.go          — PID file write/read/remove plus a kill -0
//     liveness probe; used by cmd/awp/main.go to
//     gate against an existing daemon before
//     starting a new one.
package ipc
