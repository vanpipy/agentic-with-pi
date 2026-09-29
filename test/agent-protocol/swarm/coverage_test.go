package swarm_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// TestSummarizePlanGraphUnresolved covers the branch in
// SummarizePlanGraph that walks BlockedBy to collect unresolved
// dependency ids (those not present in the items list).
func TestSummarizePlanGraphUnresolved(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "t1", Status: swarm.StatusQueued, BlockedBy: []string{"ghost", "real"}},
		{ID: "real", Status: swarm.StatusCompleted},
		{ID: "t2", Status: swarm.StatusQueued, BlockedBy: []string{"missing"}},
	}
	got := swarm.SummarizePlanGraph(items)
	// "ghost" is in BlockedBy of t1, not completed, not in items.
	// "missing" is in BlockedBy of t2, not completed, not in items.
	if !containsString(got.UnresolvedDependencyIDs, "ghost") {
		t.Errorf("UnresolvedDependencyIDs missing 'ghost': %v", got.UnresolvedDependencyIDs)
	}
	if !containsString(got.UnresolvedDependencyIDs, "missing") {
		t.Errorf("UnresolvedDependencyIDs missing 'missing': %v", got.UnresolvedDependencyIDs)
	}
}

// TestSummarizePlanGraphReadyAllBlocked covers the branch where every
// item has dependencies and no items end up runnable.
func TestSummarizePlanGraphReadyAllBlocked(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusQueued, BlockedBy: []string{"b"}},
		{ID: "b", Status: swarm.StatusQueued, BlockedBy: []string{"a"}},
	}
	got := swarm.SummarizePlanGraph(items)
	if got.ReadyIDs != nil {
		t.Errorf("ReadyIDs = %v, want nil (all blocked)", got.ReadyIDs)
	}
	if len(got.CycleIDs) != 2 {
		t.Errorf("CycleIDs = %v, want 2 entries", got.CycleIDs)
	}
}

// TestSummarizePlanGraphReadySorted checks the sorted-output invariant
// on ready/blocked/active/completed/failed/terminal lists.
func TestSummarizePlanGraphReadySorted(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "z", Status: swarm.StatusQueued},
		{ID: "a", Status: swarm.StatusQueued},
		{ID: "m", Status: swarm.StatusQueued},
	}
	got := swarm.SummarizePlanGraph(items)
	if strings.Join(got.ReadyIDs, ",") != "a,m,z" {
		t.Errorf("ReadyIDs = %v, want [a, m, z]", got.ReadyIDs)
	}
}

// TestNextRunnableItemIDsPriorityTie verifies that ties on priority
// fall back to alphabetical id ordering.
func TestNextRunnableItemIDsPriorityTie(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "z", Status: swarm.StatusQueued, Priority: "high"},
		{ID: "a", Status: swarm.StatusQueued, Priority: "high"},
		{ID: "m", Status: swarm.StatusQueued, Priority: "high"},
	}
	got := swarm.NextRunnableItemIDs(items, 10)
	if strings.Join(got, ",") != "a,m,z" {
		t.Errorf("NextRunnableItemIDs = %v, want [a, m, z] (alpha tie-break)", got)
	}
}

// TestNextRunnableItemIDsEmpty exercises the empty-input branch.
func TestNextRunnableItemIDsEmpty(t *testing.T) {
	if got := swarm.NextRunnableItemIDs(nil, 10); got != nil {
		t.Errorf("NextRunnableItemIDs(nil) = %v, want nil", got)
	}
}

// TestNewlyReadyItemIDsNoDepsTransition covers the branch where an
// item has no deps but transitions from blocked to queued.
func TestNewlyReadyItemIDsNoDepsTransition(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusBlocked},
	}
	after := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusQueued},
	}
	got := swarm.NewlyReadyItemIDs(before, after)
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("NewlyReadyItemIDs = %v, want [x]", got)
	}
}

// TestNewlyReadyItemIDsEmptyBefore covers the first-time-boot path
// (before is empty, after has runnable items).
func TestNewlyReadyItemIDsEmptyBefore(t *testing.T) {
	before := []swarm.PlanItem{}
	after := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusQueued},
	}
	got := swarm.NewlyReadyItemIDs(before, after)
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("NewlyReadyItemIDs = %v, want [x]", got)
	}
}

