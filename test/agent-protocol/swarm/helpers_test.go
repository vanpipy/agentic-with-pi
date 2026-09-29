package swarm_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// fixtures used by helpers_test.go
func TestPriorityRank(t *testing.T) {
	cases := []struct {
		in   string
		want uint8
	}{
		{"high", 3},
		{"HIGH", 3},
		{" High ", 3},
		{"medium", 2},
		{"low", 1},
		{"LOW", 1},
		{"", 0},
		{"unknown", 0},
		{"critical", 0},
	}
	for _, c := range cases {
		if got := swarm.PriorityRank(c.in); got != c.want {
			t.Errorf("PriorityRank(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestValidateTLDR(t *testing.T) {
	// Short body — TLDR not required, but TLDR must still be <= cap.
	short := strings.Repeat("a", swarm.TLDRRequiredOverChars-1)
	if err := swarm.ValidateTLDR("", short); err != nil {
		t.Errorf("ValidateTLDR('', short) = %v, want nil", err)
	}
	// Long body without TLDR — must error.
	long := strings.Repeat("a", swarm.TLDRRequiredOverChars+1)
	if err := swarm.ValidateTLDR("", long); err == nil {
		t.Error("ValidateTLDR('', long) = nil, want error")
	}
	// Long body with TLDR — ok.
	if err := swarm.ValidateTLDR("summary", long); err != nil {
		t.Errorf("ValidateTLDR('summary', long) = %v, want nil", err)
	}
	// TLDR too long.
	huge := strings.Repeat("z", swarm.MaxTLDRChars+1)
	if err := swarm.ValidateTLDR(huge, short); err == nil {
		t.Error("ValidateTLDR(too-long-tldr, short) = nil, want error")
	}
	// Boundary: exactly TLDRRequiredOverChars — no TLDR required.
	boundary := strings.Repeat("a", swarm.TLDRRequiredOverChars)
	if err := swarm.ValidateTLDR("", boundary); err != nil {
		t.Errorf("ValidateTLDR('', boundary) = %v, want nil (boundary inclusive)", err)
	}
	// One over boundary — TLDR required.
	if err := swarm.ValidateTLDR("", boundary+"a"); err == nil {
		t.Error("ValidateTLDR('', boundary+1) = nil, want error")
	}
}

func TestDeriveTaskLabel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "implement auth", "implement auth"},
		{"leading-hash", "# implement auth", "implement auth"},
		{"leading-dash", "- implement auth", "implement auth"},
		{"leading-bullet", "* implement auth", "implement auth"},
		{"leading-quote", "> implement auth", "implement auth"},
		{"markdown-bold", "**implement auth**", "implement auth"},
		{"code-quote", "`fix bug`", "fix bug"},
		{"first-line-only", "implement auth\n\nmore details", "implement auth"},
		{"whitespace-only-first", "\n\n\nreal task", "real task"},
		{"strip-punct", "fix: bug", "fix bug"},
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

func TestDeriveTaskLabelTruncation(t *testing.T) {
	// Build a long string and verify it gets truncated with ellipsis.
	long := strings.Repeat("a", swarm.MaxTaskLabelChars*2)
	got := swarm.DeriveTaskLabel(long)
	runeCount := len([]rune(got))
	if runeCount > swarm.MaxTaskLabelChars {
		t.Errorf("rune count of DeriveTaskLabel(long) = %d > MaxTaskLabelChars (%d)", runeCount, swarm.MaxTaskLabelChars)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix on truncation, got %q", got)
	}
}

// TestSummarizePlanGraph exercises the bucket-sort over a 4-node
// plan with one of each lifecycle state.
func TestSummarizePlanGraph(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "t1", Status: swarm.StatusCompleted},
		{ID: "t2", Status: swarm.StatusFailed},
		{ID: "t3", Status: swarm.StatusRunning},
		{ID: "t4", Status: swarm.StatusBlocked, BlockedBy: []string{"t1"}},
	}
	got := swarm.SummarizePlanGraph(items)
	if len(got.CompletedIDs) != 1 || got.CompletedIDs[0] != "t1" {
		t.Errorf("CompletedIDs = %v, want [t1]", got.CompletedIDs)
	}
	if len(got.FailedIDs) != 1 || got.FailedIDs[0] != "t2" {
		t.Errorf("FailedIDs = %v, want [t2]", got.FailedIDs)
	}
	if len(got.ActiveIDs) != 1 || got.ActiveIDs[0] != "t3" {
		t.Errorf("ActiveIDs = %v, want [t3]", got.ActiveIDs)
	}
	if len(got.BlockedIDs) != 1 || got.BlockedIDs[0] != "t4" {
		t.Errorf("BlockedIDs = %v, want [t4]", got.BlockedIDs)
	}
	if len(got.TerminalIDs) != 2 {
		t.Errorf("TerminalIDs = %v, want 2 entries", got.TerminalIDs)
	}
}

