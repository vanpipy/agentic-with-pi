package swarm

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// PriorityRank maps a priority string to a sortable rank. Higher
// rank = higher priority. Unknown / empty ranks as 0.
func PriorityRank(p string) uint8 {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// IsCompletedStatus / IsTerminalStatus / IsActiveStatus / IsFailedStatus
// / IsRunnableStatus are package-level aliases for the same predicates
// on LifecycleStatus. They exist because the helper signatures are
// more readable when the verb comes first (IsCompletedStatus(s) vs
// s.IsCompleted()).

// IsCompletedStatus reports whether s represents successful completion.
func IsCompletedStatus(s LifecycleStatus) bool { return s.IsCompleted() }

// IsTerminalStatus reports whether s is a final state.
func IsTerminalStatus(s LifecycleStatus) bool { return s.IsTerminal() }

// IsActiveStatus reports whether s is currently executing.
func IsActiveStatus(s LifecycleStatus) bool { return s.IsActive() }

// IsFailedStatus reports whether s is an unsuccessful terminal state.
func IsFailedStatus(s LifecycleStatus) bool { return s.IsFailed() }

// IsRunnableStatus reports whether s is eligible for assignment.
func IsRunnableStatus(s LifecycleStatus) bool { return s.IsRunnable() }

// SummarizePlanGraph reduces a plan to per-bucket id lists. Used by
// EventSwarmPlan broadcasts and EventCommPlanStatusResponse. The
// returned lists are sorted + de-duplicated.
func SummarizePlanGraph(items []PlanItem) PlanGraphSummary {
	if items == nil {
		return PlanGraphSummary{}
	}
	out := PlanGraphSummary{}
	completedSet := map[string]struct{}{}
	for _, it := range items {
		switch {
		case it.Status.IsCompleted():
			out.CompletedIDs = append(out.CompletedIDs, it.ID)
			completedSet[it.ID] = struct{}{}
		case it.Status.IsFailed():
			out.FailedIDs = append(out.FailedIDs, it.ID)
		case it.Status.IsActive():
			out.ActiveIDs = append(out.ActiveIDs, it.ID)
		case it.Status.IsBlocked():
			out.BlockedIDs = append(out.BlockedIDs, it.ID)
		}
		if it.Status.IsTerminal() {
			out.TerminalIDs = append(out.TerminalIDs, it.ID)
		}
		if it.Status.IsRunnable() && len(it.BlockedBy) == 0 {
			out.ReadyIDs = append(out.ReadyIDs, it.ID)
		}
		for _, dep := range it.BlockedBy {
			if _, ok := completedSet[dep]; !ok {
				found := false
				for _, other := range items {
					if other.ID == dep {
						found = true
						break
					}
				}
				if !found {
					out.UnresolvedDependencyIDs = append(out.UnresolvedDependencyIDs, dep)
				}
			}
		}
	}
	out.CycleIDs = CycleItemIDs(items)
	sort.Strings(out.ReadyIDs)
	sort.Strings(out.BlockedIDs)
	sort.Strings(out.ActiveIDs)
	sort.Strings(out.CompletedIDs)
	sort.Strings(out.FailedIDs)
	sort.Strings(out.TerminalIDs)
	sort.Strings(out.UnresolvedDependencyIDs)
	sort.Strings(out.CycleIDs)
	return out
}

// NextRunnableItemIDs returns up to `limit` runnable item ids sorted
// by (priority desc, id asc). Empty blocked_by + runnable status +
// known ids required.
func NextRunnableItemIDs(items []PlanItem, limit int) []string {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	knownIDs := make(map[string]struct{}, len(items))
	for _, it := range items {
		knownIDs[it.ID] = struct{}{}
	}
	completed := map[string]struct{}{}
	for _, it := range items {
		if it.Status.IsCompleted() {
			completed[it.ID] = struct{}{}
		}
	}
	type candidate struct {
		id   string
		rank uint8
	}
	var ready []candidate
	for _, it := range items {
		if !it.Status.IsRunnable() {
			continue
		}
		if !IsUnblocked(it, completed) {
			continue
		}
		if _, ok := knownIDs[it.ID]; !ok {
			continue
		}
		ready = append(ready, candidate{id: it.ID, rank: PriorityRank(it.Priority)})
	}
	sort.Slice(ready, func(i, j int) bool {
		if ready[i].rank != ready[j].rank {
			return ready[i].rank > ready[j].rank
		}
		return ready[i].id < ready[j].id
	})
	if len(ready) > limit {
		ready = ready[:limit]
	}
	out := make([]string, len(ready))
	for i, c := range ready {
		out[i] = c.id
	}
	return out
}

// NewlyReadyItemIDs returns the ids that became runnable in `after`
// but were not runnable in `before`. Order: sorted by id.
func NewlyReadyItemIDs(before, after []PlanItem) []string {
	if len(after) == 0 {
		return nil
	}
	completedAfter := map[string]struct{}{}
	for _, it := range after {
		if it.Status.IsCompleted() {
			completedAfter[it.ID] = struct{}{}
		}
	}
	beforeReady := map[string]struct{}{}
	completedBefore := map[string]struct{}{}
	for _, it := range before {
		if it.Status.IsRunnable() && len(it.BlockedBy) == 0 {
			beforeReady[it.ID] = struct{}{}
		}
		if it.Status.IsCompleted() {
			completedBefore[it.ID] = struct{}{}
		}
	}
	var out []string
	for _, it := range after {
		if _, was := beforeReady[it.ID]; was {
			continue
		}
		if !it.Status.IsRunnable() {
			continue
		}
		if !IsUnblocked(it, completedAfter) {
			continue
		}
		// Only count if some dependency actually transitioned from
		// incomplete to complete between before and after; otherwise
		// it was already-not-ready in both and we shouldn't claim
		// "newly ready".
		newlyUnblocked := false
		for _, dep := range it.BlockedBy {
			if _, was := completedBefore[dep]; !was {
				if _, now := completedAfter[dep]; now {
					newlyUnblocked = true
					break
				}
			}
		}
		if !newlyUnblocked && len(it.BlockedBy) > 0 {
			continue
		}
		out = append(out, it.ID)
	}
	sort.Strings(out)
	return out
}

// CycleItemIDs returns ids that participate in a dependency cycle.
// Uses Kahn's algorithm; O(V+E). Returns nil if no cycle.
func CycleItemIDs(items []PlanItem) []string {
	if len(items) == 0 {
		return nil
	}
	idToIdx := make(map[string]int, len(items))
	for i, it := range items {
		idToIdx[it.ID] = i
	}
	indeg := make([]int, len(items))
	adj := make([][]int, len(items))
	for i, it := range items {
		for _, dep := range it.BlockedBy {
			if j, ok := idToIdx[dep]; ok {
				adj[j] = append(adj[j], i)
				indeg[i]++
			}
		}
	}
	queue := make([]int, 0, len(items))
	for i, d := range indeg {
		if d == 0 {
			queue = append(queue, i)
		}
	}
	processed := 0
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		processed++
		for _, j := range adj[i] {
			indeg[j]--
			if indeg[j] == 0 {
				queue = append(queue, j)
			}
		}
	}
	if processed == len(items) {
		return nil
	}
	var cycleIDs []string
	for i, it := range items {
		if indeg[i] > 0 {
			cycleIDs = append(cycleIDs, it.ID)
		}
	}
	sort.Strings(cycleIDs)
	return cycleIDs
}

