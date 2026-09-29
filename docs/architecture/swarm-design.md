# awp Swarm Layer Design

Status: draft (2026-09-29)
Author: design pass
Scope: full multi-session agent coordination over the existing JSON-RPC 2.0 UDS
transport, mirroring the surface of `~/Project/jcode`'s swarm core.

---

## 0. Principles

1. **No backward compat** (AGENTS.md golden rule #1). The protocol layer is
   sealed and bumpable in lockstep. Old method/event constants stay or get
   deleted in the same commit. `internal/agent-protocol/swarm/` is the only
   candidate for `// stable:` markers because it crosses the UDS boundary.
2. **Sealed types via unexported constructors** (golden rule #2). Every enum
   (`Role`, `LifecycleStatus`, `PlanMode`, `NodeKind`, `NodeStatus`,
   `DeliveryMode`, `TaskControlAction`, `ConfidenceLevel`) is a struct with a
   private string field. Caller-facing values are package-level vars; parsed
   values go through a constructor function that accepts the `Other` fallback.
   Type switches over the same struct stay exhaustive.
3. **Context first** (golden rule #3). `func(ctx context.Context, ...)`. No
   nil ctx. `http.NewRequestWithContext` everywhere. `defer cancel()` on
   every `WithCancel` / `WithTimeout`.
4. **No goroutine leaks** (golden rule #5). Every background watcher
   (await, heartbeat, idle reap, persist batcher) keys its exit on
   `ctx.Done()` and a `sync.WaitGroup` so `Server.Shutdown` can drain.
6. **Tests external** (golden rule #6). Three test trees:
   `test/agent-protocol/swarm/`, `test/agent-server/swarm/`,
   `test/agent-server/swarm_e2e/`. White-box through exported helpers only.

---

## 1. Surface (29 request methods, complete mirror of jcode)

Source of truth for the method list: `~/Project/jcode`'s `Request::Comm*`
enum in `crates/jcode-protocol/src/lib.rs`. Awp uses the same names minus
the `Comm` prefix on the wire (e.g. `comm_spawn` -> `method: "comm.spawn"`
per the existing `awp` UDS wire convention — see §6.4).

| # | jcode request | awp method constant | Phase | Internal handler |
|---|---|---|---|---|
| 1 | CommShare | MethodCommShare | S4 | swarm.share |
| 2 | CommRead | MethodCommRead | S4 | swarm.share |
| 3 | CommMessage | MethodCommMessage | S2 | swarm.message |
| 4 | CommList | MethodCommList | S2 | swarm.register |
| 5 | CommListChannels | MethodCommListChannels | S2 | swarm.channel |
| 6 | CommChannelMembers | MethodCommChannelMembers | S2 | swarm.channel |
| 7 | CommProposePlan | MethodCommProposePlan | S7 | swarm.proposal |
| 8 | CommApprovePlan | MethodCommApprovePlan | S7 | swarm.proposal |
| 9 | CommRejectPlan | MethodCommRejectPlan | S7 | swarm.proposal |
| 10 | CommSeedGraph | MethodCommSeedGraph | S5 | swarm.dag |
| 11 | CommExpandNode | MethodCommExpandNode | S5 | swarm.dag |
| 12 | CommCompleteNode | MethodCommCompleteNode | S5 | swarm.dag |
| 13 | CommInjectGap | MethodCommInjectGap | S5 | swarm.dag |
| 14 | CommSpawn | MethodCommSpawn | S2 | swarm.register |
| 15 | CommListModels | MethodCommListModels | S2 | swarm.register |
| 16 | CommStop | MethodCommStop | S2 | swarm.register |
| 17 | CommAssignRole | MethodCommAssignRole | S2 | swarm.register |
| 18 | CommSummary | MethodCommSummary | S3 | swarm.report |
| 19 | CommStatus | MethodCommStatus | S3 | swarm.report |
| 20 | CommReport | MethodCommReport | S3 | swarm.report |
| 21 | CommReadContext | MethodCommReadContext | S3 | swarm.message |
| 22 | CommResyncPlan | MethodCommResyncPlan | S5 | swarm.plan_store |
| 23 | CommPlanStatus | MethodCommPlanStatus | S5 | swarm.plan_store |
| 24 | CommAssignTask | MethodCommAssignTask | S5 | swarm.assign |
| 25 | CommAssignNext | MethodCommAssignNext | S5 | swarm.assign |
| 26 | CommTaskControl | MethodCommTaskControl | S5 | swarm.assign |
| 27 | CommSubscribeChannel | MethodCommSubscribeChannel | S2 | swarm.channel |
| 28 | CommUnsubscribeChannel | MethodCommUnsubscribeChannel | S2 | swarm.channel |
| 29 | CommAwaitMembers | MethodCommAwaitMembers | S3 | swarm.await |

Not in scope (jcode legacy / out-of-band):
`agent_register`, `agent_task`, `agent_capabilities`, `agent_dismiss`,
`agent_set_session_saved`, `rename_session`, `set_default_model`,
`set_default_effort`, `set_session_saved`, `extension_set`,
`tool_follow_up_resolve`, `tool_follow_up_status`. These are admin /
configuration flows, not multi-session coordination. They live in
`internal/agent-protocol/admin/` if re-implemented.

---

## 2. ServerEvent additions

| jcode event | awp event constant | Phase |
|---|---|---|
| `swarm_status` | EventSwarmStatus | S2 |
| `swarm_member_update` | EventSwarmMemberUpdate | S2 |
| `swarm_channel_message` | EventSwarmChannelMessage | S2 |
| `comm_spawn_response` | EventCommSpawnResponse | S2 |
| `comm_message_response` | EventCommMessageResponse | S2 |
| `comm_list_response` | EventCommListResponse | S2 |
| `comm_list_channels_response` | EventCommListChannelsResponse | S2 |
| `comm_channel_members_response` | EventCommChannelMembersResponse | S2 |
| `comm_list_models_response` | EventCommListModelsResponse | S2 |
| `comm_summary_response` | EventCommSummaryResponse | S3 |
| `comm_status_response` | EventCommStatusResponse | S3 |
| `comm_report_response` | EventCommReportResponse | S3 |
| `comm_read_context_response` | EventCommReadContextResponse | S3 |
| `comm_await_members_response` | EventCommAwaitMembersResponse | S3 |
| `swarm_plan` | EventSwarmPlan | S5 |
| `swarm_plan_proposal` | EventSwarmPlanProposal | S7 |
| `swarm_plan_proposal_response` | EventSwarmPlanProposalResponse | S7 |
| `comm_resync_plan_response` | EventCommResyncPlanResponse | S5 |
| `comm_plan_status_response` | EventCommPlanStatusResponse | S5 |
| `comm_assign_task_response` | EventCommAssignTaskResponse | S5 |
| `comm_assign_next_response` | EventCommAssignNextResponse | S5 |
| `comm_task_control_response` | EventCommTaskControlResponse | S5 |
| `comm_share_response` | EventCommShareResponse | S4 |
| `comm_read_response` | EventCommReadResponse | S4 |
| `comm_error` | EventCommError | S2 |

Response shape per event:

```go
type CommResponse[Out any] struct {
    ID      uint64          `json:"id"`
    Method  string          `json:"method"`
    Success bool            `json:"success"`
    Out     *Out            `json:"out,omitempty"`
    Error   *CommError      `json:"error,omitempty"`
}

type CommError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

Failure semantics:
- 4xx-style codes (caller-side error) -> `success: false`, response sent
  immediately, no state mutation.
- 5xx-style codes (server-side fault) -> `success: false`, partial mutation
  possible; client may retry with the same `request_nonce` for idempotency
  (comm_spawn / comm_complete_node are the only idempotent ones in MVP).
- 6xx-style codes (session/agent gone) -> `success: false`, treat as
  terminal for the request; client should re-resolve the member id.

---

## 3. Type package layout

```
internal/agent-protocol/swarm/
  doc.go                                # package godoc, stable-marker rationale
  lifecycle.go                          # LifecycleStatus + helpers (14 values)
  role.go                               # Role (3 values)
  member.go                             # MemberStatus, MemberRuntime, TodoItem,
                                        # ToolIntent, ToolProgress
  record.go                             # MemberRecord, ChannelSubscriptionRecord
                                        # (persistence-only)
  channel.go                            # ChannelInfo, ChannelMessage,
                                        # ChannelMessageEvent
  context.go                            # ContextEntry, ShareParams, ReadParams
  plan.go                               # PlanItem, PlanGraphStatus,
                                        # PlanGraphSummary, PlanMode,
                                        # TaskControlAction, VersionedPlan,
                                        # TaskProgress
  dag.go                                # TaskNode, TaskGraphNodeSpec,
                                        # NodeKind, NodeStatus, NodeOrigin,
                                        # Mode, HandoffArtifact,
                                        # ConfidenceLevel, NodeMeta
  await.go                              # AwaitedMemberStatus, AwaitOptions,
                                        # DeliveryMode
  helpers.go                            # 8 algorithmic helpers (priority
                                        # rank, cycle detect, ready-set,
                                        # TLDR validation, task label
                                        # derivation, terminal-status set,
                                        # action-allows-status, summarize)
```

Sealed-type pattern (lifecycle.go is canonical):

```go
package swarm

type LifecycleStatus struct{ s string }

var (
    StatusSpawned      = LifecycleStatus{"spawned"}
    StatusReady        = LifecycleStatus{"ready"}
    StatusRunning      = LifecycleStatus{"running"}
    StatusRunningStale = LifecycleStatus{"running_stale"}
    StatusCompleted    = LifecycleStatus{"completed"}
    StatusDone         = LifecycleStatus{"done"}
    StatusFailed       = LifecycleStatus{"failed"}
    StatusStopped      = LifecycleStatus{"stopped"}
    StatusCrashed      = LifecycleStatus{"crashed"}
    StatusQueued       = LifecycleStatus{"queued"}
    StatusBlocked      = LifecycleStatus{"blocked"}
    StatusPending      = LifecycleStatus{"pending"}
    StatusTodo         = LifecycleStatus{"todo"}
    StatusOther        = LifecycleStatus{""}
)

func ParseLifecycleStatus(s string) LifecycleStatus {
    switch s {
    case "spawned": return StatusSpawned
    // ... 13 more arms ...
    default:        return StatusOther
    }
}

func (l LifecycleStatus) String() string { return l.s }

func (l LifecycleStatus) Valid() bool { return l.s != "" || true /* Other valid */ }

func (l LifecycleStatus) IsTerminal() bool {
    switch l {
    case StatusCompleted, StatusDone, StatusFailed, StatusStopped, StatusCrashed:
        return true
    }
    return false
}

func (l LifecycleStatus) IsCompleted() bool {
    return l == StatusCompleted || l == StatusDone
}

func (l LifecycleStatus) IsFailed() bool {
    return l.IsTerminal() && !l.IsCompleted()
}

func (l LifecycleStatus) IsActive() bool {
    return l == StatusRunning || l == StatusRunningStale
}

func (l LifecycleStatus) IsRunnable() bool {
    return l == StatusQueued || l == StatusReady || l == StatusPending || l == StatusTodo
}
```

Type-switch exhaustiveness: every internal call site uses `switch status {
case swarm.StatusRunning: ... }` not `if status == "running"`. The sealed
struct makes the type-switch enforcement automatic. `StatusOther` is a
non-matched value that callers may check explicitly (typically for
forward-compat).

Constants (single source of truth):

```go
const (
    MaxTLDRChars                  = 200
    TLDRRequiredOverChars         = 240
    MaxCompletionReportChars      = 4000
    MaxPlanItems                  = 1024
    MaxSwarmMembers               = 1000
    MaxTaskLabelChars             = 48
    MaxSharedContextKeyBytes      = 4 * 1024
    MaxSharedContextKeysPerSwarm = 100
    MaxChannelNameBytes           = 64
    MaxChannelMessageBodyBytes    = 32 * 1024
    SpawnAdmissionPerSwarm        = 16
    HeartbeatIntervalSeconds      = 30
    StaleAfterSeconds             = 300
    IdleReapAfterSeconds          = 1800
    AwaitTickIntervalMs           = 500
    AwaitDefaultTimeoutSeconds    = 3600
    PersistBatchIntervalMs        = 5000
)
```

---

## 4. Server package layout

```
internal/agent-server/swarm/
  doc.go
  state.go                 # SwarmState (root), Member, Awaiter
  channel.go               # ChannelIndex
  register.go              # Register / Unregister / Spawn / Stop / AssignRole
  message.go                # SendMessage / ReadContext
  report.go                # Report / FormatCompletionReport
  share.go                 # Share / Read context
  await.go                 # AwaitMembers (background watcher)
  plan_store.go            # PlanStore (VersionedPlan mutations + persistence)
  dag.go                   # SeedGraph / ExpandNode / CompleteNode / InjectGap
  proposal.go              # ProposePlan / ApprovePlan / RejectPlan
  assign.go                # AssignTask / AssignNext / TaskControl
  broadcast.go             # Broadcaster interface + default impl
  persist.go               # Persister (append-only JSONL + snapshot)
  restore.go               # Restore struct + LoadAll on startup
  handle.go                # Method -> handler dispatch table
  runtime.go               # RuntimeOpts (admission, heartbeat, reap, tick)
  errors.go                # ErrNoPlan, ErrUnknownNode, ErrNotOwner,
                           # ErrCycleDetected, ErrUnknownDependency, ...
```

---

## 5. State machine

### 5.1 Member lifecycle

```
                +-----------+
   spawn ---->  |  spawned  |  --immediate assignment-->
                +-----------+                          v
                                                   +-------+
   +--------+    report("ready")    +<-------------| ready |
   | queued | <---------------------|               +-------+
   +--------+    (when coordinator  |                  |
        ^         assigns task)     |                  | report("running")
        |                            v                  |
        |                       +----------+ <-----------+
        |                       | running  |
        |                       +----------+
        |                           |
        |       +-------------------+-------------------+
        |       |                   |                   |
        |       v                   v                   v
        |  +-----------+    | running_stale |    +----------+
        |  | completed |    | (5min no hb) |    |  failed  |
        |  +-----------+    +---------------+    +----------+
        |                                            |
        |              +-----------+                  |
        |              |  crashed  | <----- kill -9 / panic
        |              +-----------+
        |
        | (no transition; queued is one-shot entry, not a
        |  re-enterable sink)
        v
   +---------+    stop (force)    +---------+
   |  done   | <------------------ | stopped |
   +---------+                    +---------+
```

- `queued` is the only entry state for tasks not yet assigned.
- `ready` is the only entry state for members not yet assigned to any task.
- Transitions are server-authoritative (clients report, server assigns).
- `running_stale` is auto-set by the heartbeat watcher (see §7.5).
- `stopped` and `crashed` are terminal; `crashed` is reached only when
  the agent goroutine panics or the underlying session jsonl path is gone.

### 5.2 DAG node lifecycle (per node)

```
    SeedGraph / ExpandNode / InjectGap
                  |
                  v
              +-------+
              | queued | --AssignTask--> running --CompleteNode--> done
              +-------+                                   \
                                                          \--  (errors)
                                                             |
                                                             v
                                                          +--------+
                                                          | failed |
                                                          +--------+

  In deep mode, ExpandNode auto-inserts a gate node (kind: verify or
  critique) whose blocked_by covers all children. The gate becomes
  queued after all children are done, and CompleteNode on the gate
  with artifact.confidence >= medium closes the parent composite.
```

### 5.3 Await lifecycle

```
    AwaitMembers(opts) ---+----> registered in state.awaiters map
                           |
                           v
                       background watcher goroutine
                           |
                           | every AwaitTickIntervalMs (500ms):
                           |   snapshot await target states
                           |   evaluate: all/any reached target_status
                           |
                           +--matched--> deliver EventCommAwaitMembersResponse
                           |              (and soft-interrupt if wake=true)
                           +--timeout---> deliver with success=false
                           +--cancel----> drop silently
```

---

## 6. Wire format

### 6.1 Method naming convention

Awp wire methods use a `comm.*` dotted prefix (existing convention from
`internal/agent-protocol/json_rpc/methods.go`):

```go
const (
    MethodCommShare             = "comm.share"
    MethodCommRead              = "comm.read"
    MethodCommMessage           = "comm.message"
    MethodCommList              = "comm.list"
    MethodCommListChannels      = "comm.list_channels"
    MethodCommChannelMembers    = "comm.channel_members"
    MethodCommProposePlan       = "comm.propose_plan"
    MethodCommApprovePlan       = "comm.approve_plan"
    MethodCommRejectPlan        = "comm.reject_plan"
    MethodCommSeedGraph         = "comm.seed_graph"
    MethodCommExpandNode        = "comm.expand_node"
    MethodCommCompleteNode      = "comm.complete_node"
    MethodCommInjectGap         = "comm.inject_gap"
    MethodCommSpawn             = "comm.spawn"
    MethodCommListModels        = "comm.list_models"
    MethodCommStop              = "comm.stop"
    MethodCommAssignRole        = "comm.assign_role"
    MethodCommSummary           = "comm.summary"
    MethodCommStatus            = "comm.status"
    MethodCommReport            = "comm.report"
    MethodCommReadContext       = "comm.read_context"
    MethodCommResyncPlan        = "comm.resync_plan"
    MethodCommPlanStatus        = "comm.plan_status"
    MethodCommAssignTask        = "comm.assign_task"
    MethodCommAssignNext        = "comm.assign_next"
    MethodCommTaskControl       = "comm.task_control"
    MethodCommSubscribeChannel  = "comm.subscribe_channel"
    MethodCommUnsubscribeChannel = "comm.unsubscribe_channel"
    MethodCommAwaitMembers      = "comm.await_members"
)
```

### 6.2 Request envelope

Standard JSON-RPC 2.0 with `params` carrying the typed payload:

```json
{
    "jsonrpc": "2.0",
    "id": <uint64>,
    "method": "comm.spawn",
    "params": {
        "from_session": "<session_id>",
        "request_nonce": "<uuidv7>",
        "initial_message": "...",
        "model": "claude-opus-4-7",
        "effort": "high",
        "label": "coder-1",
        "spawn_mode": "headless",
        "working_dir": "/path/to/proj"
    }
}
```

`request_nonce` (uuidv7) is mandatory for `comm.spawn`,
`comm.complete_node`, `comm.assign_task`. Other methods may omit it.
Server keeps a 24h LRU cache keyed by `(from_session, nonce)` for
idempotency replay.

### 6.3 Response event envelope

Responses to most comm methods are **events** (not direct replies),
because the result may take arbitrarily long (DAG completion, await
timeout, slow model). The id field on the response matches the request:

```json
{
    "jsonrpc": "2.0",
    "event": "comm.spawn_response",
    "id": <matching request id>,
    "data": {
        "session_id": "<requester>",
        "new_session_id": "<uuidv7>",
        "request_nonce": "<echoed>"
    }
}
```

A small subset of methods (comm.list, comm.list_channels,
comm.channel_members, comm.status, comm.plan_status, comm.list_models,
comm.assign_next, comm.task_control, comm.resync_plan) send the data
inline in the JSON-RPC result. The dispatch table in handle.go flags
each method as `kind: event-response` vs `kind: result`.

### 6.4 Error envelope

```json
{
    "jsonrpc": "2.0",
    "event": "comm.error",
    "id": <uint64>,
    "data": {
        "session_id": "<requester>",
        "method": "comm.spawn",
        "code": "swarm.capacity_exceeded",
        "message": "spawn admission limit reached for swarm sw-xxx"
    }
}
```

Codes:
- `swarm.unknown_session`
- `swarm.unknown_channel`
- `swarm.already_subscribed`
- `swarm.not_subscribed`
- `swarm.capacity_exceeded` (per-swarm SpawnAdmissionPerSwarm)
- `swarm.not_owner` (comm.complete_node / comm.expand_node)
- `swarm.unknown_node`
- `swarm.cycle_detected`
- `swarm.unknown_dependency`
- `swarm.artifact_invalid` (deep mode)
- `swarm.duplicate_nonce` (idempotency hit, response replayed)
- `swarm.session_gone` (6xx-style terminal)
- `swarm.too_large` (TLDR / body / label / shared-context key)

---

## 7. Server integration

### 7.1 The structural change (not 5 new fields)

`agentserver.Server` today holds a single `*agentcore.Agent` field and
serves a single session over the UDS socket. Adding swarm requires:

1. **Per-sessionID dispatch**: `Server.sessions map[string]*sessionAgent`
   keyed by session id, with the existing `s.agent` becoming
   `s.sessions[primarySessionID]` for the legacy single-session path.
2. **Per-conn subscription sink**: each UDS connection registers a
   `chan<- ServerEvent` in `s.swarms` (when swarm is enabled); the server
   fans comm events to that channel. Multiple live connections per
   session are allowed (TUI + a debug socket, for example).
3. **ServerEvent fan-out**: a new method `Server.FanoutSwarmEvent(ev,
   filter)` iterates live connections and pushes ev to each sink with
   non-blocking select.
4. **Session lifecycle hooks**: when a UDS connection drops, the server
   may or may not deregister the session (depends on whether the session
   has other live connections). When the last connection for a session
   drops and the session is headless, the server starts an idle-reap
   timer.
5. **One Agent per spawn**: spawn creates a fresh `*agentcore.Agent`
   instance and registers it in `s.sessions[newID]`. Headless agents run
   on a dedicated goroutine; visible ones are bound to a TUI conn (same
   pattern as the primary session, but indexed by session id not "the
   primary").

Existing API preservation: `New(ag, socketPath)` keeps the same
signature and is preserved as a convenience for non-swarm use. The new
constructor `NewWithSwarm(ag, socketPath, opts SwarmOpts)` adds swarm
state when callers need it.

### 7.2 sessionAgent struct

```go
type sessionAgent struct {
    Agent       *agentcore.Agent
    SessionID   string
    SwarmID     string
    Role        swarm.Role
    IsHeadless  bool
    ConnEventCh chan ServerEvent
    ParentID    string
    CreatedAt   time.Time
}
```

### 7.3 SwarmOpts

```go
type SwarmOpts struct {
    RuntimeOpts swarm.RuntimeOpts
    SessionsDir string              // ~/.local/share/awp/sessions/
    EnableBackgroundAwait bool     // default true
    SpawnParentHandoff bool         // default true; parent session gets
                                    // EventCommSpawnResponse on success
}
```

### 7.4 Dispatch fallback

Existing dispatch:

```go
func (s *Server) handle(req JSONRPCRequest) JSONRPCResponse { /* ... */ }
```

New dispatch adds the swarm fallback before the "unknown method" arm:

```go
default:
    if s.swarm != nil && swarm.IsCommMethod(req.Method) {
        s.swarm.Handle(conn, connCtx, req)
        return
    }
    // unknown method 4xx
```

`swarm.Handle` looks up the method in the dispatch table, validates the
session, and either sends the inline result or registers a background
operation that emits the response event later.

### 7.5 Background watchers

Four long-running goroutines per server (not per swarm):

1. **Heartbeat watcher**: every `HeartbeatIntervalSeconds` (30s), scan
   members in `running`. For each member where
   `now - lastHeartbeat > StaleAfterSeconds`, set `StatusRunningStale`.
2. **Idle reap watcher**: every 60s, scan headless members in
   `ready` / `running_stale` where `now - lastActivity >
   IdleReapSeconds`. For each, fire `Server.Stop(member.SessionID,
   force=true)`.
3. **Await tick**: per Awaiter, every 500ms evaluate target states. See §5.3.
4. **Persist batcher**: every `PersistBatchIntervalMs` (5s), flush the
   pending append queue to disk. On crash, last 5s of mutations may be
   lost; that's the documented SLA. Crash replay restores state from
   disk + the optional `.swarm-plans/<swarm_id>.jsonl` snapshot file.

All four goroutines take `ctx.Done()` and a `sync.WaitGroup` so
`Server.Shutdown` drains them.

---

## 8. agent-core integration

`internal/agent-core/agent.go` gains one optional field:

```go
type Agent struct {
    // ... existing fields ...
    swarmIdentity *swarm.MemberRecord
}
```

`WithSwarmIdentity` is called by `agentserver.swarm.register.Spawn`
after creating the agent, never by the user-facing Agent constructor.

Hooks live on the strategy, not on Agent:

```go
type SwarmHook interface {
    OnTurnBoundary(ev Event)
}

func (r *ReActStrategy) WithSwarmHook(h SwarmHook) { r.swarmHook = h }
```

Default no-op preserves existing tests. Swarm-aware agents override
to call `ReportStatus` on turn boundaries, subscribe to channels,
or surface completion reports.

---

## 9. Persistence model

### 9.1 Append-only JSONL (extension to existing Store)

`internal/agent-server.Store` currently writes per-session `.jsonl`.
Swarm extends with new entry kinds:

| Entry kind | Shape | Trigger |
|---|---|---|
| `swarm.member_update` | `MemberRecord` | Every UpdateStatus / SetLatestReport |
| `swarm.channel_sub` | `ChannelSubscriptionRecord` | SubscribeChannel |
| `swarm.channel_unsub` | `ChannelSubscriptionRecord` | UnsubscribeChannel |
| `swarm.completion_report` | `{session_id, body, validation, follow_up, tldr, ts}` | Report |
| `swarm.shared_context` | `{swarm_id, key, entry}` | Share |

Loaded on Server.Start; replayed into `state.members` /
`state.channels` / `state.sharedCtx`.

### 9.2 Plan snapshot

Plan files are separate from session jsonl because they cross multiple
sessions. Path: `~/.local/share/awp/.swarm-plans/<swarm_id>.jsonl`.

Each entry is a `VersionedPlan` snapshot (full items array + version
+ participants + node meta). On startup, the latest snapshot per
swarm_id is loaded; subsequent entries are append-only.

### 9.3 Restore order on startup

1. Iterate `~/.local/share/awp/sessions/*.jsonl`, dispatch by entry kind
   into the in-memory state.
2. For each active session id (those with `swarm.member_update` entries),
   reattach the live UDS connection count from the last known value
   (default 0 — they'll re-attach on next connect).
3. Load `~/.local/share/awp/.swarm-plans/<swarm_id>.jsonl` for each
   swarm_id seen in step 1.
4. Resume background watchers (heartbeat / idle reap / persist batcher /
   per-awaiter ticks). Awaiters that timed out during the downtime are
   delivered immediately with `success: false` and a `crashed` server
   reason.

### 9.4 Idempotency replay

`comm.spawn` / `comm.complete_node` / `comm.assign_task` carry a
`request_nonce` (uuidv7). Server keeps a per-process LRU keyed by
`(from_session, nonce)` with 24h TTL. Hit -> replay the cached response
verbatim. Cold miss -> normal path. The LRU is intentionally in-memory,
not persisted: a crash invalidates the cache, and a retry after crash
is treated as a fresh request.

---

## 10. SDK (`internal/agent-client/swarm/`)

Go SDK mirroring the wire surface 1:1, plus stream subscriptions:

```go
type Client struct {
    rpc  *Client               // existing transport
    swID string                  // set by Register, returned by server
}

func (c *Client) Spawn(ctx, req) (SpawnResponse, error)
func (c *Client) Stop(ctx, target string, force bool) error
func (c *Client) List(ctx) ([]MemberStatus, error)
func (c *Client) ListChannels(ctx) ([]ChannelInfo, error)
func (c *Client) ChannelMembers(ctx, channel string) ([]string, error)
func (c *Client) Subscribe(ctx, channel string) error
func (c *Client) Unsubscribe(ctx, channel string) error
func (c *Client) Message(ctx, opts MessageOptions) error
func (c *Client) Report(ctx, opts ReportOptions) error
func (c *Client) ReadContext(ctx, target string) ([]json_rpc.Message, error)
func (c *Client) AwaitMembers(ctx, opts AwaitOptions) (AwaitResult, error)
func (c *Client) ProposePlan(ctx, items []PlanItem) error
func (c *Client) ApprovePlan(ctx, proposer string) error
func (c *Client) RejectPlan(ctx, proposer string, reason string) error
func (c *Client) SeedGraph(ctx, mode PlanMode, nodes []TaskGraphNodeSpec) (VersionedPlan, error)
func (c *Client) ExpandNode(ctx, parentID string, children []TaskGraphNodeSpec) error
func (c *Client) CompleteNode(ctx, nodeID string, artifact HandoffArtifact) error
func (c *Client) InjectGap(ctx, gateID string, nodes []TaskGraphNodeSpec) error
func (c *Client) PlanStatus(ctx) (PlanGraphStatus, error)
func (c *Client) ResyncPlan(ctx) (VersionedPlan, error)
func (c *Client) AssignTask(ctx, taskID, target string, message string) error
func (c *Client) AssignNext(ctx, target string, opts AssignNextOptions) (string, error)
func (c *Client) TaskControl(ctx, action TaskControlAction, taskID, target string) (PlanGraphStatus, error)
func (c *Client) Status(ctx, target string) (MemberStatus, error)
func (c *Client) Summary(ctx, target string, limit int) ([]ToolCallSummary, error)
func (c *Client) AssignRole(ctx, target string, role Role) error
func (c *Client) Share(ctx, key, value string, append bool) error
func (c *Client) Read(ctx, key string) ([]ContextEntry, error)
func (c *Client) ListModels(ctx) (ListModelsResponse, error)
```

Stream subscriptions (typed channels, not raw ServerEvent):

```go
func (c *Client) SwarmStatus(ctx) (<-chan []MemberStatus, error)
func (c *Client) ChannelMessages(ctx) (<-chan ChannelMessageEvent, error)
func (c *Client) PlanUpdates(ctx) (<-chan SwarmPlanEvent, error)
```

Subscriptions multiplex over the same underlying UDS connection. Each
fires a `comm.subscribe_channel` request under the hood and demuxes
incoming `swarm_*` events by event name.

---

## 11. CLI (`cmd/awp/swarm.go`)

```text
awp swarm spawn   --label <8char> --message <body> [--model inherit|...]
                   [--effort low|medium|high|xhigh|max]
                   [--spawn-mode visible|headless]
                   [--working-dir <path>]
awp swarm stop    --target <session_id> [--force]
awp swarm list
awp swarm status  --target <session_id>
awp swarm message --to-session <id> | --channel <name>
                   --body <body> [--tldr <tldr>] [--delivery notify|interrupt|wake]
awp swarm await   --target-status <csv> [--session-ids <csv>]
                   [--mode all|any] [--timeout <secs>]
                   [--sync]                                   # default background
awp swarm report  --target <session_id>
                   [--status ready|completed|failed|...]
                   --message <body> [--tldr <tldr>]
awp swarm share   --key <name> --value <body> [--append]
awp swarm read    --key <name>
awp swarm channels list
awp swarm channels subscribe   <name>
awp swarm channels unsubscribe <name>
awp swarm channels members     <name>
awp swarm plan seed    --mode light|deep --graph <file>
awp swarm plan expand  --parent <id> --children <file>
awp swarm plan complete --node <id> --artifact <file>
awp swarm plan gap      --gate <id> --nodes <file>
awp swarm plan status
awp swarm plan resync
awp swarm plan assign   --task <id> --target <session_id>
awp swarm plan next     --target <session_id> [--spawn]
awp swarm plan control  --action <start|wake|resume|retry|reassign|replace|salvage>
                         --task <id> --target <session_id>
awp swarm propose       --plan <file>             # sub-agent only
awp swarm approve       --proposer <session_id>   # coordinator only
awp swarm reject        --proposer <session_id> --reason <text>
awp swarm models
awp swarm assign-role   --target <session_id> --role agent|coordinator
```

CLI is a thin wrapper around the Go SDK. Each subcommand has a
`runE` that opens a UDS socket, calls the SDK method, formats output
(human-readable by default, `--json` flag for machine-readable).

---

## 12. TUI integration (S9 only, deferred)

`internal/tui/swarm/panel.go` exposes a `SwarmPanel` bubbletea
component using `bubbles/list`:

```go
type SwarmPanel struct {
    members []swarm.MemberStatus
    focused string            // session_id
    width, height int
    styles SwarmPanelStyles
    list   list.Model         // bubbles/list
}

func New() tea.Model
func (p *SwarmPanel) Update(msg tea.Msg) tea.Cmd
func (p *SwarmPanel) View() string
```

Toggle key: `ctrl+s` in `internal/tui/chat.go`. List shows member
friendly name, status icon, task label, and last activity age.
Selecting a member opens a side panel with the member's
`MemberRuntime` (model, effort, elapsed).

---

## 13. Testing

### 13.1 Protocol layer (`test/agent-protocol/swarm/`)

- `lifecycle_test.go`: 14 valid ParseLifecycleStatus round-trips + 12
  unknown-string fallback to StatusOther + IsTerminal/IsActive/IsRunnable
  classification truth table (5x14 = 70 cases).
- `role_test.go`: 3 valid + 1 fallback round-trip.
- `member_test.go`: MarshalJSON byte-equality with jcode fixture
  (extracted from a patched-jcode run with all fields populated).
- `plan_test.go`: PlanItem + PlanGraphStatus + VersionedPlan fixtures.
- `dag_test.go`: TaskGraphNodeSpec + HandoffArtifact.
- `helpers_test.go`: 8 helpers including `CycleItemIDs` on 6 fixture
  graphs (1 trivial / 1 self-cycle / 1 two-cycle / 1 three-cycle /
  1 diamond / 1 large linear).

### 13.2 Server state machine (`test/agent-server/swarm/`)

- `state_test.go`: Register / Unregister / UpdateStatus concurrent
  race (100 goroutines × 100 ops).
- `channel_test.go`: Subscribe / Unsubscribe bi-directional index
  consistency; RemoveSession full cleanup.
- `register_test.go`: Spawn admission lock with 17 concurrent spawns
  hitting a 16-cap swarm.
- `message_test.go`: comm_message to_session direct delivery;
  channel fan-out with 5 subscribers.
- `await_test.go`: AwaitMembers 100ms heartbeat / 50ms timeout /
  cancel via context / background-notify delivery.
- `dag_test.go`: SeedGraph cycle detection + ExpandNode deep mode
  auto-gate insertion + CompleteNode deep mode artifact validation.
- `proposal_test.go`: Propose / Approve / Reject state machine.
- `broadcast_test.go`: 5 connections all receiving the same
  EventSwarmStatus fan-out.
- `persist_test.go`: Append 100 entries + LoadAll equivalence.
- `handle_dispatch_test.go`: All 29 method constants resolve to a
  handler; unknown method -> 4xx; wrong session owner -> 4xx.

### 13.3 End-to-end (`test/agent-server/swarm_e2e/`)

| Scenario | Steps |
|---|---|
| spawn + report + await | (1) coordinator conn `Spawn` child session; (2) child mock agent reports `completed`; (3) coordinator conn `AwaitMembers(target=[completed])` -> matches |
| channel broadcast | (1) coordinator + 2 child conns `Subscribe("user-review")`; (2) coordinator `Message(channel="user-review")`; (3) all three conns receive `EventSwarmChannelMessage` |
| plan lifecycle (light) | (1) `SeedGraph([a,b,c], blocked_by)`; (2) `ExpandNode(a, [a1,a2])`; (3) `CompleteNode(a1)`; (4) `CompleteNode(a2)`; (5) `PlanStatus` -> counts verified |
| plan lifecycle (deep) | (1) `SeedGraph(mode=deep, [a])`; (2) `ExpandNode(a, [a1,a2])` -> auto-inserts gate g; (3) `CompleteNode(a1)` + `CompleteNode(a2)` -> g becomes ready; (4) `CompleteNode(g, artifact=high_confidence)` -> a becomes done |
| crash recovery | (1) start server + register A + subscribe channel; (2) `kill -9`; (3) restart; (4) `ListMembers` -> A present; `ListChannels` -> channel restored |
| await timeout | (1) `AwaitMembers(target=[completed], timeout=2s)`; (2) member never moves; (3) 2s -> response with `completed: false` |
| await background + wake | (1) `AwaitMembers(background=true, wake=true)`; (2) member completes -> server soft-interrupts requester -> next requester turn receives `EventCommAwaitMembersResponse` |
| idempotent spawn | (1) spawn twice with same `request_nonce`; (2) server returns the same `new_session_id` from cache; (3) only one Member actually created |
| proposal flow | (1) coordinator `Spawn` sub-agent; (2) sub-agent `ProposePlan`; (3) coordinator receives `EventSwarmPlanProposal`; (4) coordinator `ApprovePlan`; (5) `PlanStatus` shows the new plan |

All e2e tests run under `-race -count=1 -timeout=120s`.

---

## 14. Phasing (revised, honest estimates)

| Phase | Deliverable | Prod LOC | Test LOC | Depends on |
|---|---|---|---|---|
| S1 | Protocol types + helpers + method/event/response constants | 1300 | 600 | — |
| S2 | Spawn / stop / list / message / channel runtime + server integration | 3000 | 1400 | S1 |
| S3 | comm_summary/status/report/read_context + comm_await_members | 1500 | 1000 | S2 |
| S4 | comm_share/read + SharedContext persistence | 800 | 400 | S3 |
| S5 | DAG light mode (SeedGraph/ExpandNode/CompleteNode/InjectGap + Assign*) | 3000 | 1800 | S4 |
| S6 | DAG deep mode (HandoffArtifact + auto-gate + confidence validation) | 2200 | 1400 | S5 |
| S7 | comm_propose_plan/approve/reject + EventSwarmPlanProposal* | 1400 | 600 | S5 |
| S8 | CLI (`cmd/awp swarm`) + SDK (`internal/agent-client/swarm`) + agent-core integration | 2000 | 1000 | S3 |
| S9 | TUI swarm gallery panel + TUI toggle integration | 1800 | 1500 | S7 |

**MVP (S1 + S2 + S3 + S8)** = ~7800 prod + ~4000 test = ~11800 LOC.

**Full swarm (S1..S9)** = ~17000 prod + ~9700 test = ~26700 LOC.

Honest comparison: jcode's swarm layer is ~16000 prod + ~6000 test =
~22000 LOC. awp's full swarm will land slightly **larger** than
jcode's, not the same magnitude — Go's simpler async model saves
~1000 LOC versus tokio, but fresh code without 9 years of accumulated
edge cases adds ~3000 LOC in the form of explicit branches Go would
not tolerate.

---

## 15. Decision record

| ID | Decision | Choice | Rationale |
|---|---|---|---|
| D1 | Process model | **single socket, multi-session Agent in process** | Avoids IPC socket fanout; spawn = server-internal `*agentcore.Agent`. Trade-off: one OOM kills the whole server. Mitigation: `Server.Shutdown` + heartbeat-based crash detection. |
| D2 | Persistence scope | **full replay on startup** | Mirrors jcode. Slight startup cost (~1ms per 1000 entries) is acceptable. |
| D3 | Await blocking | **default `background=true, wake=true`** | Matches jcode; preserves requester turn throughput. Sync mode available via `--sync`. |
| D4 | Delivery modes | **3 modes (notify / interrupt / wake)** | Mirrors jcode's `CommDeliveryMode` enum. Interrupt is rare; wake is the default. |
| D5 | DAG phasing | **separate sprint, NOT in MVP** | S5+S6+S7 is ~6600 prod LOC — meaningful enough to ship as its own sprint with its own scout + audit. |
| D6 | SDK in MVP | **yes (S8 dependency)** | E2E tests in `test/agent-server/swarm_e2e/` cannot be cleanly written without the SDK. |
| D7 | CLI in MVP | **yes** | Dogfooding requires CLI. |
| D8 | TUI in MVP | **no (deferred to S9)** | S2-S8 is reachable entirely through CLI + SDK; TUI is a UX improvement, not a feature gate. |
| D9 | `// stable:` markers | **yes on `internal/agent-protocol/swarm/`** | Crosses the UDS boundary; clients depend on field names + method names. Server-side internals are not stable. |
| D10 | Spawn admission | **16 per swarm** | Mirrors jcode. Configurable via RuntimeOpts. |
| D11 | Channel ACL | **none in MVP** | Phase 2 may add `ChannelInfo.AclMembers`. |
| D12 | Shared Context quota | **4 KB per key, 100 keys per swarm** | Hard cap; further requests return `swarm.too_large`. |

D1, D5, D9 are the architecturally consequential ones; the rest mirror
jcode defaults and can be amended without restructuring.

---

## 16. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Server crash kills every spawned session | high | Document 5s persist SLA; offer `--worker-durable` flag in S9 for disk-backed journal; for MVP accept in-process only. |
| Spawn admission starvation under burst | medium | Fair queue per-swarm; rejection returns `swarm.capacity_exceeded`; caller can retry with backoff |
| Await ghost members after crash | high | Awaiters re-delivered with `success: false` + `crashed` reason on restart; idempotent retry safe |
| DAG cycle in user input | high | Cycle detection in SeedGraph; cycle-aware `CycleItemIDs` helper for diagnostics |
| Deep mode artifact spam (gates always pass with "high confidence") | medium | Server validates `findings` non-empty + minimum evidence count; future S7.5 may add cross-gate review |
| TUI fan-out lag (events back up) | medium | Drop oldest per-conn event with a single `comm.dropped` event flag; never block fan-out on slow conn |
| LLM cost attribution per member | medium (future) | Defer to S10; not in scope MVP |
| Model auth-route prefix confusion (`openai-api:gpt-5.5` vs `gpt-5.5`) | low | `model_override` carries the full string verbatim; `list_models` returns both forms |

---

## 17. Out of scope (deferred)

- Cross-server swarms (multiple awp instances coordinating)
- Long-running durable workers (S9+ ships a `--worker-durable` flag but
  no journal implementation)
- Token usage / cost attribution per member
- Channel ACL
- Per-route credential rotation
- TUI panel (S9)
- LLM-side rate limiting per swarm
- Distributed consensus on DAG completion (single-server only)

---

## 18. References

- jcode source: `~/Project/jcode/crates/`
  - `jcode-app-core/src/server/comm_*.rs`
  - `jcode-app-core/src/server/swarm_mutation_state.rs`
  - `jcode-app-core/src/server/state.rs`
  - `jcode-app-core/src/server/headless.rs`
  - `jcode-protocol/src/lib.rs` (wire types)
  - `jcode-swarm-core/src/lib.rs`
  - `jcode-plan/src/lib.rs`
  - `jcode-plan/src/dag/{mod,ops,schedule,sim,tests}.rs`
- awp source:
  - `internal/agent-protocol/` (existing wire types)
  - `internal/agent-server/` (existing JSON-RPC dispatch)
  - `internal/agent-client/` (existing Go SDK)
  - `internal/agent-core/agent.go` (existing Agent struct)