func TestSummarizePlanGraphNilEmpty(t *testing.T) {
	if got := swarm.SummarizePlanGraph(nil); got.CompletedIDs != nil {
		t.Errorf("nil plan -> CompletedIDs = %v, want nil", got.CompletedIDs)
	}
	if got := swarm.SummarizePlanGraph([]swarm.PlanItem{}); got.CompletedIDs != nil {
		t.Errorf("empty plan -> CompletedIDs = %v, want nil", got.CompletedIDs)
	}
}

func TestNextRunnableItemIDsSorted(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusQueued, Priority: "low"},
		{ID: "b", Status: swarm.StatusQueued, Priority: "high"},
		{ID: "c", Status: swarm.StatusQueued, Priority: "high"},
		{ID: "d", Status: swarm.StatusRunning, Priority: "high"},
	}
	got := swarm.NextRunnableItemIDs(items, 10)
	want := []string{"b", "c", "a"}
	if !equalStrings(got, want) {
		t.Errorf("NextRunnableItemIDs = %v, want %v", got, want)
	}
}

func TestNextRunnableItemIDsLimit(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusQueued, Priority: "high"},
		{ID: "b", Status: swarm.StatusQueued, Priority: "high"},
		{ID: "c", Status: swarm.StatusQueued, Priority: "high"},
	}
	got := swarm.NextRunnableItemIDs(items, 2)
	if len(got) != 2 {
		t.Errorf("len(NextRunnableItemIDs(limit=2)) = %d, want 2", len(got))
	}
	if got := swarm.NextRunnableItemIDs(items, 0); got != nil {
		t.Errorf("NextRunnableItemIDs(limit=0) = %v, want nil", got)
	}
}

func TestNewlyReadyItemIDs(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusRunning},
		{ID: "b", Status: swarm.StatusQueued, BlockedBy: []string{"a"}},
	}
	after := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusCompleted},
		{ID: "b", Status: swarm.StatusQueued, BlockedBy: []string{"a"}},
	}
	got := swarm.NewlyReadyItemIDs(before, after)
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("NewlyReadyItemIDs = %v, want [b]", got)
	}
}

func TestNewlyReadyItemIDsNoChange(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusQueued, BlockedBy: []string{"b"}},
		{ID: "b", Status: swarm.StatusQueued, BlockedBy: []string{"a"}},
	}
	got := swarm.NewlyReadyItemIDs(before, before)
	if got != nil {
		t.Errorf("no-change diff = %v, want nil", got)
	}
}

