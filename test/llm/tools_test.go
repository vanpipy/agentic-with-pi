package llm_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestToolDefValidateOK(t *testing.T) {
	td := llm.ToolDef{
		Type: "function",
		Function: llm.FunctionDef{
			Name:        "get_weather",
			Description: "Get the weather",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"location": map[string]any{"type": "string"},
				},
			},
		},
	}
	if err := td.Validate(); err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestToolDefValidateEmptyTypeIsOK(t *testing.T) {
	td := llm.ToolDef{Function: llm.FunctionDef{Name: "x"}}
	if err := td.Validate(); err != nil {
		t.Errorf("Validate failed for empty Type (treated as function): %v", err)
	}
}

func TestToolDefValidateBadType(t *testing.T) {
	td := llm.ToolDef{Type: "unknown", Function: llm.FunctionDef{Name: "x"}}
	err := td.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("err = %q, want mentions type", err.Error())
	}
}

func TestToolDefValidateEmptyName(t *testing.T) {
	td := llm.ToolDef{Type: "function", Function: llm.FunctionDef{}}
	err := td.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("err = %q, want mentions name", err.Error())
	}
}

func TestToolDefValidateNonJSONSchemaParams(t *testing.T) {
	td := llm.ToolDef{
		Type:     "function",
		Function: llm.FunctionDef{Name: "x", Parameters: "not an object"},
	}
	err := td.Validate()
	if err == nil {
		t.Fatal("expected error for non-JSON-object params")
	}
}

func TestToolDefValidateNilParamsOK(t *testing.T) {
	td := llm.ToolDef{Type: "function", Function: llm.FunctionDef{Name: "x"}}
	if err := td.Validate(); err != nil {
		t.Errorf("Validate failed with nil params: %v", err)
	}
}

func TestToolChoiceValidateModes(t *testing.T) {
	for _, mode := range []string{"auto", "any", "none"} {
		tc := llm.ToolChoice{Mode: mode}
		if err := tc.Validate(); err != nil {
			t.Errorf("mode %q: %v", mode, err)
		}
	}
}

func TestToolChoiceValidateToolRequiresName(t *testing.T) {
	tc := llm.ToolChoice{Mode: "tool"}
	if err := tc.Validate(); err == nil {
		t.Error("expected error for tool mode without name")
	}
	tc = llm.ToolChoice{Mode: "tool", Name: "foo"}
	if err := tc.Validate(); err != nil {
		t.Errorf("tool+name should be valid: %v", err)
	}
}

func TestToolChoiceValidateInvalidMode(t *testing.T) {
	tc := llm.ToolChoice{Mode: "bogus"}
	if err := tc.Validate(); err == nil {
		t.Error("expected error for invalid mode")
	}
}
