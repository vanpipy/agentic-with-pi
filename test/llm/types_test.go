package llm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestFinishReasonStringAllValues(t *testing.T) {
	cases := []struct {
		reason llm.FinishReason
		want   string
	}{
		{llm.FinishReasonUnknown, "unknown"},
		{llm.FinishReasonStop, "stop"},
		{llm.FinishReasonLength, "length"},
		{llm.FinishReasonToolUse, "tool_use"},
		{llm.FinishReasonContentFilter, "content_filter"},
		{llm.FinishReasonError, "error"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.reason.String(); got != tc.want {
				t.Errorf("FinishReason(%d).String() = %q, want %q", int(tc.reason), got, tc.want)
			}
		})
	}
}

func TestFinishReasonStringOutOfRange(t *testing.T) {
	got := llm.FinishReason(999).String()
	if got != "unknown" {
		t.Errorf("out-of-range FinishReason.String() = %q, want %q", got, "unknown")
	}
}

func TestChatRequestJSONRoundTrip(t *testing.T) {
	req := llm.ChatRequest{
		Model: "claude-opus-4-7",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
		},
		Tools: []llm.ToolDef{
			{Type: "function", Function: llm.FunctionDef{Name: "bash"}},
		},
		ToolChoice: &llm.ToolChoice{Mode: "auto"},
		Stream:     true,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var back llm.ChatRequest
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Model != req.Model || back.Stream != req.Stream {
		t.Errorf("round trip mismatch: %+v", back)
	}
	if len(back.Messages) != 1 || back.Messages[0].Role != "user" {
		t.Errorf("messages mismatch: %+v", back.Messages)
	}
	if len(back.Tools) != 1 || back.Tools[0].Function.Name != "bash" {
		t.Errorf("tools mismatch: %+v", back.Tools)
	}
}

func TestStreamEventZeroValue(t *testing.T) {
	var ev llm.StreamEvent
	if ev != nil {
		t.Errorf("zero StreamEvent = %+v, want nil", ev)
	}
}

func TestStreamEventRollbackField(t *testing.T) {
	var ev llm.StreamEvent = llm.EventRetryRollback{Attempt: 1, Max: 3}
	if _, ok := ev.(llm.EventRetryRollback); !ok {
		t.Errorf("ev = %T, want llm.EventRetryRollback", ev)
	}
}

func TestUsageTotalTokens(t *testing.T) {
	u := llm.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}
	if u.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, want 30", u.TotalTokens)
	}
}

func TestToolCallJSONShape(t *testing.T) {
	tc := llm.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      "bash",
			Arguments: `{"cmd":"ls"}`,
		},
	}
	b, err := json.Marshal(tc)
	if err != nil {
		t.Fatal(err)
	}
	var back llm.ToolCall
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back != tc {
		t.Errorf("round trip mismatch: got %+v, want %+v", back, tc)
	}
}
