package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// TestLifecycleStatusParseRoundTrip exercises the 13 explicit values
// plus empty/unknown fall-through. Each case verifies String() returns
// the input, ParseLifecycleStatus returns the matching constant, and
// the constant is not StatusOther.
func TestLifecycleStatusParseRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.LifecycleStatus
	}{
		{"spawned", swarm.StatusSpawned},
		{"ready", swarm.StatusReady},
		{"running", swarm.StatusRunning},
		{"running_stale", swarm.StatusRunningStale},
		{"completed", swarm.StatusCompleted},
		{"done", swarm.StatusDone},
		{"failed", swarm.StatusFailed},
		{"stopped", swarm.StatusStopped},
		{"crashed", swarm.StatusCrashed},
		{"queued", swarm.StatusQueued},
		{"blocked", swarm.StatusBlocked},
		{"pending", swarm.StatusPending},
		{"todo", swarm.StatusTodo},
	}
	for _, c := range cases {
		got := swarm.ParseLifecycleStatus(c.wire)
		if got != c.want {
			t.Errorf("ParseLifecycleStatus(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.IsOther() {
			t.Errorf("ParseLifecycleStatus(%q).IsOther() = true, want false", c.wire)
		}
		if got.String() != c.wire {
			t.Errorf("ParseLifecycleStatus(%q).String() = %q, want %q", c.wire, got.String(), c.wire)
		}
	}
}

// TestLifecycleStatusUnknownFallback verifies unknown wire strings
// map to StatusOther (forward-compat with jcode additions).
func TestLifecycleStatusUnknownFallback(t *testing.T) {
	unknowns := []string{"", "frozen", "queued-but-canceled", "abandoned", "timeout"}
	for _, s := range unknowns {
		got := swarm.ParseLifecycleStatus(s)
		if !got.IsOther() {
			t.Errorf("ParseLifecycleStatus(%q).IsOther() = false, want true", s)
		}
		if got.String() != "" {
			t.Errorf("ParseLifecycleStatus(%q).String() = %q, want empty", s, got.String())
		}
	}
}

// TestLifecycleStatusClassification is the full 13-value truth table
// for IsTerminal / IsCompleted / IsFailed / IsActive / IsRunnable /
// IsBlocked. Combined matrix = 13 values * 6 predicates = 78 cases.
func TestLifecycleStatusClassification(t *testing.T) {
	type expect struct {
		terminal  bool
		completed bool
		failed    bool
		active    bool
		runnable  bool
		blocked   bool
	}
	want := map[swarm.LifecycleStatus]expect{
		swarm.StatusSpawned:      {false, false, false, false, false, false},
		swarm.StatusReady:        {false, false, false, false, true, false},
		swarm.StatusRunning:      {false, false, false, true, false, false},
		swarm.StatusRunningStale: {false, false, false, true, false, false},
		swarm.StatusCompleted:    {true, true, false, false, false, false},
		swarm.StatusDone:         {true, true, false, false, false, false},
		swarm.StatusFailed:       {true, false, true, false, false, false},
		swarm.StatusStopped:      {true, false, true, false, false, false},
		swarm.StatusCrashed:      {true, false, true, false, false, false},
		swarm.StatusQueued:       {false, false, false, false, true, false},
		swarm.StatusBlocked:      {false, false, false, false, false, true},
		swarm.StatusPending:      {false, false, false, false, true, false},
		swarm.StatusTodo:         {false, false, false, false, true, false},
	}
	for status, exp := range want {
		if got := status.IsTerminal(); got != exp.terminal {
			t.Errorf("%v.IsTerminal() = %v, want %v", status, got, exp.terminal)
		}
		if got := status.IsCompleted(); got != exp.completed {
			t.Errorf("%v.IsCompleted() = %v, want %v", status, got, exp.completed)
		}
		if got := status.IsFailed(); got != exp.failed {
			t.Errorf("%v.IsFailed() = %v, want %v", status, got, exp.failed)
		}
		if got := status.IsActive(); got != exp.active {
			t.Errorf("%v.IsActive() = %v, want %v", status, got, exp.active)
		}
		if got := status.IsRunnable(); got != exp.runnable {
			t.Errorf("%v.IsRunnable() = %v, want %v", status, got, exp.runnable)
		}
		if got := status.IsBlocked(); got != exp.blocked {
			t.Errorf("%v.IsBlocked() = %v, want %v", status, got, exp.blocked)
		}
		// Package-level aliases must agree with method predicates.
		if got := swarm.IsTerminalStatus(status); got != exp.terminal {
			t.Errorf("IsTerminalStatus(%v) = %v, want %v", status, got, exp.terminal)
		}
		if got := swarm.IsCompletedStatus(status); got != exp.completed {
			t.Errorf("IsCompletedStatus(%v) = %v, want %v", status, got, exp.completed)
		}
		if got := swarm.IsActiveStatus(status); got != exp.active {
			t.Errorf("IsActiveStatus(%v) = %v, want %v", status, got, exp.active)
		}
		if got := swarm.IsRunnableStatus(status); got != exp.runnable {
			t.Errorf("IsRunnableStatus(%v) = %v, want %v", status, got, exp.runnable)
		}
		if got := swarm.IsFailedStatus(status); got != exp.failed {
			t.Errorf("IsFailedStatus(%v) = %v, want %v", status, got, exp.failed)
		}
	}
}

