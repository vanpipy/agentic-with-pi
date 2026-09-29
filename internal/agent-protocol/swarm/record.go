package swarm

// MemberRecord is the persistence-layer shape of a swarm member.
// Internal/agent-server writes one EntrySwarmMemberUpdate line per
// state transition; on startup these lines are replayed into the
// in-memory State to reconstruct the swarm.
//
// All timestamps are RFC3339Nano UTC strings (no Go time.Time on the
// wire or on disk — keeps the persistence layer string-only and
// immune to monotonic-clock skew).
type MemberRecord struct {
	SessionID        string          `json:"session_id"`
	SwarmID          string          `json:"swarm_id,omitempty"`
	WorkingDir       string          `json:"working_dir,omitempty"`
	SwarmEnabled     bool            `json:"swarm_enabled"`
	Status           LifecycleStatus `json:"status"`
	Detail           string          `json:"detail,omitempty"`
	TaskLabel        string          `json:"task_label,omitempty"`
	FriendlyName     string          `json:"friendly_name,omitempty"`
	ReportBackToID   string          `json:"report_back_to_session_id,omitempty"`
	LatestReport     string          `json:"latest_completion_report,omitempty"`
	LatestReportTLDR string          `json:"latest_completion_report_tldr,omitempty"`
	Role             Role            `json:"role"`
	IsHeadless       bool            `json:"is_headless"`
	ParentSessionID  string          `json:"parent_session_id,omitempty"`
	CreatedAt        string          `json:"created_at"`                  // RFC3339Nano
	LastUpdatedAt    string          `json:"last_updated_at"`             // RFC3339Nano
	LastHeartbeatAt  string          `json:"last_heartbeat_at,omitempty"` // RFC3339Nano
}

// ChannelSubscriptionRecord is the persistence-layer shape of a
// channel subscription. Internal/agent-server writes
// EntrySwarmChannelSub / EntrySwarmChannelUnsub lines on every
// subscribe / unsubscribe; replay rebuilds the ChannelIndex.
type ChannelSubscriptionRecord struct {
	SessionID    string `json:"session_id"`
	SwarmID      string `json:"swarm_id"`
	Channel      string `json:"channel"`
	SubscribedAt string `json:"subscribed_at"` // RFC3339Nano
}

// CompletionReportRecord is the persistence-layer shape of a
// comm.report call. Internal/agent-server writes one
// EntrySwarmCompletionReport per call; replay updates LatestReport
// + LatestReportTLDR on the corresponding MemberRecord.
type CompletionReportRecord struct {
	SessionID  string          `json:"session_id"`
	SwarmID    string          `json:"swarm_id"`
	Body       string          `json:"body"`
	Validation string          `json:"validation,omitempty"`
	FollowUp   string          `json:"follow_up,omitempty"`
	TLDR       string          `json:"tldr,omitempty"`
	Status     LifecycleStatus `json:"status"`
	ReportedAt string          `json:"reported_at"` // RFC3339Nano
}

// SharedContextRecord is the persistence-layer shape of a shared
// context entry (comm.share). Loaded into State.sharedCtx on
// startup.
type SharedContextRecord struct {
	SwarmID  string `json:"swarm_id"`
	Key      string `json:"key"`
	Value    string `json:"value"`
	Append   bool   `json:"append"`    // true if this appended to an existing value
	SharedBy string `json:"shared_by"` // session_id of the writer
	SharedAt string `json:"shared_at"` // RFC3339Nano
}