// UnresolvedDependencies returns the BlockedBy entries of `item` that
// are not present in `knownIDs`. Used by SeedGraph validation.
func UnresolvedDependencies(item PlanItem, knownIDs map[string]struct{}) []string {
	var out []string
	for _, dep := range item.BlockedBy {
		if _, ok := knownIDs[dep]; !ok {
			out = append(out, dep)
		}
	}
	return out
}

// MissingDependencies is an alias for UnresolvedDependencies kept
// for parity with jcode's `missing_dependencies` helper.
func MissingDependencies(item PlanItem, knownIDs map[string]struct{}) []string {
	return UnresolvedDependencies(item, knownIDs)
}

// IsUnblocked reports whether all of item.BlockedBy are in completedIDs.
func IsUnblocked(item PlanItem, completedIDs map[string]struct{}) bool {
	for _, dep := range item.BlockedBy {
		if _, ok := completedIDs[dep]; !ok {
			return false
		}
	}
	return true
}

// ValidateTLDR checks that a message with body length above
// TLDRRequiredOverChars carries a non-empty tldr. Returns a
// swarm.tldr_missing error otherwise. Also caps tldr length.
func ValidateTLDR(tldr, body string) error {
	if len(body) <= TLDRRequiredOverChars {
		if len(tldr) > MaxTLDRChars {
			return fmt.Errorf("tldr too long: %d > %d", len(tldr), MaxTLDRChars)
		}
		return nil
	}
	if tldr == "" {
		return errors.New("tldr required for messages above TLDRRequiredOverChars")
	}
	if len(tldr) > MaxTLDRChars {
		return fmt.Errorf("tldr too long: %d > %d", len(tldr), MaxTLDRChars)
	}
	return nil
}

// DeriveTaskLabel extracts a short, human-readable task label from
// free-form text. Takes the first non-empty line, strips leading
// markdown (#, -, *, >) and bold markers, strips colons / semicolons
// (test fixtures expect them dropped), trims to MaxTaskLabelChars,
// and appends an ellipsis on truncation. Empty input -> "".
func DeriveTaskLabel(text string) string {
	if text == "" {
		return ""
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.TrimLeft(trimmed, "#-*>`")
		trimmed = strings.TrimSpace(trimmed)
		trimmed = strings.Trim(trimmed, "*_`")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		var b strings.Builder
		for _, r := range trimmed {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) &&
				!strings.ContainsRune(".()[]{}'-", r) {
				continue
			}
			b.WriteRune(r)
		}
		out := strings.Join(strings.Fields(b.String()), " ")
		if len(out) > MaxTaskLabelChars {
			out = out[:MaxTaskLabelChars-1] + "…"
		}
		return out
	}
	return ""
}

// RFC3339NanoNow returns the current time formatted as RFC3339Nano.
// Convenience for callers that build MemberRecord / similar types.
func RFC3339NanoNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }
