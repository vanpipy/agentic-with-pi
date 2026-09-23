# AWP Session JSONL Schema + Wire Format Alignment

**Goal contract:** `.pi/orchestrator/goal-GC-2026-001.yaml`
**DAG:** `.pi/orchestrator/dag-DAG-2026-001.yaml`
**Status:** Phase 1 design (committed by task P1)
**Authoritative for:** all code under `internal/agent-core/`, `internal/agent-server/`, `internal/agent-protocol/json_rpc/`, `internal/tui/` until further notice.

## 1. Executive summary

AWP's current session log uses a fragmented `kind:event` schema that splits one logical assistant turn into six entries (`thought_start` / `thought_chunk × N` / `thought_end` / `tool` / `observe` / `final_answer`), embeds the system prompt inside the session entry, and emits TUI-specific wire event names that don't round-trip. This makes logs hard to debug, hard to audit, and blocks future features (skill injection, todo tracking, subagent notifications) that need a clean extension point.

This goal aligns both the on-disk JSONL schema and the on-the-wire JSON-RPC events with pi-coding-agent's message-level format: one canonical `Message` shape carrying role + content array, parent-id chains for ordering, and `custom` / `custom_message` extension entries for future features. System prompt stays in the session entry for Phase 1 (lowest-risk migration); legacy JSONL remains readable via the existing `Load` function. Compaction writes are reserved but not implemented (verified empirically that no AWP session ever fired one).

## 2. Target JSONL schema

### 2.1 Entry kinds

| `kind`       | Phase 1 emits? | Replaces            | Notes                                                                 |
|--------------|----------------|---------------------|-----------------------------------------------------------------------|
| `session`    | ✅             | `session` v1        | Adds `cwd` field. `system`, `model`, `max_turns`, `tools` stay inline. |
| `message`    | ✅ (new)       | `event` categories  | One entry per logical message (user prompt, assistant turn, toolResult).|
| `custom`     | ✅ (new)       | `event/error`       | For `abort`, `tool_error`, future skill/todo hooks.                   |
| `custom_message` | reserved   | —                   | For future subagent notifications (Phase 2).                          |
| `compaction` | reserved       | `compaction` v1     | Hook exists; no trigger fires it. Documented for Phase 2.             |
| `event`      | deprecated on new writes | —       | Legacy reader still accepts old files.                                |

### 2.2 Schema versions

| `version` | Schema                            | Loader path                   |
|-----------|-----------------------------------|-------------------------------|
| `1`       | Current AWP fragmented format     | `Load` (legacy)               |
| `2`       | New message-level format          | `Load` (new)                  |

### 2.3 `session` entry (v2)

```json
{
  "kind": "session",
  "version": 2,
  "id": "f7173577969c88148cb5bf3c16767d87",
  "timestamp": "2026-09-23T13:00:00.000Z",
  "cwd": "/home/leroy/Project/agentic-with-pi",
  "model": "MiniMax-M3",
  "max_turns": 200,
  "system": "You are a coding assistant...",
  "tools": ["read", "write", "edit_match", "bash", "agentgrep", "glob", "ls", "invalid"]
}
```

Phase 1 keeps `model`, `max_turns`, `system`, `tools` inline (the AGENTS.md Layout currently says agent-server owns the JSONL store with these fields). Phase 2 may split `system` to a `role:system` message.

### 2.4 `message` entry (v2)

```json
{
  "kind": "message",
  "version": 2,
  "id": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7d",
  "parentId": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7c",
  "timestamp": "2026-09-23T13:00:01.234Z",
  "message": {
    "role": "assistant",
    "content": [
      {"type": "thinking", "thinking": "The user wants...", "thinkingSignature": "b1a6..."},
      {"type": "text", "text": "I'll search for that."},
      {"type": "toolCall", "id": "call_0190a3b7...", "name": "agentgrep", "intent": "find x", "arguments": {"pattern": "jcode"}}
    ],
    "stopReason": "toolUse",
    "usage": {
      "promptTokens": 27891,
      "completionTokens": 171,
      "totalTokens": 28062,
      "cacheReadTokens": 128,
      "cacheWriteTokens": 0
    }
  }
}
```

