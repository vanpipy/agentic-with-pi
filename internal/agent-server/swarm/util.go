// Package swarm — small helpers shared by register / channel / message.
package swarm

import (
	"encoding/json"
	"sort"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// newMessageID returns a uuidv7 string suitable for the ChannelMessage
// MessageID field. The uuidv7 generator lives in
// internal/agent-protocol/json_rpc so both server and client produce
// monotonic ids. Server-side this gives subscribers a stable sort key
// even when Publish fires concurrently.
func newMessageID() string {
	return jsonrpc.NewV7()
}

// marshalAny is the package-internal helper that JSON-encodes v into
// a RawMessage for the jsonrpc.Response Data field. It panics on
// marshal failure: the only callers marshal wire-shape structs that
// are validated by the swarmproto package at construction time, so a
// marshal failure indicates a programmer error in swarmproto.
func marshalAny(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("swarm: marshal " + err.Error())
	}
	return b
}

// sortChannelInfos sorts ChannelInfos in place by channel name.
// Stable so callers can rely on deterministic output for tests and
// wire snapshots.
func sortChannelInfos(s []swarmproto.ChannelInfo) {
	sort.Slice(s, func(i, j int) bool { return s[i].Channel < s[j].Channel })
}

// sortStrings sorts a slice of strings in place. Used for sorted
// ChannelMembers output and anywhere else a deterministic session-id
// list is needed.
func sortStrings(s []string) {
	sort.Strings(s)
}
