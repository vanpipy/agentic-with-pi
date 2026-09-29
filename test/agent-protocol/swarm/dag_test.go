package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestModeRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.Mode
	}{
		{"deep", swarm.ModeDeep},
		{"light", swarm.ModeLight},
	}
	for _, c := range cases {
		got := swarm.ParseMode(c.wire)
		if got != c.want {
			t.Errorf("ParseMode(%q) = %v, want %v", c.wire, got, c.want)
		}
	}
	for _, s := range []string{"", "auto", "hybrid"} {
		got := swarm.ParseMode(s)
		if !got.IsOther() {
			t.Errorf("ParseMode(%q).IsOther() = false, want true", s)
		}
	}
}

func TestModeJSON(t *testing.T) {
	for _, want := range []swarm.Mode{swarm.ModeDeep, swarm.ModeLight, swarm.ModeOther} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got swarm.Mode
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}

func TestModePlanModeConsistency(t *testing.T) {
	// Mode and PlanMode must round-trip identically for the same wire string.
	// Verify by parsing both and comparing .String().
	pairs := map[string]struct {
		mode swarm.Mode
		plan swarm.PlanMode
	}{
		"deep":  {swarm.ModeDeep, swarm.PlanModeDeep},
		"light": {swarm.ModeLight, swarm.PlanModeLight},
	}
	for wire, want := range pairs {
		if swarm.ParseMode(wire) != want.mode {
			t.Errorf("ParseMode(%q) = %v, want %v", wire, swarm.ParseMode(wire), want.mode)
		}
		if swarm.ParsePlanMode(wire) != want.plan {
			t.Errorf("ParsePlanMode(%q) = %v, want %v", wire, swarm.ParsePlanMode(wire), want.plan)
		}
		if swarm.ParseMode(wire).String() != swarm.ParsePlanMode(wire).String() {
			t.Errorf("Mode/PlanMode wire mismatch for %q", wire)
		}
	}
}

func TestNodeKindRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.NodeKind
	}{
		{"explore", swarm.NodeExplore},
		{"implement", swarm.NodeImplement},
		{"verify", swarm.NodeVerify},
		{"fix", swarm.NodeFix},
		{"synthesize", swarm.NodeSynthesize},
		{"critique", swarm.NodeCritique},
	}
	for _, c := range cases {
		got := swarm.ParseNodeKind(c.wire)
		if got != c.want {
			t.Errorf("ParseNodeKind(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.IsOther() {
			t.Errorf("ParseNodeKind(%q).IsOther() = true, want false", c.wire)
		}
		if got.String() != c.wire {
			t.Errorf("ParseNodeKind(%q).String() = %q, want %q", c.wire, got.String(), c.wire)
		}
	}
}

func TestNodeKindUnknown(t *testing.T) {
	for _, s := range []string{"", "research", "test", "doc"} {
		got := swarm.ParseNodeKind(s)
		if !got.IsOther() {
			t.Errorf("ParseNodeKind(%q).IsOther() = false, want true", s)
		}
	}
}

func TestNodeKindGateClassification(t *testing.T) {
	if !swarm.NodeVerify.IsGateKind() {
		t.Error("NodeVerify.IsGateKind() = false, want true")
	}
	if !swarm.NodeCritique.IsGateKind() {
		t.Error("NodeCritique.IsGateKind() = false, want true")
	}
	if swarm.NodeImplement.IsGateKind() {
		t.Error("NodeImplement.IsGateKind() = true, want false")
	}
	if swarm.NodeExplore.IsGateKind() {
		t.Error("NodeExplore.IsGateKind() = true, want false")
	}
	if swarm.NodeOther.IsGateKind() {
		t.Error("NodeOther.IsGateKind() = true, want false")
	}
}