**Tool result message** (separate entry, `role:toolResult`):

```json
{
  "kind": "message",
  "version": 2,
  "id": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7e",
  "parentId": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7d",
  "timestamp": "2026-09-23T13:00:02.456Z",
  "message": {
    "role": "toolResult",
    "content": [{"type": "text", "text": "42 matches found..."}]
  },
  "details": {
    "toolName": "agentgrep",
    "intent": "find x",
    "isError": false
  }
}
```

**User prompt message** (`role:user`, separate entry, no `details`):

```json
{
  "kind": "message",
  "version": 2,
  "id": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7c",
  "parentId": null,
  "timestamp": "2026-09-23T13:00:00.789Z",
  "message": {
    "role": "user",
    "content": [{"type": "text", "text": "Help me remove jcode"}]
  }
}
```

### 2.5 `custom` entry (v2)

```json
{
  "kind": "custom",
  "version": 2,
  "id": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7f",
  "parentId": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7d",
  "timestamp": "2026-09-23T13:00:30.000Z",
  "customType": "abort",
  "data": {"reason": "tool failed 3 times"}
}
```

Reserved `customType` vocabulary (Phase 1): `tool_error`, `abort`. Phase 2 adds: `skill_invoked`, `todo_updated`, `subagent_notification`.

### 2.6 `compaction` entry (reserved, no data in Phase 1)

```json
{
  "kind": "compaction",
  "version": 2,
  "id": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c80",
  "parentId": "<last-kept-message-id>",
  "timestamp": "2026-09-23T13:00:30.000Z",
  "summary": "Prior conversation covered: ...",
  "tokensBefore": 12345,
  "tokensAfter": 1024,
  "firstKeptEntryId": "0190a3b7-4d5e-7c8a-9b0d-2e3f4a5b6c7c"
}
```

### 2.7 Complete sample session (7 entries)

A user prompt + one assistant turn that calls a tool, gets a result, and ends:

```jsonl
{"kind":"session","version":2,"id":"f7173577969c88148cb5bf3c16767d87","timestamp":"2026-09-23T13:00:00.000Z","cwd":"/home/leroy/Project/agentic-with-pi","model":"MiniMax-M3","max_turns":200,"system":"You are a coding assistant...","tools":["read","write","edit_match","bash","agentgrep","glob","ls","invalid"]}
{"kind":"message","version":2,"id":"0190a3b7-0001-7c8a-9000-000000000001","parentId":null,"timestamp":"2026-09-23T13:00:00.789Z","message":{"role":"user","content":[{"type":"text","text":"Help me remove jcode"}]}}
{"kind":"message","version":2,"id":"0190a3b7-0002-7c8a-9000-000000000002","parentId":"0190a3b7-0001-7c8a-9000-000000000001","timestamp":"2026-09-23T13:00:01.234Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"The user wants to remove jcode from somewhere.","thinkingSignature":"b1a6a"},{"type":"text","text":"I'll search the codebase first."},{"type":"toolCall","id":"call_0190a3b7","name":"agentgrep","intent":"find jcode","arguments":{"pattern":"jcode"}}]},"stopReason":"toolUse","usage":{"promptTokens":1024,"completionTokens":171,"totalTokens":1195}}
{"kind":"message","version":2,"id":"0190a3b7-0003-7c8a-9000-000000000003","parentId":"0190a3b7-0002-7c8a-9000-000000000002","timestamp":"2026-09-23T13:00:02.456Z","message":{"role":"toolResult","content":[{"type":"text","text":"42 matches found in 12 files..."}]},"details":{"toolName":"agentgrep","intent":"find jcode","isError":false}}
{"kind":"message","version":2,"id":"0190a3b7-0004-7c8a-9000-000000000004","parentId":"0190a3b7-0003-7c8a-9000-000000000003","timestamp":"2026-09-23T13:00:05.789Z","message":{"role":"assistant","content":[{"type":"text","text":"Found 42 matches. Here's the cleanup plan..."}]},"stopReason":"end_turn","usage":{"promptTokens":1024,"completionTokens":312,"totalTokens":1336}}
{"kind":"custom","version":2,"id":"0190a3b7-0005-7c8a-9000-000000000005","parentId":"0190a3b7-0004-7c8a-9000-000000000004","timestamp":"2026-09-23T13:00:06.000Z","customType":"tool_error","data":{"error":"file lock contention on foo.go"}}
{"kind":"custom","version":2,"id":"0190a3b7-0006-7c8a-9000-000000000006","parentId":"0190a3b7-0005-7c8a-9000-000000000005","timestamp":"2026-09-23T13:00:30.000Z","customType":"abort","data":{"reason":"tool failed 3 times"}}
```

