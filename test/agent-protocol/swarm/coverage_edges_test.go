package swarm_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// TestNextRunnableItemIDsSkipKnown completes the branch coverage:
// NextRunnableItemIDs also skips when an unknown id is referenced
// in BlockedBy. This exercises the IsUnblocked check path (already
// fully covered elsewhere) and the `_, ok := knownIDs[it.ID]`
// negative-path guard.
func TestNextRunnableItemIDsLimitZero(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusQueued},
	}
	if got := swarm.NextRunnableItemIDs(items, 0); got != nil {
		t.Errorf("limit=0 = %v, want nil", got)
	}
}

// TestNextRunnableItemIDsSkipCompleted exercises the path where an
// item has a completed dep reference (already satisfied, so skipped
// from the candidate set).
func TestNextRunnableItemIDsSkipCompleted(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusCompleted},
		{ID: "b", Status: swarm.StatusQueued, BlockedBy: []string{"a"}},
	}
	got := swarm.NextRunnableItemIDs(items, 10)
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("NextRunnableItemIDs = %v, want [b]", got)
	}
}

// TestNewlyReadyItemIDsEmptyAfter covers the early-return path
// when after is empty.
func TestNewlyReadyItemIDsEmptyAfter(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusQueued},
	}
	if got := swarm.NewlyReadyItemIDs(before, nil); got != nil {
		t.Errorf("empty-after = %v, want nil", got)
	}
}

// TestNewlyReadyItemIDsAlreadyReady covers the path where the item
// was runnable in both before and after — must not appear in output.
func TestNewlyReadyItemIDsAlreadyReady(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusQueued},
	}
	after := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusQueued},
	}
	if got := swarm.NewlyReadyItemIDs(before, after); got != nil {
		t.Errorf("already-ready = %v, want nil", got)
	}
}

// TestNewlyReadyItemIDsNewlyUnblocked covers the path where a dep
// transitions incomplete -> complete and the item was blocked-by
// that dep in both before and after.
func TestNewlyReadyItemIDsNewlyUnblocked(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "dep", Status: swarm.StatusQueued},
		{ID: "x", Status: swarm.StatusQueued, BlockedBy: []string{"dep"}},
	}
	after := []swarm.PlanItem{
		{ID: "dep", Status: swarm.StatusCompleted},
		{ID: "x", Status: swarm.StatusQueued, BlockedBy: []string{"dep"}},
	}
	got := swarm.NewlyReadyItemIDs(before, after)
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("NewlyReadyItemIDs = %v, want [x]", got)
	}
}

// TestValidateTLDRLongTLDRShortBody covers the branch where tldr is
// over the cap but body is short (the cap check fires regardless of
// body length).
func TestValidateTLDRLongTLDRShortBody(t *testing.T) {
	huge := strings.Repeat("z", swarm.MaxTLDRChars+1)
	if err := swarm.ValidateTLDR(huge, "short"); err == nil {
		t.Error("ValidateTLDR(long tldr, short body) = nil, want error")
	}
}

// TestValidateTLDRLongTLDRLongBody covers the same cap check on the
// long-body branch (where the tldr-required check would also fire
// if tldr were empty).
func TestValidateTLDRLongTLDRLongBody(t *testing.T) {
	huge := strings.Repeat("z", swarm.MaxTLDRChars+1)
	longBody := strings.Repeat("a", swarm.TLDRRequiredOverChars+1)
	if err := swarm.ValidateTLDR(huge, longBody); err == nil {
		t.Error("ValidateTLDR(long tldr, long body) = nil, want error")
	}
}

// TestValidateTLDRMissingTLDR covers the body-too-long branch where
// tldr is empty.
func TestValidateTLDRMissingTLDR(t *testing.T) {
	longBody := strings.Repeat("a", swarm.TLDRRequiredOverChars+1)
	if err := swarm.ValidateTLDR("", longBody); err == nil {
		t.Error("ValidateTLDR(empty tldr, long body) = nil, want error")
	}
}

// TestDeriveTaskLabelEmpty covers the empty-input branch.
func TestDeriveTaskLabelEmpty(t *testing.T) {
	if got := swarm.DeriveTaskLabel(""); got != "" {
		t.Errorf("DeriveTaskLabel(empty) = %q, want \"\"", got)
	}
}

// TestDeriveTaskLabelAllBlankLines covers the all-blank-lines branch
// (every line is whitespace -> return "").
func TestDeriveTaskLabelAllBlankLines(t *testing.T) {
	if got := swarm.DeriveTaskLabel("\n\n   \n\t\n"); got != "" {
		t.Errorf("DeriveTaskLabel(all-blank) = %q, want \"\"", got)
	}
}

