package swarm

// ContextEntry is one version of a shared-context key. comm.share
// appends a new entry; comm.read returns the latest N entries (default
// last 1, configurable up to 100).
type ContextEntry struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	SharedBy string `json:"shared_by"` // session_id of the writer
	SharedAt string `json:"shared_at"` // RFC3339Nano
	Append   bool   `json:"append"`    // true if this appended to a prior value
}

// ShareParams is the params payload for MethodCommShare. Either
// Value (single-shot write) or AppendToValue (append to existing)
// must be set; both empty is a server-side error.
type ShareParams struct {
	SwarmID       string `json:"swarm_id"`
	FromSessionID string `json:"from_session"`
	Key           string `json:"key"`
	Value         string `json:"value,omitempty"`
	Append        bool   `json:"append,omitempty"` // true = append to existing value
	RequestNonce  string `json:"request_nonce,omitempty"`
}

// ReadParams is the params payload for MethodCommRead. Limit caps
// the number of historical entries returned (default 1, max 100).
type ReadParams struct {
	SwarmID       string `json:"swarm_id"`
	FromSessionID string `json:"from_session"`
	Key           string `json:"key"`
	Limit         int    `json:"limit,omitempty"` // default 1, max 100
}

// ReadResult is the data payload for EventCommReadResponse.
type ReadResult struct {
	Key     string         `json:"key"`
	Entries []ContextEntry `json:"entries"`
}