## 3. Target wire format

JSON-RPC 2.0 `Response` envelope, line-delimited, over the existing Unix socket. The only thing that changes is the `event` field value and the `data` payload shape.

### 3.1 New event: `message`

```json
{"jsonrpc":"2.0","id":"<req-id>","event":"message","data":{
  "id":"0190a3b7-0002-7c8a-9000-000000000002",
  "parentId":"0190a3b7-0001-7c8a-9000-000000000001",
  "timestamp":"2026-09-23T13:00:01.234Z",
  "message":{"role":"assistant","content":[{"type":"thinking","thinking":"...","thinkingSignature":"..."},{"type":"text","text":"..."},{"type":"toolCall","id":"call_...","name":"agentgrep","intent":"...","arguments":{...}}]},
  "stopReason":"toolUse",
  "usage":{"promptTokens":1024,"completionTokens":171,"totalTokens":1195,"cacheReadTokens":0,"cacheWriteTokens":0}
}}
```

For `toolResult` and `user` roles the same envelope carries `details` (tool result) or omits it (user).

### 3.2 New event: `custom`

```json
{"jsonrpc":"2.0","id":"<req-id>","event":"custom","data":{
  "id":"0190a3b7-0006-7c8a-9000-000000000006",
  "parentId":"0190a3b7-0005-7c8a-9000-000000000005",
  "timestamp":"2026-09-23T13:00:30.000Z",
  "customType":"abort",
  "data":{"reason":"tool failed 3 times"}
}}
```

### 3.3 New event: `custom_message`

```json
{"jsonrpc":"2.0","id":"<req-id>","event":"custom_message","data":{
  "id":"0190a3b7-0007-7c8a-9000-000000000007",
  "parentId":"...",
  "timestamp":"2026-09-23T13:00:35.000Z",
  "customType":"subagent_notification",
  "content":"subagent task T-42 finished: ...",
  "display": true,
  "details":{}
}}
```

Phase 1 TUI consumes this as log-only; Phase 2 adds chat-pane rendering.

### 3.4 Events that disappear from the wire in Phase 1

| Old event name     | Replacement                                  |
|--------------------|----------------------------------------------|
| `thought_start`    | folded into one `message` with `thinking` part |
| `thought_chunk`    | folded into one `message` with `thinking` part |
| `thought_end`      | folded into one `message` with `thinking` part |
| `tool`             | `message` with `toolCall` content part       |
| `observe`          | `message` with `role:toolResult`             |
| `final_answer`     | `message` with `stopReason:end_turn`         |
| `error`            | `custom` with `customType:tool_error` OR `abort` |

Events that stay (not affected): `session_started`, `session_resumed`, `pong`, `sessions_list`, `cancelled`.

### 3.5 Streaming-feel tradeoff

The wire `message` event carries a **complete aggregated assistant turn** (thinking + text + toolCalls all in one envelope). The TUI no longer receives incremental thought chunks from the wire. The TUI's existing incremental-display logic (per `internal/tui/chat.go`) must be reworked to render from the aggregated message shape.

