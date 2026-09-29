package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestContextEntryJSON(t *testing.T) {
	in := swarm.ContextEntry{
		Key:      "design",
		Value:    "use JWT",
		SharedBy: "s1",
		SharedAt: "2026-01-15T10:00:00Z",
		Append:   true,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ContextEntry
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}
}

func TestShareParamsJSON(t *testing.T) {
	in := swarm.ShareParams{
		SwarmID:       "sw1",
		FromSessionID: "s1",
		Key:           "design",
		Value:         "use JWT",
		Append:        true,
		RequestNonce:  "n1",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ShareParams
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}
}

func TestReadParamsJSON(t *testing.T) {
	in := swarm.ReadParams{
		SwarmID:       "sw1",
		FromSessionID: "s1",
		Key:           "design",
		Limit:         5,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ReadParams
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, in)
	}
}

func TestReadResultJSON(t *testing.T) {
	in := swarm.ReadResult{
		Key: "design",
		Entries: []swarm.ContextEntry{
			{Key: "design", Value: "use JWT", SharedBy: "s1", SharedAt: "2026-01-15T10:00:00Z"},
			{Key: "design", Value: "and refresh tokens", SharedBy: "s1", SharedAt: "2026-01-15T10:30:00Z", Append: true},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ReadResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(got.Entries))
	}
	if got.Entries[1].Value != "and refresh tokens" {
		t.Errorf("Entries[1].Value = %q, want %q", got.Entries[1].Value, "and refresh tokens")
	}
}
