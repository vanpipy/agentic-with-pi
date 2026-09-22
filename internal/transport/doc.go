// Package transport wraps the awp process-boundary IPC.
//
// This package is not a protocol implementation. It sits below
// internal/protocol/json_rpc/ in the stack: transport decides how
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
//                     Listen, Dial, IsRunning, and the unixListener
//                     wrapper that adds Path() and on-close cleanup
//                     of the socket file.
package transport
