// Package sse provides SSE wire-format primitives shared by protocol
// implementations. It is intentionally minimal: only the primitives that
// have failed in production (UTF-8 boundaries across chunked reads,
// concatenated JSON objects from proxies that drop the SSE event
// separator, and a hung trailing-event flush on EOF). Anything more
// specific to a single provider lives next to the provider's parser.
package sse