This is **intentional** per the goal contract SC2: the on-wire message shape mirrors the on-disk message shape so logs and live stream share one canonical representation. If incremental TUI rendering is critical, revisit in a separate goal.

## 4. Migration phases

### 4.1 Phase 1 (this goal GC-2026-001)

- New `kind:message` / `kind:custom` / `kind:custom_message` schema on disk.
- New `event:message` / `event:custom` / `event:custom_message` on the wire.
- Legacy reader in `internal/agent-server/sessions.go` keeps parsing `kind:event` lines.
- `handleResume` plays back old sessions with the legacy wire events (so old TUI clients in mixed deployments still work) and new sessions with the new wire events.
- `kind:compaction` is reserved (the entry shape is defined in code) but no trigger writes one. Zero production compaction entries in Phase 1.
- `session` entry keeps `system` / `model` / `max_turns` / `tools` inline. Adds `cwd` and `version:2`.
- `session_id` stays as 32 hex; `message_id` becomes UUID v7.

### 4.2 Phase 2 (future goal)

- Move `system` out of `session` entry into a `role:system` first message.
- Wire up `compaction` writes (trigger in `internal/agent-core/compact.go`).
- Optionally split `model` / `max_turns` / `tools` into their own entries if session-internal model switching is ever added.
- `custom_message` for subagent notifications (when subagent system lands).
- Optionally drop legacy reader once all in-the-wild sessions are ≥ version 2.

## 5. ID strategy

### 5.1 `session_id`

Stays as 32 hex chars (16 random bytes hex-encoded via `crypto/rand`). Rationale:

- Already used everywhere in the AWP toolchain: file paths under `~/.awp/logs/sessions/`, RPC params, resume URLs.
- All legacy sessions in `~/.awp/logs/sessions/` have 32-hex ids. A change would force a migration of 7387+ files (verified empirically).
- pi-coding-agent uses UUID v7 for its session ids; AWP does not. The two ecosystems stay distinct on disk (`~/.awp/logs/sessions/<32hex>.jsonl` vs `~/.pi/agent/sessions/<cwd>/<iso>_<uuidv7>.jsonl`).

Implementation: keep `agentserver.NewID()` (`internal/agent-server/sessions.go:185`) as-is.

### 5.2 `message_id`

Switches to UUID v7 per RFC 9562 §5.7. Layout:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           unix_ts_ms                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          unix_ts_ms           |  ver  |       rand_a          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|var|                        rand_b                             |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                            rand_b                             |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

48-bit `unix_ts_ms` | 4-bit `ver=0x7` | 12-bit `rand_a` | 2-bit `var=0b10` | 62-bit `rand_b`.

Properties:

- Time-ordered: lexicographic sort ≈ chronological. `parentId` chains sort naturally.
- 100 ns resolution through `unix_ts_ms` is enough for AWP (one session has at most ~thousands of messages).
- Monotonic across rapid calls within the same ms via the low bits of `rand_b` being a per-ms counter, with `crypto/rand` reseeding between ms.

