package llm

import "encoding/json"

type Message struct {
	Role         string     `json:"role"`
	Content      string     `json:"content,omitzero"`
	Reasoning    string     `json:"reasoning_content,omitzero"`
	ReasoningSig string     `json:"reasoning_signature,omitzero"`
	ToolCallID   string     `json:"tool_call_id,omitzero"`
	ToolCalls    []ToolCall `json:"tool_calls,omitzero"`
}

type ToolChoice struct {
	Mode string
	Name string
}

func (tc ToolChoice) Validate() error {
	switch tc.Mode {
	case "auto", "any", "none", "tool":
	default:
		return &Error{Kind: ErrorKindClient, Message: "tool_choice mode " + tc.Mode + " invalid (must be auto/any/none/tool)"}
	}
	if tc.Mode == "tool" && tc.Name == "" {
		return &Error{Kind: ErrorKindClient, Message: "tool_choice mode \"tool\" requires name"}
	}
	return nil
}

type ChatRequest struct {
	Model         string      `json:"model"`
	Messages      []Message   `json:"messages"`
	Tools         []ToolDef   `json:"tools,omitzero"`
	ToolChoice    *ToolChoice `json:"tool_choice,omitzero"`
	StopSequences []string    `json:"stop_sequences,omitzero"`
	Temperature   float32     `json:"temperature,omitzero"`
	MaxTokens     int         `json:"max_tokens,omitzero"`
	Stream        bool        `json:"stream"`
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
	Usage   *Usage         `json:"usage,omitzero"`
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

func (t ToolDef) Validate() error {
	if t.Type != "" && t.Type != "function" {
		return &Error{Kind: ErrorKindClient, Message: "tool type " + t.Type + " unsupported (only \"function\")"}
	}
	if t.Function.Name == "" {
		return &Error{Kind: ErrorKindClient, Message: "tool function name is required"}
	}
	if t.Function.Parameters != nil {
		b, err := json.Marshal(t.Function.Parameters)
		if err != nil {
			return &Error{Kind: ErrorKindClient, Message: "tool parameters not JSON-serializable", Cause: err}
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return &Error{Kind: ErrorKindClient, Message: "tool parameters must be a JSON object"}
		}
	}
	return nil
}

func validateTools(tools []ToolDef) error {
	for i, t := range tools {
		if err := t.Validate(); err != nil {
			return &Error{Kind: ErrorKindClient, Message: "tools[" + itoa(i) + "]: " + err.Error(), Cause: err}
		}
	}
	return nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s []byte
	for i > 0 {
		s = append([]byte{byte('0' + i%10)}, s...)
		i /= 10
	}
	return string(s)
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
