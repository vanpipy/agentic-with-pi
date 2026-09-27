package tools_test

import (
	"encoding/json"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/tools"
)

func TestReadAftSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.ReadAftForTest(nil))

	if _, ok := props["intent"]; !ok {
		t.Error("read schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("read schema missing 'accept_large_output' field")
	}
}

func TestWriteAftSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.WriteAftForTest(nil))

	if _, ok := props["intent"]; !ok {
		t.Error("write schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("write schema missing 'accept_large_output' field")
	}
}

func TestBashAftSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.BashAftForTest(nil))

	if _, ok := props["intent"]; !ok {
		t.Error("bash schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("bash schema missing 'accept_large_output' field")
	}
}

func TestEditAftSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.EditAftForTest(nil))

	if _, ok := props["intent"]; !ok {
		t.Error("edit schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("edit schema missing 'accept_large_output' field")
	}
}

func TestGoReadSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.ReadFile("/tmp", tools.FileOptions{}))

	if _, ok := props["intent"]; !ok {
		t.Error("go read schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("go read schema missing 'accept_large_output' field")
	}
}

func TestGoWriteSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.WriteFile("/tmp"))

	if _, ok := props["intent"]; !ok {
		t.Error("go write schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("go write schema missing 'accept_large_output' field")
	}
}

func TestGoBashSchemaIncludesIntentAndAcceptLargeOutput(t *testing.T) {
	props := schemaProps(t, tools.Bash("/tmp", tools.BashOptions{}))

	if _, ok := props["intent"]; !ok {
		t.Error("go bash schema missing 'intent' field")
	}
	if _, ok := props["accept_large_output"]; !ok {
		t.Error("go bash schema missing 'accept_large_output' field")
	}
}

func TestIntentFieldIsOptional(t *testing.T) {
	raw := tools.ReadFile("/tmp", tools.FileOptions{}).Parameters()
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schemaBytes(raw), &schema); err != nil {
		t.Fatal(err)
	}
	for _, f := range schema.Required {
		if f == "intent" {
			t.Errorf("'intent' should not be in required fields; got %v", schema.Required)
		}
	}
}

func TestAcceptLargeOutputDescriptionSet(t *testing.T) {
	if tools.AcceptLargeOutputDescription == "" {
		t.Error("AcceptLargeOutputDescription should not be empty")
	}
	if tools.AcceptLargeOutputField != "accept_large_output" {
		t.Errorf("AcceptLargeOutputField = %q, want 'accept_large_output'", tools.AcceptLargeOutputField)
	}
}

func schemaProps(t *testing.T, tool agentcore.Tool) map[string]any {
	t.Helper()
	raw := tool.Parameters()
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schemaBytes(raw), &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return schema.Properties
}

func schemaBytes(m map[string]any) []byte {
	b, _ := json.Marshal(m)
	return b
}
