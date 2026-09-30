package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/vanpiyp/awp/internal/llm"
)

const defaultAnthropicMaxTokens = 4096

type AnthropicRequest struct {
	Model         string
	Messages      []AnthropicMessage
	System        []llm.ContentBlock
	Tools         []AnthropicTool
	ToolChoice    *AnthropicToolChoice
	Thinking      *AnthropicThinking
	OutputConfig  *AnthropicOutputConfig
	MaxTokens     int
	Temperature   *float64
	Stream        bool
	Metadata      map[string]string
	StopSequences []string
	CacheControl  *llm.CacheControl
}

// AnthropicToolChoice mirrors the Anthropic tool_choice field. Mode
// is "auto" / "any" / "tool"; when Mode == "tool", Name selects the
// required tool.
type AnthropicToolChoice struct {
	Mode string
	Name string
}

// AnthropicThinking is the Anthropic extended-thinking envelope.
// Type is "enabled" (with a BudgetTokens cap) or "adaptive" (the
// model decides its own budget; BudgetTokens is unused and
// omitted). Set Type to "adaptive" when the model accepts adaptive
// thinking per anthropic_caps.ReasoningCaps(model).
type AnthropicThinking struct {
	Type         string
	BudgetTokens int
}

// AnthropicOutputConfig carries the per-request `output_config` block
// the modern Messages API uses to control reasoning effort. Effort is
// one of "none", "low", "medium", "high", "xhigh", "max" — the
// allowed values are filtered per-model by
// anthropic_caps.AvailableReasoningEfforts(model).
type AnthropicOutputConfig struct {
	Effort string
}

type AnthropicMessage struct {
	Role    llm.Role
	Content []llm.ContentBlock
}

type AnthropicTool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type wireCacheControl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type wireSystemBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
}

type wireTextBlock struct {
	Type         string            `json:"type"`
	Text         string            `json:"text"`
	CacheControl *wireCacheControl `json:"cache_control,omitempty"`
}

type wireImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type wireImageBlock struct {
	Type   string          `json:"type"`
	Source wireImageSource `json:"source"`
}

type wireToolUseBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type wireToolResultBlock struct {
	Type      string            `json:"type"`
	ToolUseID string            `json:"tool_use_id"`
	Content   []json.RawMessage `json:"content"`
	IsError   bool              `json:"is_error,omitempty"`
}

type wireThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type wireMessage struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type wireBody struct {
	Model         string            `json:"model"`
	Messages      []wireMessage     `json:"messages"`
	System        []wireSystemBlock `json:"system,omitempty"`
	Tools         []wireTool        `json:"tools,omitempty"`
	ToolChoice    *wireToolChoice   `json:"tool_choice,omitempty"`
	Thinking      *wireThinking     `json:"thinking,omitempty"`
	OutputConfig  *wireOutputConfig `json:"output_config,omitempty"`
	MaxTokens     int               `json:"max_tokens"`
	Temperature   *float64          `json:"temperature,omitempty"`
	Stream        bool              `json:"stream"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type wireToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type wireThinking struct {
	Type string `json:"type"`
	// BudgetTokens is only set when Type is "enabled". Adaptive
	// thinking omits it (the model decides its own budget).
	BudgetTokens int `json:"budget_tokens,omitempty"`
}

type wireOutputConfig struct {
	Effort string `json:"effort"`
}

func cacheControlToWire(cc *llm.CacheControl) *wireCacheControl {
	if cc == nil {
		return nil
	}
	return &wireCacheControl{Type: cc.Type, TTL: cc.TTL}
}

func marshalContentBlock(b llm.ContentBlock) (json.RawMessage, error) {
	switch v := b.(type) {
	case llm.ContentText:
		wt := wireTextBlock{Type: "text", Text: v.Text, CacheControl: cacheControlToWire(v.CacheControl)}
		out, err := json.Marshal(wt)
		if err != nil {
			return nil, fmt.Errorf("marshal ContentText: %w", err)
		}
		return out, nil
	case llm.ContentImage:
		wi := wireImageBlock{
			Type:   "image",
			Source: wireImageSource{Type: "base64", MediaType: v.MediaType, Data: v.Data},
		}
		out, err := json.Marshal(wi)
		if err != nil {
			return nil, fmt.Errorf("marshal ContentImage: %w", err)
		}
		return out, nil
	case llm.ContentToolUse:
		input := v.Input
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		var asObject json.RawMessage
		if json.Valid(input) && len(input) > 0 && input[0] == '{' {
			asObject = input
		} else {
			wrapped, err := json.Marshal(map[string]json.RawMessage{"_raw": input})
			if err != nil {
				return nil, fmt.Errorf("marshal ContentToolUse: %w", err)
			}
			asObject = wrapped
		}
		wt := wireToolUseBlock{Type: "tool_use", ID: v.ID, Name: v.Name, Input: asObject}
		out, err := json.Marshal(wt)
		if err != nil {
			return nil, fmt.Errorf("marshal ContentToolUse: %w", err)
		}
		return out, nil
	case llm.ContentToolResult:
		inner := make([]json.RawMessage, 0, len(v.Content))
		for _, c := range v.Content {
			raw, err := marshalContentBlock(c)
			if err != nil {
				return nil, err
			}
			inner = append(inner, raw)
		}
		wt := wireToolResultBlock{
			Type:      "tool_result",
			ToolUseID: v.ToolUseID,
			Content:   inner,
			IsError:   v.IsError,
		}
		out, err := json.Marshal(wt)
		if err != nil {
			return nil, fmt.Errorf("marshal ContentToolResult: %w", err)
		}
		return out, nil
	case llm.ContentThinking:
		wt := wireThinkingBlock{Type: "thinking", Thinking: v.Text, Signature: v.Signature}
		out, err := json.Marshal(wt)
		if err != nil {
			return nil, fmt.Errorf("marshal ContentThinking: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported content block %T", b)
	}
}

func BuildAnthropicRequest(req AnthropicRequest) (json.RawMessage, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("anthropic request: model is required")
	}
	if req.MaxTokens <= 0 {
		return nil, fmt.Errorf("anthropic request: max_tokens must be > 0 (Anthropic requires explicit max_tokens)")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("anthropic request: at least one message required")
	}

	body := wireBody{
		Model:         req.Model,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		Stream:        req.Stream,
		StopSequences: req.StopSequences,
		Metadata:      req.Metadata,
	}

	body.Messages = make([]wireMessage, 0, len(req.Messages))
	for i, m := range req.Messages {
		blocks := make([]json.RawMessage, 0, len(m.Content))
		for _, c := range m.Content {
			raw, err := marshalContentBlock(c)
			if err != nil {
				return nil, fmt.Errorf("messages[%d]: %w", i, err)
			}
			blocks = append(blocks, raw)
		}
		body.Messages = append(body.Messages, wireMessage{Role: string(m.Role), Content: blocks})
	}

	if len(req.System) > 0 {
		body.System = make([]wireSystemBlock, 0, len(req.System))
		for _, c := range req.System {
			tb, ok := c.(llm.ContentText)
			if !ok {
				return nil, fmt.Errorf("system content blocks must be ContentText, got %T", c)
			}
			ws := wireSystemBlock{
				Type:         "text",
				Text:         tb.Text,
				CacheControl: cacheControlToWire(tb.CacheControl),
			}
			body.System = append(body.System, ws)
		}
	}

	if len(req.Tools) > 0 {
		body.Tools = make([]wireTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			schema := t.InputSchema
			if len(schema) == 0 {
				schema = json.RawMessage(`{}`)
			}
			body.Tools = append(body.Tools, wireTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: schema,
			})
		}
	}

	if req.ToolChoice != nil {
		body.ToolChoice = &wireToolChoice{Type: req.ToolChoice.Mode, Name: req.ToolChoice.Name}
	}
	if req.Thinking != nil {
		w := &wireThinking{Type: req.Thinking.Type}
		// Only emit budget_tokens for the legacy "enabled" envelope;
		// adaptive thinking leaves the budget up to the model and
		// the API rejects budget_tokens alongside it.
		if req.Thinking.Type == "enabled" {
			w.BudgetTokens = req.Thinking.BudgetTokens
		}
		body.Thinking = w
	}
	if req.OutputConfig != nil {
		body.OutputConfig = &wireOutputConfig{Effort: req.OutputConfig.Effort}
	}

	return json.Marshal(body)
}

func MapToAnthropicRequest(llmReq llm.ChatRequest) (AnthropicRequest, error) {
	out := AnthropicRequest{
		Model:         llmReq.Model,
		Stream:        llmReq.Stream,
		StopSequences: llmReq.StopSequences,
	}
	if llmReq.MaxTokens > 0 {
		out.MaxTokens = llmReq.MaxTokens
	} else {
		out.MaxTokens = defaultAnthropicMaxTokens
	}
	if llmReq.Temperature != 0 {
		t := float64(llmReq.Temperature)
		out.Temperature = &t
	}

	if len(llmReq.Messages) > 0 {
		out.Messages = make([]AnthropicMessage, 0, len(llmReq.Messages))
	}
	for _, m := range llmReq.Messages {
		role := llm.Role(m.Role)
		if role != "user" && role != "assistant" && role != "system" && role != "tool" {
			continue
		}
		if role == "system" {
			if m.Content != "" {
				out.System = append(out.System, llm.ContentText{Text: m.Content})
			}
			continue
		}
		if role == "tool" {
			blocks := []llm.ContentBlock{llm.ContentToolResult{
				ToolUseID: m.ToolCallID,
				Content:   []llm.ContentBlock{llm.ContentText{Text: m.Content}},
			}}
			out.Messages = append(out.Messages, AnthropicMessage{Role: "user", Content: blocks})
			continue
		}
		blocks := make([]llm.ContentBlock, 0, len(m.ToolCalls)+2)

		if m.Reasoning != "" {
			blocks = append(blocks, llm.ContentThinking{Text: m.Reasoning, Signature: m.ReasoningSig})
		}
		if m.Content != "" {
			blocks = append(blocks, llm.ContentText{Text: m.Content})
		}
		for _, tc := range m.ToolCalls {
			argBytes := []byte(tc.Function.Arguments)
			if len(argBytes) == 0 {
				argBytes = json.RawMessage(`{}`)
			}
			blocks = append(blocks, llm.ContentToolUse{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: argBytes,
			})
		}
		out.Messages = append(out.Messages, AnthropicMessage{Role: role, Content: blocks})
	}

	if llmReq.ToolChoice != nil {
		out.ToolChoice = &AnthropicToolChoice{Mode: llmReq.ToolChoice.Mode, Name: llmReq.ToolChoice.Name}
	}

	if len(llmReq.Tools) > 0 {
		out.Tools = make([]AnthropicTool, 0, len(llmReq.Tools))
		for _, td := range llmReq.Tools {
			var schema json.RawMessage
			if td.Function.Parameters != nil {
				raw, err := json.Marshal(td.Function.Parameters)
				if err != nil {
					return AnthropicRequest{}, fmt.Errorf("tool %q parameters: %w", td.Function.Name, err)
				}
				schema = raw
			}
			out.Tools = append(out.Tools, AnthropicTool{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				InputSchema: schema,
			})
		}
	}

	return out, nil
}
