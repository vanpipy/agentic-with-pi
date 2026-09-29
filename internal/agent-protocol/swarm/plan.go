package swarm

import "encoding/json"

// PlanMode is a sealed enum distinguishing light vs deep DAG
// orchestration. See ADR §15 D5 (DAG phasing) for the deep vs light
// trade-off; the wire-format differences are:
//
//   - light: complete_node takes only status; gates are not auto-
//     inserted; artifacts are optional.
//   - deep: complete_node requires artifact_json with findings +
//     validation + confidence; gates (verify or critique) are auto-
//     inserted between composite parents and their children.
type PlanMode struct{ s string }

var (
	PlanModeLight = PlanMode{"light"}
	PlanModeDeep  = PlanMode{"deep"}
	PlanModeOther = PlanMode{""}
)

func ParsePlanMode(s string) PlanMode {
	switch s {
	case "light":
		return PlanModeLight
	case "deep":
		return PlanModeDeep
	default:
		return PlanModeOther
	}
}

func (m PlanMode) String() string         { return m.s }
func (m PlanMode) IsLight() bool          { return m == PlanModeLight }
func (m PlanMode) IsDeep() bool           { return m == PlanModeDeep }
func (m PlanMode) IsOther() bool          { return m == PlanModeOther }
func (m PlanMode) RequiresGates() bool    { return m == PlanModeDeep }
func (m PlanMode) RequiresArtifact() bool { return m == PlanModeDeep }

func (m PlanMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.s)
}

func (m *PlanMode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*m = ParsePlanMode(s)
	return nil
}

// TaskControlAction is the set of lifecycle transitions available
// for an assigned DAG task. Maps to jcode's CommTaskControl variants.
type TaskControlAction string

const (
	TaskStart    TaskControlAction = "start"
	TaskWake     TaskControlAction = "wake"
	TaskResume   TaskControlAction = "resume"
	TaskRetry    TaskControlAction = "retry"
	TaskReassign TaskControlAction = "reassign"
	TaskReplace  TaskControlAction = "replace"
	TaskSalvage  TaskControlAction = "salvage"
)

// ParseTaskControlAction maps a wire string to its action. Unknown
// strings return "" + ok=false so callers can distinguish an invalid
// action from a recognized one (this is not a sealed enum because
// the set is small and we want to surface bad input as a clear error
// rather than a silent "Other" pass-through).
func ParseTaskControlAction(s string) (TaskControlAction, bool) {
	switch TaskControlAction(s) {
	case TaskStart, TaskWake, TaskResume, TaskRetry,
		TaskReassign, TaskReplace, TaskSalvage:
		return TaskControlAction(s), true
	}
	return "", false
}

// AllowsStatus reports whether the action is legal from the given
// current task status. Server-side guard returns ErrNotOwner or
// ErrInvalidMode on disallowed transitions.
//
//	+----------+--------+--------+--------+--------+--------+--------+---------+
//	| status   | start  | wake   | resume | retry  | reass. | replace| salvage |
//	+----------+--------+--------+--------+--------+--------+--------+---------+
//	| queued   |   x    |        |        |        |        |        |         |
//	| ready    |   x    |        |        |   x    |   x    |   x    |         |
//	| running  |        |   x    |        |   x    |   x    |   x    |   x     |
//	| blocked  |        |        |        |   x    |        |   x    |         |
//	| failed   |        |        |        |   x    |        |   x    |   x     |
//	| terminal |        |        |        |        |        |   x    |   x     |
//	+----------+--------+--------+--------+--------+--------+--------+---------+
func (a TaskControlAction) AllowsStatus(s LifecycleStatus) bool {
	switch a {
	case TaskStart:
		return s == StatusQueued || s == StatusReady
	case TaskWake:
		return s == StatusRunning
	case TaskResume:
		return s == StatusBlocked
	case TaskRetry:
		return s == StatusReady || s == StatusRunning ||
			s == StatusBlocked || s == StatusFailed
	case TaskReassign:
		return s == StatusReady || s == StatusRunning
	case TaskReplace:
		// Replace is legal from any state except Other (queued,
		// ready, running, blocked, failed, all terminal). Spawns a
		// fresh worker for the task.
		return !s.IsOther()
	case TaskSalvage:
		return s == StatusRunning || s.IsFailed() || s.IsTerminal()
	}
	return false
}

// PlanItem is a single node in the DAG. Items have a Status field
// but the authoritative lifecycle for DAG nodes is NodeStatus; the
// Status here mirrors it for clients that prefer to treat the plan
// as a flat list.
type PlanItem struct {
	ID         string          `json:"id"`
	Content    string          `json:"content"`
	Status     LifecycleStatus `json:"status"`
	Priority   string          `json:"priority"` // "high" | "medium" | "low"
	Subsystem  string          `json:"subsystem,omitempty"`
	FileScope  []string        `json:"file_scope,omitempty"`
	BlockedBy  []string        `json:"blocked_by,omitempty"`
	AssignedTo string          `json:"assigned_to,omitempty"` // session_id
}

