package swarm

// MemberStatus is the wire shape of a swarm member's current state,
// sent in EventSwarmStatus.data.members and EventSwarmMemberUpdate.data.
//
// This is the JSON-marshaled struct that crosses the UDS boundary.
// Internal persistence (MemberRecord in record.go) carries timestamps
// + working dir; those fields are intentionally omitted here because
// they don't change turn-to-turn.
type MemberStatus struct {
	SessionID        string          `json:"session_id"`
	FriendlyName     string          `json:"friendly_name,omitempty"`
	Status           LifecycleStatus `json:"status"`
	Detail           string          `json:"detail,omitempty"`
	TaskLabel        string          `json:"task_label,omitempty"`
	Role             Role            `json:"role,omitempty"`
	IsHeadless       bool            `json:"is_headless,omitempty"`
	LiveAttachments  int             `json:"live_attachments,omitempty"`
	StatusAgeSecs    *uint64         `json:"status_age_secs,omitempty"`
	LastActivitySecs *uint64         `json:"last_activity_age_secs,omitempty"`
	ReportBackToID   string          `json:"report_back_to_session_id,omitempty"`
	LatestReport     string          `json:"latest_completion_report,omitempty"`
	LatestReportTLDR string          `json:"latest_completion_report_tldr,omitempty"`
	TodoProgress     *[2]uint32      `json:"todo_progress,omitempty"`
	Runtime          MemberRuntime   `json:"runtime,omitempty"`
	ParentSessionID  string          `json:"parent_session_id,omitempty"`
	SwarmID          string          `json:"swarm_id,omitempty"`
}

// MemberRuntime describes the model / provider / effort that the
// member is currently running. Updated by heartbeat (S2+).
type MemberRuntime struct {
	Model       string  `json:"model,omitempty"`
	Provider    string  `json:"provider,omitempty"`
	AuthMethod  string  `json:"auth_method,omitempty"`
	Effort      string  `json:"effort,omitempty"`
	ElapsedSecs *uint64 `json:"elapsed_secs,omitempty"`
}

// TodoItem represents a single entry in the member's todo list, as
// surfaced by comm.summary. ToolIntents describe what the agent is
// about to do for this todo (e.g. "read file foo.go").
type TodoItem struct {
	Content     string       `json:"content"`
	Status      string       `json:"status"` // "pending" | "in_progress" | "completed"
	ToolIntents []ToolIntent `json:"tool_intents,omitempty"`
}

// ToolIntent describes a tool call the agent intends to execute for
// a todo. Progress is optional and only set for long-running tools.
type ToolIntent struct {
	ToolName  string        `json:"tool_name"`
	Intent    string        `json:"intent"`
	Status    string        `json:"status"` // "running" | "completed" | "error"
	Progress  *ToolProgress `json:"progress,omitempty"`
	StartedAt string        `json:"started_at,omitempty"` // RFC3339Nano
	EndedAt   string        `json:"ended_at,omitempty"`   // RFC3339Nano
}

// ToolProgress is a generic counter for long-running tools (e.g.
// grep over N files, currently on file K).
type ToolProgress struct {
	Current uint64 `json:"current"`
	Total   uint64 `json:"total"`
	Unit    string `json:"unit,omitempty"` // e.g. "files", "tokens", "bytes"
}
