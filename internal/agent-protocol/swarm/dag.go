package swarm

import "encoding/json"

// Mode is a sealed enum that disambiguates between light and deep
// DAG orchestration at the node granularity (PlanMode is at the plan
// level; Mode is at the node level so per-node metadata can refer to
// the originating mode).
//
// Equivalent to PlanMode but kept distinct because:
//   - Mode is the canonical wire shape for TaskNode and NodeMeta
//   - PlanMode is the canonical wire shape for PlanGraphStatus
//
// The two are guaranteed to round-trip through string equality.
type Mode struct{ s string }

var (
	ModeDeep  = Mode{"deep"}
	ModeLight = Mode{"light"}
	ModeOther = Mode{""}
)

func ParseMode(s string) Mode {
	switch s {
	case "deep":
		return ModeDeep
	case "light":
		return ModeLight
	default:
		return ModeOther
	}
}

func (m Mode) String() string      { return m.s }
func (m Mode) IsDeep() bool        { return m == ModeDeep }
func (m Mode) IsLight() bool       { return m == ModeLight }
func (m Mode) IsOther() bool       { return m == ModeOther }
func (m Mode) RequiresGates() bool { return m == ModeDeep }

func (m Mode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.s)
}

func (m *Mode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*m = ParseMode(s)
	return nil
}

// NodeKind is a sealed enum describing what kind of work a node
// represents. Maps to jcode's TaskNodeKind (5 variants).
type NodeKind struct{ s string }

var (
	NodeExplore    = NodeKind{"explore"}
	NodeImplement  = NodeKind{"implement"}
	NodeVerify     = NodeKind{"verify"}
	NodeFix        = NodeKind{"fix"}
	NodeSynthesize = NodeKind{"synthesize"}
	NodeCritique   = NodeKind{"critique"}
	NodeOther      = NodeKind{""}
)

func ParseNodeKind(s string) NodeKind {
	switch s {
	case "explore":
		return NodeExplore
	case "implement":
		return NodeImplement
	case "verify":
		return NodeVerify
	case "fix":
		return NodeFix
	case "synthesize":
		return NodeSynthesize
	case "critique":
		return NodeCritique
	default:
		return NodeOther
	}
}

func (k NodeKind) String() string { return k.s }
func (k NodeKind) IsOther() bool  { return k.s == "" }

// IsGateKind reports whether this kind is one that gates downstream
// progress (verify = artifact-validating, critique = plan-validating).
func (k NodeKind) IsGateKind() bool {
	return k == NodeVerify || k == NodeCritique
}

// GateKind returns the gate kind that should follow a node of this
// kind in deep mode. Implement -> verify; explore/synthesize ->
// critique; gate kinds map to themselves (already a gate); other ->
// Other (no auto-gate).
func (k NodeKind) GateKind() NodeKind {
	switch k {
	case NodeImplement, NodeFix:
		return NodeVerify
	case NodeExplore, NodeSynthesize:
		return NodeCritique
	}
	return k
}

func (k NodeKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.s)
}

func (k *NodeKind) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*k = ParseNodeKind(s)
	return nil
}

// NodeStatus is a sealed enum describing a node's lifecycle.
// Distinct from LifecycleStatus because DAG nodes have their own
// narrower set: queued / running / done / failed.
type NodeStatus struct{ s string }

var (
	NodeQueued      = NodeStatus{"queued"}
	NodeRunning     = NodeStatus{"running"}
	NodeDone        = NodeStatus{"done"}
	NodeFailed      = NodeStatus{"failed"}
	NodeStatusOther = NodeStatus{""}
)

func ParseNodeStatus(s string) NodeStatus {
	switch s {
	case "queued":
		return NodeQueued
	case "running":
		return NodeRunning
	case "done":
		return NodeDone
	case "failed":
		return NodeFailed
	default:
		return NodeStatusOther
	}
}

func (s NodeStatus) String() string   { return s.s }
func (s NodeStatus) IsOther() bool    { return s.s == "" }
func (s NodeStatus) IsTerminal() bool { return s == NodeDone || s == NodeFailed }

func (s NodeStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.s)
}

func (s *NodeStatus) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = ParseNodeStatus(v)
	return nil
}

// NodeOrigin is a sealed enum describing how a node came to exist
// in the plan (initial seed vs. expanded by an agent vs. injected
// as a gap after gate failure vs. synthesized as a gate).
type NodeOrigin struct{ s string }

var (
	OriginSeed   = NodeOrigin{"seed"}
	OriginExpand = NodeOrigin{"expand"}
	OriginGap    = NodeOrigin{"gap"}
	OriginGate   = NodeOrigin{"gate"}
	OriginOther  = NodeOrigin{""}
)