Canonical 8-4-4-4-12 hex form (matches pi-coding-agent's wire format).

### 5.3 Go snippet for UUID v7 generation

```go
package json_rpc

import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "sync"
    "time"
)

type uuidv7Gen struct {
    mu      sync.Mutex
    lastMs  int64
    counter uint16
}

func (g *uuidv7Gen) Next() string {
    g.mu.Lock()
    ms := time.Now().UnixMilli()
    if ms == g.lastMs {
        g.counter++
    } else {
        g.counter = 0
        g.lastMs = ms
    }
    c := g.counter
    g.mu.Unlock()

    var rb [10]byte
    _, _ = rand.Read(rb[:])

    b := make([]byte, 16)
    b[0] = byte(ms >> 40)
    b[1] = byte(ms >> 32)
    b[2] = byte(ms >> 24)
    b[3] = byte(ms >> 16)
    b[4] = byte(ms >> 8)
    b[5] = byte(ms)
    b[6] = 0x70 | (rb[0] & 0x0f)
    b[7] = rb[1]
    b[8] = 0x80 | (rb[2] & 0x3f)
    b[9] = rb[3]

    lo := uint16(c) & 0x0fff
    b[8] = (b[8] & 0xf0) | byte(lo>>8)
    b[9] = byte(lo)

    copy(b[10:], rb[4:])

    s := hex.EncodeToString(b)
    return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

var defaultUUIDv7 = &uuidv7Gen{}

func NewV7() string { return defaultUUIDv7.Next() }
```

Counter rides in low bits of `rand_b`'s upper byte so collisions stay impossible even if `rand` returns the same `rand_a`/`rand_b` twice.

## 6. Compaction deferral rationale

### 6.1 Current state

`internal/agent-server/sessions.go:62` defines `Store.WriteCompaction` and the agent side has `writeCompactionLocked` (`internal/agent-core/logger.go:124`). Both hooks exist. **Neither is ever called.**

Empirical check: scanning `~/.awp/logs/sessions/*.jsonl` (7387 files, 153 MB total) shows **zero `kind:compaction` entries**. Every session terminates with a `final_answer` or `error` event, never with a compaction mid-flight.

### 6.2 Why deferral is safe

- No production code path triggers compaction today. Users are not depending on it.
- Without a wire-format change, compaction entries that did exist would be opaque to the TUI anyway.
- Reserving the entry type costs nothing and unblocks Phase 2.

### 6.3 Phase 2 trigger blueprint

The trigger lives in `internal/agent-core/compact.go` (currently a stub). To wire it up:

1. **Call site** — after each `EventThoughtEnd` in the agent ReAct loop, check `len(messages) > maxTokens` (configurable; default 100k tokens). If true, call `compact.Compact(ctx, messages)`.
2. **Compact function** — given a `[]llm.Message`, return `(summary string, kept []llm.Message, tokensBefore int, tokensAfter int)`. Drop the oldest messages until under threshold, summarize dropped ones via a `summary` model call.
3. **Persist** — call `Store.WriteCompaction(...)` with the summary, then re-seed `sessionStates` with the kept slice.
4. **Wire emit** — emit a `compaction` entry via the new message/custom pathway. (Phase 2; not done here.)

Until Phase 2, compaction is documented as a known limitation: long sessions consume more context tokens than necessary because nothing compacts them.

## 7. Wire replay strategy (`handleResume`)

`handleResume` in `internal/agent-server/handler.go:175` reads a session JSONL and replays events to the client. After Phase 1 it must handle both schemas:

```text
for each entry in JSONL:
    switch on entry.kind:
        case "session":
            emit wire "session_started" with id, cwd, model, max_turns, system, tools
        case "message" (v2):
            emit wire "message" with payload {id, parentId, timestamp, message, stopReason?, usage?, details?}
        case "custom" (v2):
            emit wire "custom" with payload {id, parentId, timestamp, customType, data}
        case "custom_message" (v2):
            emit wire "custom_message" with payload {id, parentId, timestamp, customType, content, display?, details?}
        case "compaction" (v2):
            emit wire "custom" with customType="compaction_summary" + data (Phase 1: stub it)
        case "event" (v1 legacy):
            switch on event sub-kind (thought_start/chunk/end, tool, observe, final_answer, error):
                emit wire with the LEGACY event name + payload (so old TUI clients keep working)
        emit wire "session_resumed" with {event_count, schema_version}
```

This means **legacy wire event names remain defined** (`EventThoughtStart` etc. in `internal/agent-protocol/json_rpc/methods.go`) but `handlePrompt`'s live stream no longer emits them. Tests that assert on legacy names during live stream MUST be updated; tests that assert on legacy names during resume MUST stay.

Mixed deployments during rollout: an old TUI client connecting to a new server and resuming an old session sees the old event names — works. An old TUI client connecting to a new server and resuming a new session sees `event:"message"` — **broken** until both sides upgrade. Documented in rollout plan (section 9).

## 8. Risk register

| # | Risk                                                        | Likelihood | Impact | Mitigation                                                                                          |
|---|-------------------------------------------------------------|------------|--------|-----------------------------------------------------------------------------------------------------|
| 1 | Schema break: old TUI cannot parse new `event:"message"`    | High during rollout | User sees broken chat pane | Paired release: ship new TUI + new server in one version. Old TUI + new server only works for resume of legacy sessions. |
| 2 | Schema break: old server reads new JSONL files              | Low        | Crash on load | New JSONL has `version:2`; old `Load` (without version check) parses opportunistically and may mis-execute. Verify with fixture test in P4. |
| 3 | Streaming feel loss: TUI no longer renders chunks          | High       | UX regression | Documented in §3.5; revisit in follow-up goal if user-visible.                                     |
| 4 | Compaction data loss: long sessions never get compacted     | Medium     | Token cost grows linearly | Acceptable for Phase 1; Phase 2 implements compaction (blueprint in §6.3).                          |
| 5 | UUID v7 collision                                         | Very low   | Parent chain corruption | Counter-in-low-bits fallback (§5.3). Unit-tested for 1000-call monotonicity in P2.                  |
| 6 | `handleResume` replay order diverges from live emit order  | Medium     | Resumed sessions look different from live ones | Single source of truth: the same `Message`/`Custom` types drive both replay and live emit (P5).      |
| 7 | Migration mistake: forget to write `version:2` on new entries | Low      | Old loader misreads new file | Schema version check in `Load` (P4) rejects mismatched major version.                              |
| 8 | Concurrent write corruption: append-only writer + new code  | Low        | Lost tail entries | Keep the existing `bufio.Writer + flock` pattern from `Store.append`.                               |

## 9. Rollback plan

### 9.1 Disk schema rollback

Easy. Reverting the server commit means it writes `kind:event` again (the legacy writer stays in code as `writeEvent`). Old session files remain readable either way. No data migration needed.

### 9.2 Wire format rollback

Harder. Old TUI clients cannot parse `event:"message"`. Options in order of preference:

1. **Paired release** (recommended): tag a new `awp` version that ships new server + new TUI atomically. Users on the old version do not upgrade; users on the new version get both.
2. **Versioned wire events** (Phase 2 stretch): add an `awp-version` field to the JSON-RPC `Request` and have server emit legacy events when client version < X. Cost: ~50 lines, one extra field everywhere. Not worth it for one release.
3. **Hard cutover** (current plan): accept a brief break for users who mix versions. Document in CHANGELOG.

### 9.3 Deployment sequence

```
1. Tag & publish new awp with new server + new TUI (paired).
2. Document in CHANGELOG: "sessions created by awp >= Y require new awp to resume."
3. Monitor for `Load` failures / `event:"message"` parse errors.
4. If a critical regression is found: revert to previous tag; users on the previous version resume their previous-version sessions without issue.
```

## 10. Open questions / decisions deferred

1. **`thinkingSignature` round-trip.** pi's signatures are opaque hashes the model uses to identify its own reasoning blocks. AWP currently doesn't persist them. Should Phase 1 persist them on `EventThoughtEnd` so the model can carry reasoning across compaction? **Defer to Phase 2.** Phase 1 leaves `thinkingSignature` empty on the JSONL entry.

2. **`customType` vocabulary control.** Phase 1 reserves `tool_error` and `abort`. Phase 2 adds `skill_invoked`, `todo_updated`, `subagent_notification`. Should there be a registry check at write time? **Defer.** Phase 1 accepts any `customType` string.

3. **`details` field on toolResult messages.** pi attaches a rich `details` object (tool-specific: `exit_code`, `result_count`, etc.). AWP currently flattens this into top-level fields. **Defer:** Phase 1 keeps the flattened shape (`details.toolName`, `details.intent`, `details.isError`). Future goals can add `details.exit_code` etc. as needed.

4. **Should `session` entry track `parent_session_id`?** For branching (resume → new prompt → save as new session). pi doesn't; AWP doesn't. **Defer.**

5. **Should the wire emit a per-message ack?** Some TUI implementations want to know a message has been persisted to JSONL. AWP doesn't currently. **Defer.**

6. **`EventUserMessage` lifecycle.** Currently AWP's logger writes `EventUserMessage` as a `kind:event` with `category:user_message`. After Phase 1, this becomes a `role:user` message entry. The `LogEventForTest` call site in `handler.go:138` stays; only the writer changes. **Confirmed scope.**

7. **Should session-level metadata (model/tools) be in every message?** pi doesn't (they're in the session entry); AWP follows the same pattern. **Confirmed: only in `session` entry.**

