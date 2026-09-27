package llm_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestTypesMore_ToolChoiceValidateInvalidMode(t *testing.T) {
	cases := []llm.ToolChoice{
		{Mode: "bogus"},
		{Mode: "random_mode"},
	}
	for _, tc := range cases {
		err := tc.Validate()
		if err == nil {
			t.Errorf("Validate(%q) = nil, want error", tc.Mode)
		}
		if !strings.Contains(err.Error(), "invalid") {
			t.Errorf("err = %q, want it to mention 'invalid'", err)
		}
	}
}

func TestTypesMore_ToolChoiceValidateToolRequiresName(t *testing.T) {
	tc := llm.ToolChoice{Mode: "tool", Name: ""}
	err := tc.Validate()
	if err == nil {
		t.Fatal("Validate(tool, \"\") = nil, want error")
	}
	if !strings.Contains(err.Error(), "requires name") {
		t.Errorf("err = %q, want it to mention 'requires name'", err)
	}
}

func TestToolChoiceValidateAllValidModes(t *testing.T) {
	for _, mode := range []string{"auto", "any", "none", "tool"} {
		tc := llm.ToolChoice{Mode: mode, Name: "x"}
		if err := tc.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", mode, err)
		}
	}
}

func TestToolDefValidateInvalidType(t *testing.T) {
	td := llm.ToolDef{Type: "not_function", Function: llm.FunctionDef{Name: "x"}}
	err := td.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("err = %q, want 'unsupported'", err)
	}
}

func TestTypesMore_ToolDefValidateEmptyName(t *testing.T) {
	td := llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: ""}}
	err := td.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("err = %q, want 'required'", err)
	}
}

func TestToolDefValidateParametersNotMarshalable(t *testing.T) {
	td := llm.ToolDef{
		Type:     "function",
		Function: llm.FunctionDef{Name: "x", Parameters: make(chan int)},
	}
	err := td.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for non-JSON parameters")
	}
	if !strings.Contains(err.Error(), "JSON-serializable") {
		t.Errorf("err = %q, want 'JSON-serializable'", err)
	}
}

func TestToolDefValidateParametersNotObject(t *testing.T) {
	td := llm.ToolDef{
		Type:     "function",
		Function: llm.FunctionDef{Name: "x", Parameters: "a string not object"},
	}
	err := td.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for non-object parameters")
	}
	if !strings.Contains(err.Error(), "JSON object") {
		t.Errorf("err = %q, want 'JSON object'", err)
	}
}

func TestToolDefValidateValidObjectParams(t *testing.T) {
	td := llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:       "x",
			Parameters: map[string]any{"type": "object"},
		},
	}
	if err := td.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestToolDefValidateEmptyTypeAllowed(t *testing.T) {
	td := llm.ToolDef{Type: "", Function: llm.FunctionDef{Name: "x"}}
	if err := td.Validate(); err != nil {
		t.Errorf("Validate() with empty type = %v, want nil", err)
	}
}

func TestMessageJSONRoundTrip(t *testing.T) {
	msg := llm.Message{
		Role:         "assistant",
		Content:      "hello",
		Reasoning:    "thought",
		ReasoningSig: "sig",
		ToolCallID:   "toolu_1",
		ToolCalls: []llm.ToolCall{{
			ID:       "c1",
			Type:     "function",
			Function: llm.FunctionCall{Name: "bash", Arguments: `{"cmd":"ls"}`},
		}},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var back llm.Message
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Role != msg.Role || back.Content != msg.Content || back.Reasoning != msg.Reasoning {
		t.Errorf("round-trip mismatch: got %+v, want %+v", back, msg)
	}
	if len(back.ToolCalls) != 1 || back.ToolCalls[0].ID != "c1" {
		t.Errorf("ToolCalls mismatch: %+v", back.ToolCalls)
	}
}

func TestStreamChoiceJSONRoundTrip(t *testing.T) {
	sc := llm.StreamChoice{
		Index:        1,
		Delta:        llm.Message{Content: "hi"},
		FinishReason: llm.FinishReasonToolUse,
	}
	b, err := json.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	var back llm.StreamChoice
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Index != 1 || back.Delta.Content != "hi" || back.FinishReason != llm.FinishReasonToolUse {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestModelZeroValueJSON(t *testing.T) {
	m := llm.Model{}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back llm.Model
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back != m {
		t.Errorf("round-trip mismatch: got %+v, want %+v", back, m)
	}
}