func TestNodeKindGateKind(t *testing.T) {
	cases := []struct {
		in   swarm.NodeKind
		want swarm.NodeKind
	}{
		{swarm.NodeImplement, swarm.NodeVerify},
		{swarm.NodeFix, swarm.NodeVerify},
		{swarm.NodeExplore, swarm.NodeCritique},
		{swarm.NodeSynthesize, swarm.NodeCritique},
		{swarm.NodeVerify, swarm.NodeVerify},
		{swarm.NodeCritique, swarm.NodeCritique},
		{swarm.NodeOther, swarm.NodeOther},
	}
	for _, c := range cases {
		got := c.in.GateKind()
		if got != c.want {
			t.Errorf("%v.GateKind() = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNodeStatusRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.NodeStatus
	}{
		{"queued", swarm.NodeQueued},
		{"running", swarm.NodeRunning},
		{"done", swarm.NodeDone},
		{"failed", swarm.NodeFailed},
	}
	for _, c := range cases {
		got := swarm.ParseNodeStatus(c.wire)
		if got != c.want {
			t.Errorf("ParseNodeStatus(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.IsOther() {
			t.Errorf("ParseNodeStatus(%q).IsOther() = true, want false", c.wire)
		}
	}
	for _, s := range []string{"", "completed", "abandoned"} {
		got := swarm.ParseNodeStatus(s)
		if !got.IsOther() {
			t.Errorf("ParseNodeStatus(%q).IsOther() = false, want true", s)
		}
	}
}

func TestNodeStatusTerminal(t *testing.T) {
	if !swarm.NodeDone.IsTerminal() {
		t.Error("NodeDone.IsTerminal() = false, want true")
	}
	if !swarm.NodeFailed.IsTerminal() {
		t.Error("NodeFailed.IsTerminal() = false, want true")
	}
	if swarm.NodeQueued.IsTerminal() {
		t.Error("NodeQueued.IsTerminal() = true, want false")
	}
	if swarm.NodeRunning.IsTerminal() {
		t.Error("NodeRunning.IsTerminal() = true, want false")
	}
}

func TestNodeOriginRoundTrip(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.NodeOrigin
	}{
		{"seed", swarm.OriginSeed},
		{"expand", swarm.OriginExpand},
		{"gap", swarm.OriginGap},
		{"gate", swarm.OriginGate},
	}
	for _, c := range cases {
		got := swarm.ParseNodeOrigin(c.wire)
		if got != c.want {
			t.Errorf("ParseNodeOrigin(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.IsOther() {
			t.Errorf("ParseNodeOrigin(%q).IsOther() = true, want false", c.wire)
		}
	}
	for _, s := range []string{"", "manual", "imported"} {
		got := swarm.ParseNodeOrigin(s)
		if !got.IsOther() {
			t.Errorf("ParseNodeOrigin(%q).IsOther() = false, want true", s)
		}
	}
}

func TestConfidenceLevelRoundTrip(t *testing.T) {
	lows := []string{"low", "1", "1/10", "2/10", "3/10", "not confident", "uncertain"}
	for _, s := range lows {
		got := swarm.ParseConfidenceLevel(s)
		if !got.IsOther() {
			// Use direct string comparison since ConfidenceLow has
			// string "low"
		}
		if swarm.ParseConfidenceLevel(s).String() != swarm.ConfidenceLow.String() {
			t.Errorf("ParseConfidenceLevel(%q) != ConfidenceLow", s)
		}
	}
	mediums := []string{"medium", "5", "4/10", "5/10", "6/10", "7/10", "moderate"}
	for _, s := range mediums {
		if swarm.ParseConfidenceLevel(s).String() != swarm.ConfidenceMedium.String() {
			t.Errorf("ParseConfidenceLevel(%q) != ConfidenceMedium", s)
		}
	}
	highs := []string{"high", "8", "9", "10", "8/10", "9/10", "10/10", "very confident"}
	for _, s := range highs {
		if swarm.ParseConfidenceLevel(s).String() != swarm.ConfidenceHigh.String() {
			t.Errorf("ParseConfidenceLevel(%q) != ConfidenceHigh", s)
		}
	}
	for _, s := range []string{"", "extreme", "yolo", "67"} {
		if !swarm.ParseConfidenceLevel(s).IsOther() {
			t.Errorf("ParseConfidenceLevel(%q).IsOther() = false, want true", s)
		}
	}
}

func TestConfidenceLevelJSON(t *testing.T) {
	for _, want := range []swarm.ConfidenceLevel{
		swarm.ConfidenceLow, swarm.ConfidenceMedium, swarm.ConfidenceHigh,
		swarm.ConfidenceOther,
	} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got swarm.ConfidenceLevel
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}

func TestTaskGraphNodeSpecJSON(t *testing.T) {
	in := swarm.TaskGraphNodeSpec{
		ID:        "t1",
		Content:   "implement auth",
		Kind:      swarm.NodeImplement,
		Priority:  "high",
		Subsystem: "auth",
		FileScope: []string{"auth/*.go"},
		BlockedBy: []string{"t0"},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.TaskGraphNodeSpec
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != in.ID || got.Content != in.Content || got.Kind != in.Kind ||
		got.Priority != in.Priority || got.Subsystem != in.Subsystem ||
		!equalStrings(got.FileScope, in.FileScope) ||
		!equalStrings(got.BlockedBy, in.BlockedBy) {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

func TestTaskNodeJSON(t *testing.T) {
	in := swarm.TaskNode{
		ID:           "t1",
		Content:      "implement",
		Kind:         swarm.NodeImplement,
		Status:       swarm.NodeRunning,
		AssignedTo:   "s1",
		Planner:      "root",
		Origin:       swarm.OriginSeed,
		Parent:       "t0",
		Expanded:     true,
		IsGate:       false,
		ArtifactJSON: `{"findings":"x"}`,
		CreatedAtMS:  1000,
		UpdatedAtMS:  2000,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.TaskNode
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

func TestHandoffArtifactJSON(t *testing.T) {
	in := swarm.HandoffArtifact{
		Findings:            "implemented JWT",
		Evidence:            []string{"auth/jwt.go:1-100"},
		EdgeCasesConsidered: []string{"expired tokens"},
		Validation:          "tests pass",
		OpenQuestions:       []string{"refresh flow"},
		Confidence:          swarm.ConfidenceHigh,
		WhatIDidNotCheck:    []string{"concurrent revokes"},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.HandoffArtifact
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Findings != in.Findings || got.Validation != in.Validation ||
		got.Confidence != in.Confidence ||
		!equalStrings(got.Evidence, in.Evidence) ||
		!equalStrings(got.EdgeCasesConsidered, in.EdgeCasesConsidered) ||
		!equalStrings(got.OpenQuestions, in.OpenQuestions) ||
		!equalStrings(got.WhatIDidNotCheck, in.WhatIDidNotCheck) {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

func TestNodeMetaJSON(t *testing.T) {
	in := swarm.NodeMeta{
		Kind:         swarm.NodeImplement,
		Parent:       "t0",
		Expanded:     true,
		IsGate:       false,
		Planner:      "root",
		ArtifactJSON: `{"x":1}`,
		Origin:       swarm.OriginSeed,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.NodeMeta
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}
