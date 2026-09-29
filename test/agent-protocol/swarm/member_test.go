package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestMemberStatusJSON(t *testing.T) {
	secs := uint64(3600)
	in := swarm.MemberStatus{
		SessionID:        "sess-1",
		FriendlyName:     "alice",
		Status:           swarm.StatusRunning,
		Detail:           "implementing auth",
		TaskLabel:        "implement auth",
		Role:             swarm.RoleAgent,
		IsHeadless:       false,
		LiveAttachments:  2,
		StatusAgeSecs:    &secs,
		LastActivitySecs: &secs,
		ReportBackToID:   "root",
		LatestReport:     "tested",
		LatestReportTLDR: "ok",
		ParentSessionID:  "root",
		SwarmID:          "swarm-1",
	}
	in.Runtime = swarm.MemberRuntime{
		Model:      "sonnet",
		Provider:   "anthropic",
		AuthMethod: "env",
		Effort:     "medium",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got swarm.MemberStatus
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != in.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, in.SessionID)
	}
	if got.Role != in.Role {
		t.Errorf("Role = %v, want %v", got.Role, in.Role)
	}
	if got.Status != in.Status {
		t.Errorf("Status = %v, want %v", got.Status, in.Status)
	}
	if got.TaskLabel != in.TaskLabel {
		t.Errorf("TaskLabel = %q, want %q", got.TaskLabel, in.TaskLabel)
	}
	if got.LiveAttachments != in.LiveAttachments {
		t.Errorf("LiveAttachments = %d, want %d", got.LiveAttachments, in.LiveAttachments)
	}
	if got.StatusAgeSecs == nil || *got.StatusAgeSecs != 3600 {
		t.Errorf("StatusAgeSecs = %v, want 3600", got.StatusAgeSecs)
	}
	if got.Runtime.Model != in.Runtime.Model {
		t.Errorf("Runtime.Model = %q, want %q", got.Runtime.Model, in.Runtime.Model)
	}
}

func TestMemberStatusOmitEmpty(t *testing.T) {
	in := swarm.MemberStatus{SessionID: "s"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"friendly_name", "task_label", "is_headless", "report_back_to_session_id"} {
		if contains(b, want) {
			t.Errorf("omitted field %q should not appear, got %s", want, b)
		}
	}
	if !contains(b, `"session_id":"s"`) {
		t.Errorf("session_id missing: %s", b)
	}
	// Runtime is a value type (not pointer) so it always emits
	// {"runtime":{}}; verify the shape.
	if !contains(b, `"runtime":{}`) {
		t.Errorf("runtime should serialize as empty object, got %s", b)
	}
}

func TestTodoItemJSON(t *testing.T) {
	in := swarm.TodoItem{
		Content: "implement",
		Status:  "pending",
		ToolIntents: []swarm.ToolIntent{
			{
				ToolName:  "edit",
				Intent:    "edit main.go",
				Status:    "running",
				StartedAt: "2026-01-15T10:00:00Z",
			},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.TodoItem
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Content != in.Content {
		t.Errorf("Content = %q, want %q", got.Content, in.Content)
	}
	if got.Status != in.Status {
		t.Errorf("Status = %q, want %q", got.Status, in.Status)
	}
	if len(got.ToolIntents) != 1 {
		t.Errorf("len(ToolIntents) = %d, want 1", len(got.ToolIntents))
	}
	if got.ToolIntents[0].ToolName != "edit" {
		t.Errorf("ToolIntents[0].ToolName = %q, want edit", got.ToolIntents[0].ToolName)
	}
}

func contains(haystack []byte, needle string) bool {
	s := string(haystack)
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
