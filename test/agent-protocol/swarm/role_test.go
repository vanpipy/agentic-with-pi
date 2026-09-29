package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestRoleParseRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.Role
	}{
		{"agent", swarm.RoleAgent},
		{"coordinator", swarm.RoleCoordinator},
	}
	for _, c := range cases {
		got := swarm.ParseRole(c.wire)
		if got != c.want {
			t.Errorf("ParseRole(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.IsOther() {
			t.Errorf("ParseRole(%q).IsOther() = true, want false", c.wire)
		}
		if got.String() != c.wire {
			t.Errorf("ParseRole(%q).String() = %q, want %q", c.wire, got.String(), c.wire)
		}
	}
}

func TestRoleUnknownFallback(t *testing.T) {
	unknowns := []string{"", "planner", "reviewer", "orchestrator", "user"}
	for _, s := range unknowns {
		got := swarm.ParseRole(s)
		if !got.IsOther() {
			t.Errorf("ParseRole(%q).IsOther() = false, want true", s)
		}
		if got.String() != "" {
			t.Errorf("ParseRole(%q).String() = %q, want empty", s, got.String())
		}
	}
}

func TestRolePredicates(t *testing.T) {
	if !swarm.RoleAgent.IsAgent() {
		t.Error("RoleAgent.IsAgent() = false, want true")
	}
	if swarm.RoleCoordinator.IsAgent() {
		t.Error("RoleCoordinator.IsAgent() = true, want false")
	}
	if !swarm.RoleCoordinator.IsCoordinator() {
		t.Error("RoleCoordinator.IsCoordinator() = false, want true")
	}
	if swarm.RoleAgent.IsCoordinator() {
		t.Error("RoleAgent.IsCoordinator() = true, want false")
	}
	if !swarm.RoleOther.IsOther() {
		t.Error("RoleOther.IsOther() = false, want true")
	}
}

func TestRoleJSONRoundTrip(t *testing.T) {
	for _, want := range []swarm.Role{
		swarm.RoleAgent, swarm.RoleCoordinator, swarm.RoleOther,
	} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", want, err)
		}
		var got swarm.Role
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("round-trip mismatch: got %v, want %v", got, want)
		}
	}
}

func TestRoleUnmarshalUnknown(t *testing.T) {
	var r swarm.Role
	if err := json.Unmarshal([]byte(`"planner"`), &r); err != nil {
		t.Fatal(err)
	}
	if !r.IsOther() {
		t.Errorf("Unmarshal(\"planner\") = %v, want RoleOther", r)
	}
}