func ParseNodeOrigin(s string) NodeOrigin {
	switch s {
	case "seed":
		return OriginSeed
	case "expand":
		return OriginExpand
	case "gap":
		return OriginGap
	case "gate":
		return OriginGate
	default:
		return OriginOther
	}
}

func (o NodeOrigin) String() string { return o.s }
func (o NodeOrigin) IsOther() bool  { return o.s == "" }

func (o NodeOrigin) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.s)
}

func (o *NodeOrigin) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*o = ParseNodeOrigin(v)
	return nil
}

// ConfidenceLevel is a sealed enum describing the agent's
// confidence in a completed artifact (deep mode only).
//
// Wire format accepts lenient strings via ParseConfidenceLevel:
// "low" / "1" / "1/10" / "not confident" -> ConfidenceLow
// "medium" / "5" / "5/10" / "moderate"    -> ConfidenceMedium
// "high" / "8-10" / "9/10" / "very confident" -> ConfidenceHigh
type ConfidenceLevel struct{ s string }

var (
	ConfidenceLow    = ConfidenceLevel{"low"}
	ConfidenceMedium = ConfidenceLevel{"medium"}
	ConfidenceHigh   = ConfidenceLevel{"high"}
	ConfidenceOther  = ConfidenceLevel{""}
)

func ParseConfidenceLevel(s string) ConfidenceLevel {
	switch s {
	case "low", "1", "1/10", "2/10", "3/10", "not confident", "uncertain":
		return ConfidenceLow
	case "medium", "5", "4/10", "5/10", "6/10", "7/10", "moderate":
		return ConfidenceMedium
	case "high", "8", "9", "10", "8/10", "9/10", "10/10", "very confident":
		return ConfidenceHigh
	default:
		return ConfidenceOther
	}
}

func (c ConfidenceLevel) String() string { return c.s }
func (c ConfidenceLevel) IsOther() bool  { return c.s == "" }

func (c ConfidenceLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.s)
}

func (c *ConfidenceLevel) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*c = ParseConfidenceLevel(v)
	return nil
}

// TaskGraphNodeSpec is the input to SeedGraph and ExpandNode.
// Server-side enforces MaxPlanItems, dedupe on ID, and cycle
// detection (CommCycleDetected).
type TaskGraphNodeSpec struct {
	ID        string   `json:"id"`
	Content   string   `json:"content"`
	Kind      NodeKind `json:"kind"`
	Priority  string   `json:"priority,omitempty"` // "high" | "medium" | "low"
	Subsystem string   `json:"subsystem,omitempty"`
	FileScope []string `json:"file_scope,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// TaskNode is a single node in the in-memory DAG. Persisted as part
// of VersionedPlan via the seed/expand entries.
type TaskNode struct {
	ID           NodeID     `json:"id"`
	Content      string     `json:"content"`
	Kind         NodeKind   `json:"kind"`
	Status       NodeStatus `json:"status"`
	AssignedTo   string     `json:"assigned_to,omitempty"` // session_id
	Planner      string     `json:"planner,omitempty"`     // composite owner
	Origin       NodeOrigin `json:"origin,omitempty"`
	Parent       string     `json:"parent,omitempty"` // composite parent id
	Expanded     bool       `json:"expanded"`
	IsGate       bool       `json:"is_gate"`
	ArtifactJSON string     `json:"artifact_json,omitempty"`
	CreatedAtMS  int64      `json:"created_at_unix_ms"`
	UpdatedAtMS  int64      `json:"updated_at_unix_ms"`
}

// NodeID is the canonical id type for a DAG node.
type NodeID = string

// NodeMeta carries non-item metadata for a node (used by deep mode
// to retain Kind/Origin/IsGate across rehydrations and gate insertion).
type NodeMeta struct {
	Kind         NodeKind   `json:"kind,omitempty"`
	Parent       string     `json:"parent,omitempty"`
	Expanded     bool       `json:"expanded"`
	IsGate       bool       `json:"is_gate"`
	Planner      string     `json:"planner,omitempty"`
	ArtifactJSON string     `json:"artifact_json,omitempty"`
	Origin       NodeOrigin `json:"origin,omitempty"`
}

// HandoffArtifact is the structured payload that deep-mode nodes
// must attach to comm.complete_node. Server validates that findings
// is non-empty and (for code-affecting kinds) validation is non-empty.
type HandoffArtifact struct {
	Findings            string          `json:"findings"`
	Evidence            []string        `json:"evidence,omitempty"`
	EdgeCasesConsidered []string        `json:"edge_cases_considered,omitempty"`
	Validation          string          `json:"validation,omitempty"`
	OpenQuestions       []string        `json:"open_questions,omitempty"`
	Confidence          ConfidenceLevel `json:"confidence,omitempty"`
	WhatIDidNotCheck    []string        `json:"what_i_did_not_check,omitempty"`
}
