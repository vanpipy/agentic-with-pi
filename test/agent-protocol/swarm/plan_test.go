package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestPlanModeRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.PlanMode
	}{
		{"light", swarm.PlanModeLight},
		{"deep", swarm.PlanModeDeep},
	}
	for _, c := range cases {
		got := swarm.ParsePlanMode(c.wire)
		if got != c.want {
			t.Errorf("ParsePlanMode(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.String() != c.wire {
			t.Errorf("ParsePlanMode(%q).String() = %q, want %q", c.wire, got.String(), c.wire)
		}
	}
}

func TestPlanModeUnknown(t *testing.T) {
	for _, s := range []string{"", "auto", "hybrid"} {
		got := swarm.ParsePlanMode(s)
		if !got.IsOther() {
			t.Errorf("ParsePlanMode(%q).IsOther() = false, want true", s)
		}
	}
}

func TestPlanModePredicates(t *testing.T) {
	if !swarm.PlanModeLight.IsLight() {
		t.Error("PlanModeLight.IsLight() = false, want true")
	}
	if swarm.PlanModeLight.IsDeep() {
		t.Error("PlanModeLight.IsDeep() = true, want false")
	}
	if !swarm.PlanModeDeep.IsDeep() {
		t.Error("PlanModeDeep.IsDeep() = false, want true")
	}
	if !swarm.PlanModeDeep.RequiresGates() {
		t.Error("PlanModeDeep.RequiresGates() = false, want true")
	}
	if !swarm.PlanModeDeep.RequiresArtifact() {
		t.Error("PlanModeDeep.RequiresArtifact() = false, want true")
	}
	if swarm.PlanModeLight.RequiresGates() {
		t.Error("PlanModeLight.RequiresGates() = true, want false")
	}
}

func TestPlanModeJSON(t *testing.T) {
	for _, want := range []swarm.PlanMode{swarm.PlanModeLight, swarm.PlanModeDeep, swarm.PlanModeOther} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got swarm.PlanMode
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}

func TestTaskControlActionParse(t *testing.T) {
	valid := []swarm.TaskControlAction{
		swarm.TaskStart, swarm.TaskWake, swarm.TaskResume,
		swarm.TaskRetry, swarm.TaskReassign, swarm.TaskReplace,
		swarm.TaskSalvage,
	}
	for _, a := range valid {
		got, ok := swarm.ParseTaskControlAction(string(a))
		if !ok {
			t.Errorf("ParseTaskControlAction(%q) = false, want true", a)
		}
		if got != a {
			t.Errorf("ParseTaskControlAction(%q) = %v, want %v", a, got, a)
		}
	}
	for _, s := range []string{"", "unknown", "skip", "cancel"} {
		got, ok := swarm.ParseTaskControlAction(s)
		if ok {
			t.Errorf("ParseTaskControlAction(%q) ok=true, want false", s)
		}
		if got != "" {
			t.Errorf("ParseTaskControlAction(%q) = %v, want empty", s, got)
		}
	}
}

func TestTaskControlActionAllowsStatus(t *testing.T) {
	cases := []struct {
		action swarm.TaskControlAction
		status swarm.LifecycleStatus
		want   bool
	}{
		{swarm.TaskStart, swarm.StatusQueued, true},
		{swarm.TaskStart, swarm.StatusReady, true},
		{swarm.TaskStart, swarm.StatusRunning, false},
		{swarm.TaskStart, swarm.StatusCompleted, false},

		{swarm.TaskWake, swarm.StatusRunning, true},
		{swarm.TaskWake, swarm.StatusReady, false},
		{swarm.TaskWake, swarm.StatusCompleted, false},

		{swarm.TaskResume, swarm.StatusBlocked, true},
		{swarm.TaskResume, swarm.StatusRunning, false},

		{swarm.TaskRetry, swarm.StatusReady, true},
		{swarm.TaskRetry, swarm.StatusRunning, true},
		{swarm.TaskRetry, swarm.StatusBlocked, true},
		{swarm.TaskRetry, swarm.StatusFailed, true},
		{swarm.TaskRetry, swarm.StatusCompleted, false},

		{swarm.TaskReassign, swarm.StatusReady, true},
		{swarm.TaskReassign, swarm.StatusRunning, true},
		{swarm.TaskReassign, swarm.StatusCompleted, false},

		{swarm.TaskReplace, swarm.StatusQueued, true},
		{swarm.TaskReplace, swarm.StatusReady, true},
		{swarm.TaskReplace, swarm.StatusRunning, true},
		{swarm.TaskReplace, swarm.StatusBlocked, true},
		{swarm.TaskReplace, swarm.StatusFailed, true},
		{swarm.TaskReplace, swarm.StatusCompleted, true},

		{swarm.TaskSalvage, swarm.StatusRunning, true},
		{swarm.TaskSalvage, swarm.StatusFailed, true},
		{swarm.TaskSalvage, swarm.StatusCompleted, true},
		{swarm.TaskSalvage, swarm.StatusReady, false},
	}
	for _, c := range cases {
		got := c.action.AllowsStatus(c.status)
		if got != c.want {
			t.Errorf("%v.AllowsStatus(%v) = %v, want %v", c.action, c.status, got, c.want)
		}
	}
}

func TestPlanItemJSON(t *testing.T) {
	in := swarm.PlanItem{
		ID:         "t1",
		Content:    "implement auth",
		Status:     swarm.StatusQueued,
		Priority:   "high",
		Subsystem:  "auth",
		FileScope:  []string{"auth/*.go"},
		BlockedBy:  []string{"t0"},
		AssignedTo: "s1",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.PlanItem
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != in.ID || got.Content != in.Content || got.Status != in.Status ||
		got.Priority != in.Priority || got.Subsystem != in.Subsystem ||
		got.AssignedTo != in.AssignedTo ||
		!equalStrings(got.FileScope, in.FileScope) ||
		!equalStrings(got.BlockedBy, in.BlockedBy) {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

func TestVersionedPlanJSON(t *testing.T) {
	in := swarm.VersionedPlan{
		Items: []swarm.PlanItem{
			{ID: "t1", Status: swarm.StatusQueued, Priority: "high"},
		},
		Version:      3,
		Participants: map[string]struct{}{"s1": {}},
		Mode:         swarm.PlanModeLight,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.VersionedPlan
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 3 {
		t.Errorf("Version = %d, want 3", got.Version)
	}
	if len(got.Items) != 1 {
		t.Errorf("len(Items) = %d, want 1", len(got.Items))
	}
}

func TestTaskProgressJSON(t *testing.T) {
	ms := int64(1000)
	in := swarm.TaskProgress{
		AssignedSessionID: "s1",
		AssignmentSummary: "implement",
		AssignedAtUnixMS:  &ms,
		HeartbeatCount:    func() *uint64 { v := uint64(5); return &v }(),
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.TaskProgress
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.AssignedSessionID != "s1" {
		t.Errorf("AssignedSessionID = %q, want s1", got.AssignedSessionID)
	}
	if got.AssignedAtUnixMS == nil || *got.AssignedAtUnixMS != 1000 {
		t.Errorf("AssignedAtUnixMS = %v, want 1000", got.AssignedAtUnixMS)
	}
	if got.HeartbeatCount == nil || *got.HeartbeatCount != 5 {
		t.Errorf("HeartbeatCount = %v, want 5", got.HeartbeatCount)
	}
}
