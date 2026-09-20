package protocol

import "encoding/json"

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data,omitempty"`
}

const (
	MethodPrompt = "prompt"
	MethodResume = "resume"
	MethodCancel = "cancel"
	MethodPing   = "ping"
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
)

type PromptParams struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

type ResumeParams struct {
	SessionID string `json:"session_id"`
}

type CancelParams struct{}
