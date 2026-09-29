package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestMemberRecordJSON(t *testing.T) {
	in := swarm.MemberRecord{
		SessionID:    "s1",
		SwarmID:      "sw1",
		WorkingDir:   "/tmp",
		SwarmEnabled: true,
		Status:       swarm.StatusRunning,
		FriendlyName: "alice",
		Role:         swarm.RoleAgent,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.MemberRecord
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != in.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, in.SessionID)
	}
	if got.Status != in.Status {
		t.Errorf("Status = %v, want %v", got.Status, in.Status)
	}
	if got.Role != in.Role {
		t.Errorf("Role = %v, want %v", got.Role, in.Role)
	}
	if got.SwarmEnabled != in.SwarmEnabled {
		t.Errorf("SwarmEnabled = %v, want %v", got.SwarmEnabled, in.SwarmEnabled)
	}
}

func TestChannelSubscriptionRecordJSON(t *testing.T) {
	in := swarm.ChannelSubscriptionRecord{
		SessionID:    "s1",
		SwarmID:      "sw1",
		Channel:      "main",
		SubscribedAt: "2026-01-15T10:00:00Z",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ChannelSubscriptionRecord
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}
}

func TestCompletionReportRecordJSON(t *testing.T) {
	in := swarm.CompletionReportRecord{
		SessionID:  "s1",
		SwarmID:    "sw1",
		Body:       "task finished",
		Validation: "tests pass",
		FollowUp:   "open question",
		TLDR:       "done",
		Status:     swarm.StatusCompleted,
		ReportedAt: "2026-01-15T11:00:00Z",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.CompletionReportRecord
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != in.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, in.SessionID)
	}
	if got.Body != in.Body {
		t.Errorf("Body = %q, want %q", got.Body, in.Body)
	}
	if got.TLDR != in.TLDR {
		t.Errorf("TLDR = %q, want %q", got.TLDR, in.TLDR)
	}
	if got.Status != in.Status {
		t.Errorf("Status = %v, want %v", got.Status, in.Status)
	}
}

func TestSharedContextRecordJSON(t *testing.T) {
	in := swarm.SharedContextRecord{
		SwarmID:  "sw1",
		Key:      "design",
		Value:    "use JWT",
		Append:   false,
		SharedBy: "s1",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.SharedContextRecord
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Key != in.Key {
		t.Errorf("Key = %q, want %q", got.Key, in.Key)
	}
	if got.Value != in.Value {
		t.Errorf("Value = %q, want %q", got.Value, in.Value)
	}
	if got.Append != in.Append {
		t.Errorf("Append = %v, want %v", got.Append, in.Append)
	}
}