// TestCycleItemIDs exercises 6 fixtures: no cycle, simple cycle, two
// cycles, self-cycle, fan-out, fan-in.
func TestCycleItemIDs(t *testing.T) {
	cases := []struct {
		name  string
		items []swarm.PlanItem
		want  []string
	}{
		{
			name: "no-cycle-line",
			items: []swarm.PlanItem{
				{ID: "a", BlockedBy: []string{}},
				{ID: "b", BlockedBy: []string{"a"}},
				{ID: "c", BlockedBy: []string{"b"}},
			},
			want: nil,
		},
		{
			name: "simple-cycle",
			items: []swarm.PlanItem{
				{ID: "a", BlockedBy: []string{"c"}},
				{ID: "b", BlockedBy: []string{"a"}},
				{ID: "c", BlockedBy: []string{"b"}},
			},
			want: []string{"a", "b", "c"},
		},
		{
			name: "self-cycle",
			items: []swarm.PlanItem{
				{ID: "a", BlockedBy: []string{"a"}},
			},
			want: []string{"a"},
		},
		{
			name: "two-cycles",
			items: []swarm.PlanItem{
				{ID: "a", BlockedBy: []string{"b"}},
				{ID: "b", BlockedBy: []string{"a"}},
				{ID: "c", BlockedBy: []string{"d"}},
				{ID: "d", BlockedBy: []string{"c"}},
			},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "fan-out",
			items: []swarm.PlanItem{
				{ID: "root"},
				{ID: "x", BlockedBy: []string{"root"}},
				{ID: "y", BlockedBy: []string{"root"}},
				{ID: "z", BlockedBy: []string{"root"}},
			},
			want: nil,
		},
		{
			name: "fan-in",
			items: []swarm.PlanItem{
				{ID: "a"},
				{ID: "b"},
				{ID: "c", BlockedBy: []string{"a", "b"}},
			},
			want: nil,
		},
		{
			name:  "empty",
			items: []swarm.PlanItem{},
			want:  nil,
		},
		{
			name: "partial-cycle",
			items: []swarm.PlanItem{
				{ID: "ok1"},
				{ID: "ok2", BlockedBy: []string{"ok1"}},
				{ID: "loop1", BlockedBy: []string{"loop2"}},
				{ID: "loop2", BlockedBy: []string{"loop1"}},
			},
			want: []string{"loop1", "loop2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := swarm.CycleItemIDs(c.items)
			if !equalStrings(got, c.want) {
				t.Errorf("CycleItemIDs = %v, want %v", got, c.want)
			}
		})
	}
}

func TestUnresolvedDependencies(t *testing.T) {
	item := swarm.PlanItem{ID: "x", BlockedBy: []string{"a", "b", "c"}}
	known := map[string]struct{}{"a": {}, "c": {}}
	got := swarm.UnresolvedDependencies(item, known)
	if !equalStrings(got, []string{"b"}) {
		t.Errorf("UnresolvedDependencies = %v, want [b]", got)
	}
	if got := swarm.MissingDependencies(item, known); !equalStrings(got, []string{"b"}) {
		t.Errorf("MissingDependencies = %v, want [b]", got)
	}
}

func TestIsUnblocked(t *testing.T) {
	item := swarm.PlanItem{ID: "x", BlockedBy: []string{"a", "b"}}
	allDone := map[string]struct{}{"a": {}, "b": {}}
	if !swarm.IsUnblocked(item, allDone) {
		t.Error("IsUnblocked(allDone) = false, want true")
	}
	missing := map[string]struct{}{"a": {}}
	if swarm.IsUnblocked(item, missing) {
		t.Error("IsUnblocked(missing) = true, want false")
	}
	noDeps := swarm.PlanItem{ID: "x"}
	if !swarm.IsUnblocked(noDeps, map[string]struct{}{}) {
		t.Error("IsUnblocked(noDeps, empty) = false, want true")
	}
}

func TestRFC3339NanoNow(t *testing.T) {
	got := swarm.RFC3339NanoNow()
	if got == "" {
		t.Error("RFC3339NanoNow() returned empty string")
	}
	if !strings.Contains(got, "T") {
		t.Errorf("RFC3339NanoNow() = %q, want RFC3339Nano format", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
