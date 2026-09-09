package llm

type Message struct {
	Role         string     `json:"role"`
	Content      string     `json:"content,omitzero"`
	Reasoning    string     `json:"reasoning_content,omitempty"`
	ReasoningSig string     `json:"reasoning_signature,omitempty"`
	ToolCallID   string     `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Temperature float32   `json:"temperature,omitzero"`
	MaxTokens   int       `json:"max_tokens,omitzero"`
	Stream      bool      `json:"stream"`
}

type FinishReason int

const (
	FinishReasonUnknown FinishReason = iota
	FinishReasonStop
	FinishReasonLength
	FinishReasonToolUse
	FinishReasonContentFilter
	FinishReasonError
)

func (f FinishReason) String() string {
	switch f {
	case FinishReasonStop:
		return "stop"
	case FinishReasonLength:
		return "length"
	case FinishReasonToolUse:
		return "tool_use"
	case FinishReasonContentFilter:
		return "content_filter"
	case FinishReasonError:
		return "error"
	default:
		return "unknown"
	}
}

type Choice struct {
	Index        int          `json:"index"`
	Message      Message      `json:"message"`
	FinishReason FinishReason `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type StreamChoice struct {
	Index        int          `json:"index"`
	Delta        Message      `json:"delta"`
	FinishReason FinishReason `json:"finish_reason"`
}

type StreamChunk struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Choices []StreamChoice `json:"choices"`
}

type StreamEvent struct {
	Chunk *StreamChunk
	Err   error
}

type FunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitzero"`
}

type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type Model struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Vendor            string `json:"vendor"`
	MaxContextTokens  int    `json:"max_context_tokens"`
	MaxOutputTokens   int    `json:"max_output_tokens"`
	SupportsTool      bool   `json:"supports_tool"`
	SupportsVision    bool   `json:"supports_vision"`
	SupportsStreaming bool   `json:"supports_streaming"`
	SupportsReasoning bool   `json:"supports_reasoning"`
}
