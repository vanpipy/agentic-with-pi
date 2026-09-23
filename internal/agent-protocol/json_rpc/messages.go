package json_rpc

import "encoding/json"

type MessageContentPart struct {
	Type              string          `json:"type"`
	Text              string          `json:"text,omitempty"`
	Thinking          string          `json:"thinking,omitempty"`
	ThinkingSignature string          `json:"thinkingSignature,omitempty"`
	ID                string          `json:"id,omitempty"`
	Name              string          `json:"name,omitempty"`
	Intent            string          `json:"intent,omitempty"`
	Arguments         json.RawMessage `json:"arguments,omitempty"`
}

type Message struct {
	Role    string               `json:"role"`
	Content []MessageContentPart `json:"content"`
}

type MessageDetails struct {
	ToolName string `json:"toolName,omitempty"`
	Intent   string `json:"intent,omitempty"`
	Error    string `json:"error,omitempty"`
	Args     string `json:"args,omitempty"`
}

type UsageStats struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
	CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
}

type MessageEvent struct {
	ID         string          `json:"id"`
	ParentID   string          `json:"parentId,omitempty"`
	Timestamp  string          `json:"timestamp"`
	Message    Message         `json:"message"`
	StopReason string          `json:"stopReason,omitempty"`
	Usage      *UsageStats     `json:"usage,omitempty"`
	Details    *MessageDetails `json:"details,omitempty"`
}

type CustomEvent struct {
	ID         string          `json:"id"`
	ParentID   string          `json:"parentId,omitempty"`
	Timestamp  string          `json:"timestamp"`
	CustomType string          `json:"customType"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type CustomMessageEvent struct {
	ID         string          `json:"id"`
	ParentID   string          `json:"parentId,omitempty"`
	Timestamp  string          `json:"timestamp"`
	CustomType string          `json:"customType"`
	Content    string          `json:"content"`
	Display    bool            `json:"display,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
}
