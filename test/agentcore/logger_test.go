package agentcore_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
)

func TestLegacyWriteEventErrorIncludesToolName(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	ag.WriteLegacyEventForTest(agentcore.Event{
		Category:  agentcore.EventError,
		ToolName:  "bash",
		ToolError: "Tool bash: missing required field(s) [command]",
	})

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("expected one JSONL line, got empty buffer")
	}

	var entry struct {
		Kind      string `json:"kind"`
		Category  string `json:"category"`
		ToolName  string `json:"tool_name"`
		ToolError string `json:"tool_error"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("unmarshal legacy entry: %v\nline: %s", err, line)
	}

	if entry.Kind != "event" {
		t.Errorf("entry.kind = %q, want event", entry.Kind)
	}
	if entry.Category != "error" {
		t.Errorf("entry.category = %q, want error", entry.Category)
	}
	if entry.ToolName != "bash" {
		t.Errorf("entry.tool_name = %q, want bash (EventError must propagate ToolName)", entry.ToolName)
	}
	if entry.ToolError == "" {
		t.Errorf("entry.tool_error empty, want non-empty ToolError")
	}
}

func TestLegacyWriteEventErrorOmitsToolNameWhenUnset(t *testing.T) {
	var buf bytes.Buffer
	ag := agentcore.NewAgentWithLogBufForTest(&buf)

	ag.WriteLegacyEventForTest(agentcore.Event{
		Category:  agentcore.EventError,
		ToolError: "compaction failure",
	})

	var entry struct {
		Kind      string `json:"kind"`
		Category  string `json:"category"`
		ToolName  string `json:"tool_name"`
		ToolError string `json:"tool_error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("unmarshal legacy entry: %v\nbuf: %q", err, buf.String())
	}
	if entry.ToolName != "" {
		t.Errorf("entry.tool_name = %q, want empty (untouched Error events without ToolName must not emit field)", entry.ToolName)
	}
	if entry.ToolError != "compaction failure" {
		t.Errorf("entry.tool_error = %q, want 'compaction failure'", entry.ToolError)
	}
}
