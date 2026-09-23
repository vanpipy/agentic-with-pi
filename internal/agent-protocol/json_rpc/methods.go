package json_rpc

const (
	MethodPrompt       = "prompt"
	MethodResume       = "resume"
	MethodCancel       = "cancel"
	MethodPing         = "ping"
	MethodListSessions = "list_sessions"
)

const (
	EventThoughtStart = "thought_start"
	EventThoughtChunk = "thought_chunk"
	EventThoughtEnd   = "thought_end"
	EventTool         = "tool"
	EventObserve      = "observe"
	EventFinalAnswer  = "final_answer"
	EventError        = "error"
	EventCancelAck    = "cancelled"

	EventMessage       = "message"
	EventCustom        = "custom"
	EventCustomMessage = "custom_message"
)

type PromptParams struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

type ResumeParams struct {
	SessionID string `json:"session_id"`
}

type CancelParams struct{}

type SessionSummary struct {
	SessionID string `json:"session_id"`
	Model     string `json:"model"`
	StartedAt string `json:"started_at"`
	Events    int    `json:"events"`
}

type ListSessionsParams struct{}

type ListSessionsResult struct {
	Sessions []SessionSummary `json:"sessions"`
}
