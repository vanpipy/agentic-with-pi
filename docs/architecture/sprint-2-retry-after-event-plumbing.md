# Sprint 2 — Retry-After Event Plumbing Audit

**Status:** finding from Sprint 2 commit `a9bd3a7` (Retry-After wire-up).
**Action recommended:** delete the inert field. See §4.

## 1. Finding

`internal/llm/events.go:33-35` declares

```go
type EventErr struct {
    Err            error
    RetryAfterSecs int
}
```

The `RetryAfterSecs` field is **fully inert**:

| Direction | Evidence |
|---|---|
| **Set by production code** | Never. `grep -rn 'EventErr{RetryAfter' internal/` returns 0 hits. The two `EventErr{...}` constructions in `internal/llm/protocol/anthropic/anthropic_sse.go` (lines 190, 439) both omit the field, leaving it zero. The only writers are `test/llm/events_test.go:24` and `:149`. |
| **Read by production code** | Never. `internal/agent-core/stream/typed_processor.go:68-72` is the sole consumer: |

```go
case llm.EventErr:
    if ctx.Err() != nil {
        continue_ = false
        return
    }
    emits = append(emits, EmitEvent{Kind: EmitKindError, Content: e.Err.Error()})
    continue_ = false
```

The `e.RetryAfterSecs` value is dropped on the floor. `e.Err.Error()` is forwarded; the retry-after information is not.

## 2. What the user actually sees today

The pre-stream HTTPError path **does** honor `Retry-After`:

```
client → llm.NewCore → protocol.NewHTTPRest.Stream → 429 + Retry-After: 5
        → returns *protocol.HTTPError with .RetryAfter = 5s
        → RetryCore.StreamChat extracts via retryafter.RetryAfterFromAny
        → sleeps 5s → retries → success
```

But the user sees only:
- A delay proportional to the hint (good)
- A `slog.Debug("llm: retry-after hint honored", ...)` line (default level INFO → suppressed in production)

The mid-stream path produces an `EventErr` (e.g. anthropic SSE `error` event) but the field is never populated, so the consumer never observes a retry-after.

## 3. Reference check (jcode)

`~/Project/jcode/crates/jcode-provider-core/src/retry_after.rs` and its consumers — `Retry-After` flows through the **HTTP** error path (`HTTPError`) and the **typed** `StreamEvent::Error { retry_after: Option<Duration> }`. The provider is the producer; the runtime is the consumer. In jcode the field has a real producer (`EventStreamItem::Error` construction in `sse.rs`) and a real consumer (the runtime retry coordinator).

awp's `EventErr.RetryAfterSecs` is a half-port: the field exists but neither side is wired.

## 4. Decision (per golden rule "no forward compatibility on internal/")

The internal boundary has no external consumers to protect. Three options:

| Option | Effort | Effect |
|---|---|---|
| **A. Delete the field** | Tiny — remove 1 line from `events.go`, drop 1 test case. | Loses nothing (no producer, no consumer). Simplest honest answer. |
| **B. Wire producer + consumer** | Medium — populate `EventErr.RetryAfterSecs` in `anthropic_sse.go:439` from the upstream error stream (Anthropic's `error` SSE event does carry `retry_after` in some error variants); teach `typed_processor.go` to forward it via a new `EmitKind` or attach to the existing `EmitKindError` content. | Real value to the user: TUI can show "rate limited, retrying in 5s" instead of silence. |
| **C. Leave as-is** | Zero. | Continues to be inert; misleads future readers. |

**Recommendation: A for this sprint, B as a Sprint 4+ candidate if the TUI gains an event-driven retry banner.**

Per project memory tag `alignment-audit` this is recorded for the next phase.

## 5. Sprint 2 commits that landed despite this finding

| Commit | Surface | Why safe despite inert `RetryAfterSecs` |
|---|---|---|
| `9600562` | `retryafter` primitive | Independent of the event |
| `a9bd3a7` | HTTPError + llm.Error + RetryCore + bridge | Pre-stream path works; EventErr path was already broken pre-sprint |
| `ef7ea3b` | Mid-stream rollback test | Tests `RetryCore.forwardWithMidRetry` retry loop, not the event plumbing |
| `73adfd4` | E2E test through real public API | Pre-stream HTTPError path, which is fully working |

The inert field does not affect Sprint 2 correctness — every retry path that *should* work, *does* work. It only matters for downstream observability of *why* a retry happened mid-stream.