// TestLifecycleStatusJSONRoundTrip marshals/unmarshals every value
// and verifies byte equality + the unknown-input handling.
func TestLifecycleStatusJSONRoundTrip(t *testing.T) {
	for _, want := range []swarm.LifecycleStatus{
		swarm.StatusSpawned, swarm.StatusRunning, swarm.StatusCompleted,
		swarm.StatusFailed, swarm.StatusQueued, swarm.StatusBlocked,
		swarm.StatusTodo, swarm.StatusOther,
	} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", want, err)
		}
		var got swarm.LifecycleStatus
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("Unmarshal(%q): %v", b, err)
		}
		if got != want {
			t.Errorf("round-trip mismatch: got %v, want %v", got, want)
		}
		if string(b) != `"`+want.String()+`"` {
			t.Errorf("Marshal(%v) = %q, want %q", want, string(b), `"`+want.String()+`"`)
		}
	}
}

// TestLifecycleStatusUnmarshalUnknown verifies that an unknown wire
// string unmarshals to StatusOther without error (forward-compat).
func TestLifecycleStatusUnmarshalUnknown(t *testing.T) {
	var s swarm.LifecycleStatus
	if err := json.Unmarshal([]byte(`"abandoned"`), &s); err != nil {
		t.Fatal(err)
	}
	if !s.IsOther() {
		t.Errorf("Unmarshal(\"abandoned\") = %v, want StatusOther", s)
	}
}

// TestLifecycleStatusMarshalEmpty verifies the empty string round-
// trips cleanly (used when StatusOther is the explicit value).
func TestLifecycleStatusMarshalEmpty(t *testing.T) {
	b, err := json.Marshal(swarm.StatusOther)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `""` {
		t.Errorf("Marshal(StatusOther) = %q, want %q", string(b), `""`)
	}
}

// TestStatusCoverage ensures no string in the 13-value table was
// silently dropped. Cheap guardrail against accidental edits.
func TestStatusCoverage(t *testing.T) {
	all := []swarm.LifecycleStatus{
		swarm.StatusSpawned, swarm.StatusReady, swarm.StatusRunning,
		swarm.StatusRunningStale, swarm.StatusCompleted, swarm.StatusDone,
		swarm.StatusFailed, swarm.StatusStopped, swarm.StatusCrashed,
		swarm.StatusQueued, swarm.StatusBlocked, swarm.StatusPending,
		swarm.StatusTodo,
	}
	if len(all) != 13 {
		t.Fatalf("expected 13 lifecycle values, got %d", len(all))
	}
	seen := map[swarm.LifecycleStatus]bool{}
	for _, s := range all {
		if s.IsOther() || s.String() == "" {
			t.Errorf("%v is empty or Other — not a valid status", s)
		}
		if seen[s] {
			t.Errorf("duplicate status: %v", s)
		}
		seen[s] = true
	}
}