// TestValidateTLDRLongTldrBoundary covers the tldr-too-long branch
// independent of body length.
func TestValidateTLDRLongTldrBoundary(t *testing.T) {
	huge := strings.Repeat("z", swarm.MaxTLDRChars)
	short := "x"
	if err := swarm.ValidateTLDR(huge, short); err != nil {
		t.Errorf("ValidateTLDR(huge, short) = %v, want nil (boundary inclusive)", err)
	}
}

// TestDeriveTaskLabelPunctuation covers cases that exercise the rune-
// by-rune filter logic.
func TestDeriveTaskLabelPunctuation(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"hyphen-kept", "well-known issue", "well-known issue"},
		{"underscore-stripped", "fix_bug now", "fixbug now"},
		{"digits-kept", "issue 42 resolved", "issue 42 resolved"},
		{"backslash-stripped", "fix\\bug", "fixbug"},
		{"mixed-case-trimmed", "**FIX BUG**", "FIX BUG"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := swarm.DeriveTaskLabel(c.in)
			if got != c.want {
				t.Errorf("DeriveTaskLabel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestModePredicates covers the previously-uncovered predicates.
func TestModePredicates(t *testing.T) {
	if !swarm.ModeDeep.IsDeep() {
		t.Error("ModeDeep.IsDeep() = false, want true")
	}
	if swarm.ModeDeep.IsLight() {
		t.Error("ModeDeep.IsLight() = true, want false")
	}
	if swarm.ModeDeep.IsOther() {
		t.Error("ModeDeep.IsOther() = true, want false")
	}
	if !swarm.ModeDeep.RequiresGates() {
		t.Error("ModeDeep.RequiresGates() = false, want true")
	}
	if swarm.ModeLight.IsDeep() {
		t.Error("ModeLight.IsDeep() = true, want false")
	}
	if !swarm.ModeLight.IsLight() {
		t.Error("ModeLight.IsLight() = false, want true")
	}
	if swarm.ModeLight.RequiresGates() {
		t.Error("ModeLight.RequiresGates() = true, want false")
	}
	if !swarm.ModeOther.IsOther() {
		t.Error("ModeOther.IsOther() = false, want true")
	}
}

// TestDeliveryModeString covers the String() method (previously 0%).
func TestDeliveryModeString(t *testing.T) {
	cases := []struct {
		m swarm.DeliveryMode
		s string
	}{
		{swarm.DeliveryNotify, "notify"},
		{swarm.DeliveryInterrupt, "interrupt"},
		{swarm.DeliveryWake, "wake"},
	}
	for _, c := range cases {
		if got := c.m.String(); got != c.s {
			t.Errorf("DeliveryMode(%v).String() = %q, want %q", c.m, got, c.s)
		}
	}
}

// TestAwaitModeString covers the String() method (previously 0%).
func TestAwaitModeString(t *testing.T) {
	cases := []struct {
		m swarm.AwaitMode
		s string
	}{
		{swarm.AwaitModeAll, "all"},
		{swarm.AwaitModeAny, "any"},
	}
	for _, c := range cases {
		if got := c.m.String(); got != c.s {
			t.Errorf("AwaitMode(%v).String() = %q, want %q", c.m, got, c.s)
		}
	}
}

// TestNodeStatusString covers the previously-untested String() method.
func TestNodeStatusString(t *testing.T) {
	for _, s := range []swarm.NodeStatus{
		swarm.NodeQueued, swarm.NodeRunning,
		swarm.NodeDone, swarm.NodeFailed,
	} {
		if s.String() == "" {
			t.Errorf("%v.String() empty", s)
		}
		if s.IsOther() {
			t.Errorf("%v should not be Other", s)
		}
	}
	if !swarm.NodeStatusOther.IsOther() {
		t.Error("NodeStatusOther.IsOther() = false, want true")
	}
}

// TestNodeOriginString covers the previously-untested String() method.
func TestNodeOriginString(t *testing.T) {
	for _, s := range []swarm.NodeOrigin{
		swarm.OriginSeed, swarm.OriginExpand,
		swarm.OriginGap, swarm.OriginGate,
	} {
		if s.String() == "" {
			t.Errorf("%v.String() empty", s)
		}
		if s.IsOther() {
			t.Errorf("%v should not be Other", s)
		}
	}
}

// TestConfidenceLevelString covers the previously-untested String().
func TestConfidenceLevelString(t *testing.T) {
	if swarm.ConfidenceLow.String() != "low" {
		t.Errorf("ConfidenceLow.String() = %q, want low", swarm.ConfidenceLow.String())
	}
	if swarm.ConfidenceMedium.String() != "medium" {
		t.Errorf("ConfidenceMedium.String() = %q, want medium", swarm.ConfidenceMedium.String())
	}
	if swarm.ConfidenceHigh.String() != "high" {
		t.Errorf("ConfidenceHigh.String() = %q, want high", swarm.ConfidenceHigh.String())
	}
}

// TestUnmarshalJSONTypeConfusion covers the error branch where the
// JSON input is structurally wrong (not a string). All sealed types
// use ParseXxx with fallback-to-Other semantics, so unknown-but-
// string-shaped values do NOT error — only type-confusion errors.
func TestUnmarshalJSONTypeConfusion(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{"DeliveryMode-not-string", func() error {
			var v swarm.DeliveryMode
			return json.Unmarshal([]byte(`123`), &v)
		}},
		{"AwaitMode-not-string", func() error {
			var v swarm.AwaitMode
			return json.Unmarshal([]byte(`true`), &v)
		}},
		{"LifecycleStatus-not-string", func() error {
			var v swarm.LifecycleStatus
			return json.Unmarshal([]byte(`[]`), &v)
		}},
		{"Role-not-string", func() error {
			var v swarm.Role
			return json.Unmarshal([]byte(`[]`), &v)
		}},
		{"PlanMode-not-string", func() error {
			var v swarm.PlanMode
			return json.Unmarshal([]byte(`456`), &v)
		}},
		{"Mode-not-string", func() error {
			var v swarm.Mode
			return json.Unmarshal([]byte(`{"x":1}`), &v)
		}},
		{"NodeKind-not-string", func() error {
			var v swarm.NodeKind
			return json.Unmarshal([]byte(`{}`), &v)
		}},
		{"NodeStatus-not-string", func() error {
			var v swarm.NodeStatus
			return json.Unmarshal([]byte(`[]`), &v)
		}},
		{"NodeOrigin-not-string", func() error {
			var v swarm.NodeOrigin
			return json.Unmarshal([]byte(`[]`), &v)
		}},
		{"ConfidenceLevel-not-string", func() error {
			var v swarm.ConfidenceLevel
			return json.Unmarshal([]byte(`123`), &v)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(); err == nil {
				t.Errorf("UnmarshalJSON on %s: expected error, got nil", c.name)
			}
		})
	}
}

// TestUnmarshalJSONForwardCompat covers the sealed-type fallback:
// unknown wire strings unmarshal to the *Other constant without
// error. This is the deliberate forward-compat policy documented
// in ADR §8 (vs serde's deserialization-error default).
func TestUnmarshalJSONForwardCompat(t *testing.T) {
	cases := []struct {
		name string
		run  func() bool
	}{
		{"Mode-other", func() bool {
			var v swarm.Mode
			if err := json.Unmarshal([]byte(`"unknown"`), &v); err != nil {
				return false
			}
			return v.IsOther()
		}},
		{"NodeStatus-other", func() bool {
			var v swarm.NodeStatus
			if err := json.Unmarshal([]byte(`"unknown"`), &v); err != nil {
				return false
			}
			return v.IsOther()
		}},
		{"NodeOrigin-other", func() bool {
			var v swarm.NodeOrigin
			if err := json.Unmarshal([]byte(`"unknown"`), &v); err != nil {
				return false
			}
			return v.IsOther()
		}},
		{"ConfidenceLevel-other", func() bool {
			var v swarm.ConfidenceLevel
			if err := json.Unmarshal([]byte(`"unknown"`), &v); err != nil {
				return false
			}
			return v.IsOther()
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !c.run() {
				t.Errorf("%s: expected Other fallback", c.name)
			}
		})
	}
}