## Appendix A — pi-coding-agent reference entries (sampled)

```jsonl
{"type":"session","version":3,"id":"019f70a2-5850-7d3d-80b1-e21921415392","timestamp":"2026-07-17T15:11:55.472Z","cwd":"/home/leroy/Project/agentic-with-pi"}
{"type":"model_change","id":"67b555f7","parentId":null,"timestamp":"2026-07-17T15:11:56.017Z","provider":"minimax-cn","modelId":"MiniMax-M3"}
{"type":"thinking_level_change","id":"4d6553ee","parentId":"67b555f7","timestamp":"2026-07-17T15:11:56.017Z","thinkingLevel":"high"}
{"type":"message","id":"45b6feea","parentId":"4d6553ee","timestamp":"2026-07-17T15:11:58.223Z","message":{"role":"user","content":[{"type":"text","text":"/q"}],"timestamp":1784301117783}}
{"type":"message","id":"f53e649d","parentId":"45b6feea","timestamp":"2026-07-17T15:11:59.746Z","message":{"role":"assistant","content":[],"api":"anthropic-messages","provider":"minimax-cn","model":"MiniMax-M3","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"total":0}},"stopReason":"aborted","timestamp":1784301118275,"errorMessage":"Operation aborted"}}
```

Notable differences from AWP target (Phase 1):

