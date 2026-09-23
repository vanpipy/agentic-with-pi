// Package json_rpc implements awp's server↔client wire format.
//
// A protocol is a binding of three things: the shape of a message, how
// bytes are exchanged on the wire, and who sends what when. This package
// binds those three for the awp process boundary.
//
// # Message shape
//
// We borrow the JSON-RPC 2.0 envelope (Request{jsonrpc, id, method, params}
// and Response{jsonrpc, id, ...}) as the message shape. The literal
// "jsonrpc":"2.0" field is kept for envelope compatibility but this
// package is NOT a JSON-RPC 2.0 implementation.
//
// # Byte exchange (framing)
//
// We do not use HTTP. Messages are newline-delimited JSON over a Unix
// domain socket: one JSON object per line, '\n' as the frame delimiter.
// codec.go's Marshal*/Read* functions own this layer.
//
// # Interaction semantics
//
// We deviate from JSON-RPC 2.0 in three ways:
//
//  1. The response field is not {result, error}; it is {event, data}.
//     Each request produces a stream of events.
//
//  2. A single request maps to multiple response messages (a server-
//     pushed event stream), not a single response. Callers detect end
//     of stream by Event == EventFinalAnswer (for prompt) or
//     Event == "session_resumed" (for resume).
//
//  3. Errors are delivered as ordinary events with Event == EventError,
//     not as the JSON-RPC error object.
//
// # Layout
//
//   - envelope.go  — Request / Response structs and the NewRequest constructor.
//   - methods.go   — Method* / Event* constants and the application-layer
//     parameter/result types they carry (PromptParams,
//     SessionSummary, etc.). These are the application
//     payload riding on this protocol; they are not part
//     of the wire format itself.
//   - codec.go     — Marshal*/Read* functions: the byte ↔ struct layer.
package json_rpc