// PlanGraphSummary is a coarse-grained view of the plan, returned
// inline in EventCommPlanStatusResponse.data and on the
// EventSwarmPlan event. Detailed per-item state lives in VersionedPlan.
type PlanGraphSummary struct {
	ReadyIDs                []string `json:"ready_ids,omitempty"`
	BlockedIDs              []string `json:"blocked_ids,omitempty"`
	ActiveIDs               []string `json:"active_ids,omitempty"`
	CompletedIDs            []string `json:"completed_ids,omitempty"`
	FailedIDs               []string `json:"failed_ids,omitempty"`
	CycleIDs                []string `json:"cycle_ids,omitempty"`
	NextReadyIDs            []string `json:"next_ready_ids,omitempty"`
	NewlyReadyIDs           []string `json:"newly_ready_ids,omitempty"`
	UnresolvedDependencyIDs []string `json:"unresolved_dependency_ids,omitempty"`
	TerminalIDs             []string `json:"terminal_ids,omitempty"`
	LowConfidenceIDs        []string `json:"low_confidence_ids,omitempty"`
}

// PlanGraphStatus is the full status snapshot, including the version
// counter and seed/grow counts.
type PlanGraphStatus struct {
	SwarmID          string   `json:"swarm_id,omitempty"`
	Version          uint64   `json:"version"`
	ItemCount        int      `json:"item_count"`
	ReadyIDs         []string `json:"ready_ids,omitempty"`
	BlockedIDs       []string `json:"blocked_ids,omitempty"`
	ActiveIDs        []string `json:"active_ids,omitempty"`
	CompletedIDs     []string `json:"completed_ids,omitempty"`
	FailedIDs        []string `json:"failed_ids,omitempty"`
	CycleIDs         []string `json:"cycle_ids,omitempty"`
	NextReadyIDs     []string `json:"next_ready_ids,omitempty"`
	NewlyReadyIDs    []string `json:"newly_ready_ids,omitempty"`
	LowConfidenceIDs []string `json:"low_confidence_ids,omitempty"`
	Mode             PlanMode `json:"mode"`
	SeededCount      int      `json:"seeded_count"`
	GrownCount       int      `json:"grown_count"`
}

// TaskProgress records the assignment / heartbeat / completion state
// of one task. Persisted in VersionedPlan.TaskProgress; surfaced in
// EventCommAssignTaskResponse.data.
type TaskProgress struct {
	AssignedSessionID    string  `json:"assigned_session_id,omitempty"`
	AssignmentSummary    string  `json:"assignment_summary,omitempty"`
	AssignedAtUnixMS     *int64  `json:"assigned_at_unix_ms,omitempty"`
	StartedAtUnixMS      *int64  `json:"started_at_unix_ms,omitempty"`
	LastHeartbeatUnixMS  *int64  `json:"last_heartbeat_unix_ms,omitempty"`
	LastDetail           string  `json:"last_detail,omitempty"`
	LastCheckpointUnixMS *int64  `json:"last_checkpoint_unix_ms,omitempty"`
	CheckpointSummary    string  `json:"checkpoint_summary,omitempty"`
	CompletedAtUnixMS    *int64  `json:"completed_at_unix_ms,omitempty"`
	StaleSinceUnixMS     *int64  `json:"stale_since_unix_ms,omitempty"`
	HeartbeatCount       *uint64 `json:"heartbeat_count,omitempty"`
	CheckpointCount      *uint64 `json:"checkpoint_count,omitempty"`
	NoArtifactRequeues   *uint32 `json:"no_artifact_requeues,omitempty"`
	DeadAssigneeReclaims *uint32 `json:"dead_assignee_reclaims,omitempty"`
}

// VersionedPlan is the canonical, version-counter-protected snapshot
// of the plan. Persisted to ~/.local/share/awp/.swarm-plans/<swarm_id>.jsonl.
// Each mutation (SeedGraph / ExpandNode / CompleteNode / InjectGap)
// increments Version. Clients should rely on Version for race-free
// updates (a stale snapshot at Version=N is rejected server-side
// with a fresh Version=N+K returned).
type VersionedPlan struct {
	Items        []PlanItem              `json:"items"`
	Version      uint64                  `json:"version"`
	Participants map[string]struct{}     `json:"participants"` // session_ids
	TaskProgress map[string]TaskProgress `json:"task_progress,omitempty"`
	Mode         PlanMode                `json:"mode"`
	NodeMeta     map[string]NodeMeta     `json:"node_meta,omitempty"` // item_id -> meta (deep mode)
}