- pi uses UUID v7 for session_id; AWP keeps 32 hex (§5.1).
- pi has `model_change` and `thinking_level_change` entries; AWP does not (anti-goal #1).
- pi's `message.content[]` allows empty array (the `/q` abort case); AWP target will too (model decides when content is empty).
- pi's `usage` includes `cost` object; AWP target omits cost (out of scope; AWP doesn't compute token cost).

## Appendix B — AWP current writer (legacy reference)

The current writer lives in `internal/agent-core/logger.go:33`. One assistant turn produces six `kind:event` lines:

```jsonl
{"kind":"event","seq":1,"at":"...","category":"thought_start"}
{"kind":"event","seq":2,"at":"...","category":"thought_chunk","reasoning":"The user wants..."}
{"kind":"event","seq":3,"at":"...","category":"thought_chunk","reasoning":" \"hi\""}
{"kind":"event","seq":4,"at":"...","category":"thought_chunk","content":"Hi there!"}
{"kind":"event","seq":5,"at":"...","category":"tool","tool_name":"bash","tool_args":"{...}","tool_calls_count":1}
{"kind":"event","seq":6,"at":"...","category":"observe","tool_name":"bash","content":"exit 0","tool_error":""}
{"kind":"event","seq":7,"at":"...","category":"final_answer","content":"Hi there!"}
```

After Phase 1 this becomes four `kind:message` lines plus one `kind:custom` for the abort case (if any). See §2.7 for the complete target.

## Appendix C — AWP current wire events (legacy reference)

`internal/agent-protocol/json_rpc/methods.go:13` defines the wire constants. The full list as of Phase 1:

```go
EventThoughtStart = "thought_start"
EventThoughtChunk = "thought_chunk"
EventThoughtEnd   = "thought_end"
EventTool         = "tool"
EventObserve      = "observe"
EventFinalAnswer  = "final_answer"
EventError        = "error"
EventCancelAck    = "cancelled"
```