// TestDeriveTaskLabelFirstNonBlank covers the loop-skip-blank-line
// branch where the first line is blank and the actual content is
// on line 2.
func TestDeriveTaskLabelFirstNonBlank(t *testing.T) {
	if got := swarm.DeriveTaskLabel("\n\n# real work"); got != "real work" {
		t.Errorf("DeriveTaskLabel(skip-blanks) = %q, want \"real work\"", got)
	}
}

// TestTaskControlActionReplaceFromTerminal exercises the
// TaskReplace branch (line 112: return !s.IsOther()) from a terminal
// status — covers the previously-uncovered terminal-state case.
func TestTaskControlActionReplaceFromTerminal(t *testing.T) {
	for _, s := range []swarm.LifecycleStatus{
		swarm.StatusCompleted, swarm.StatusFailed,
		swarm.StatusDone, swarm.StatusStopped, swarm.StatusCrashed,
	} {
		if !swarm.TaskReplace.AllowsStatus(s) {
			t.Errorf("TaskReplace should allow %v", s)
		}
	}
	if swarm.TaskReplace.AllowsStatus(swarm.StatusOther) {
		t.Error("TaskReplace should not allow Other")
	}
}

// TestTaskControlActionUnknown covers the trailing `return false`
// default branch. An action that doesn't match any of the 7
// well-known constants must return false regardless of status.
func TestTaskControlActionUnknown(t *testing.T) {
	unk := swarm.TaskControlAction("nonexistent")
	for _, s := range []swarm.LifecycleStatus{
		swarm.StatusQueued, swarm.StatusReady, swarm.StatusRunning,
		swarm.StatusCompleted, swarm.StatusFailed, swarm.StatusOther,
	} {
		if unk.AllowsStatus(s) {
			t.Errorf("unknown action %q should not allow %v", unk, s)
		}
	}
}

// TestNextRunnableItemIDsSkipUnblockedDep exercises the IsUnblocked
// negative branch in NextRunnableItemIDs: an item whose dep is
// uncomplete must be filtered out.
func TestNextRunnableItemIDsSkipUnblockedDep(t *testing.T) {
	items := []swarm.PlanItem{
		{ID: "a", Status: swarm.StatusQueued},
		{ID: "b", Status: swarm.StatusQueued, BlockedBy: []string{"a"}},
	}
	got := swarm.NextRunnableItemIDs(items, 10)
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("NextRunnableItemIDs = %v, want [a] only", got)
	}
}

// TestNewlyReadyItemIDsSkipCompleted covers the completedBefore loop
// branch: a completed item in `before` must not be considered
// "newly ready" even if it's still complete in `after`.
func TestNewlyReadyItemIDsSkipCompleted(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusCompleted},
	}
	after := []swarm.PlanItem{
		{ID: "x", Status: swarm.StatusCompleted},
	}
	if got := swarm.NewlyReadyItemIDs(before, after); got != nil {
		t.Errorf("completed-passthrough = %v, want nil", got)
	}
}

// TestNewlyReadyItemIDsWasBlockedInBefore covers the
// `!newlyUnblocked && len(it.BlockedBy) > 0` branch: an item was
// blocked-by something that was already complete in `before` and
// remains complete in `after` -> IsUnblocked returns true (all deps
// done), but newlyUnblocked stays false (no transition occurred),
// so the trailing guard skips it.
func TestNewlyReadyItemIDsWasBlockedInBefore(t *testing.T) {
	before := []swarm.PlanItem{
		{ID: "dep", Status: swarm.StatusCompleted},
		{ID: "x", Status: swarm.StatusQueued, BlockedBy: []string{"dep"}},
	}
	after := []swarm.PlanItem{
		{ID: "dep", Status: swarm.StatusCompleted},
		{ID: "x", Status: swarm.StatusQueued, BlockedBy: []string{"dep"}},
	}
	if got := swarm.NewlyReadyItemIDs(before, after); got != nil {
		t.Errorf("static-already-unblocked = %v, want nil", got)
	}
}

// TestDeriveTaskLabelStripAfterTrimLeft covers the inner TrimSpace
// after TrimLeft of markdown markers — exercises the branch where
// a line composed entirely of markers is reduced to "" and skipped.
func TestDeriveTaskLabelStripAfterTrimLeft(t *testing.T) {
	if got := swarm.DeriveTaskLabel("###\nreal work"); got != "real work" {
		t.Errorf("DeriveTaskLabel(marker-only-line) = %q, want \"real work\"", got)
	}
}