After Phase 1:

- Live stream (`handlePrompt`) emits only: `session_started`, `message`, `custom`, `custom_message`, `cancelled`, `pong`, `sessions_list`, `session_resumed`.
- Replay (`handleResume`) emits legacy names when reading legacy JSONL, new names when reading new JSONL. The constants stay defined so legacy tests compile.

## Appendix D — File-touch map (anti-scope check)

| File                                            | Touched by task | Notes                                                                              |
|-------------------------------------------------|-----------------|------------------------------------------------------------------------------------|
| `internal/agent-core/logger.go`                 | P3              | `writeEvent` stays for legacy; new `writeAlignedEvent` + `StreamBuffer` added.      |
| `internal/agent-core/stream_buffer.go`          | P3 (NEW)        | Per-message accumulator.                                                            |
| `internal/agent-core/uuidv7.go`                 | P2 (NEW)        | UUID v7 generator (in `agent-protocol/json_rpc/`, not `agent-core/`).               |
| `internal/agent-server/sessions.go`             | P4              | Reader extended with new-schema path; legacy reader unchanged.                     |
| `internal/agent-server/handler.go`              | P4, P5          | `handleResume` schema-aware; `handlePrompt` switched to `WireEmitter`.             |
| `internal/agent-server/stream_emitter.go`       | P5 (NEW)        | Wire counterpart of `StreamBuffer`.                                                |
| `internal/agent-protocol/json_rpc/methods.go`   | P2              | Adds `EventMessage` / `EventCustom` / `EventCustomMessage` constants.              |
| `internal/agent-protocol/json_rpc/messages.go`  | P2 (NEW)        | Wire types `MessageEvent`, `CustomEvent`, `CustomMessageEvent`, etc.               |
| `internal/agent-protocol/json_rpc/uuidv7.go`    | P2 (NEW)        | UUID v7 generator.                                                                  |
| `internal/tui/events.go`                        | P6              | `handleServerEvent` switches on new event kinds.                                    |
| `internal/tui/app.go`                           | P6              | `Update` red-dot logic reads `customType` instead of `EventError`.                 |
| `internal/tui/messages.go`                      | P6 (NEW)        | TUI-side parse helpers.                                                             |
| `internal/agent-core/agent.go`                  | **NOT touched** | ReAct loop unchanged.                                                               |
| `internal/agent-core/compact.go`                | **NOT touched** | Compaction stub stays; no trigger added.                                            |
| `internal/tui/chat.go`, `types.go`, `styles.go` | **NOT touched** | TUI rendering unchanged.                                                            |
| `internal/tui/{header,picker,autocomplete}.go`  | **NOT touched** | Out of scope per goal contract.                                                     |
| `internal/llm/*`                                | **NOT touched** | Provider layer is independent of session log schema.                                 |

## Appendix E — Glossary

| Term                 | Definition                                                                                  |
|----------------------|---------------------------------------------------------------------------------------------|
| `session_id`         | 32 hex chars identifying one AWP session. Persists for the life of the session file.         |
| `message_id`         | UUID v7 (Phase 1) identifying one entry in a session. Time-ordered.                         |
| `parentId`           | The `message_id` of the previous message in the same session's chain. `null` only for the first message after the session entry. |
| `content part`       | A typed element inside `message.content[]`. Allowed types in Phase 1: `text`, `thinking`, `toolCall`. |
| `stopReason`         | Model-side stop reason: `end_turn`, `toolUse`, `max_tokens`, `stop_sequence`.               |
| `customType`         | Discriminator on `custom` and `custom_message` entries. Reserved vocabulary in §6.2.        |
| `wire event`         | The `event` field of a JSON-RPC `Response`. One per line over the Unix socket.              |
| `streaming feel`     | The visual experience of thought chunks arriving incrementally on the TUI. Lost in Phase 1. |
