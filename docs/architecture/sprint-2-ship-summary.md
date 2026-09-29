# Sprint 2 — Ship Summary

**Sprint:** Retry-After (planned, executed, audited, merged)
**Branch:** `ws-llm-retry-after`
**Status on main:** merged at commit `5c8ffbd`; post-phase audit + Sprint 3 scout landed in follow-ups `bd20ad7`, `935aa9b`, `3ffe696`, `0b92dc5`
**Push:** local only (overlay §2 requires explicit "yes")

## What changed

| | LOC | Files |
|---|---|---|
| Code added | ~480 | `internal/llm/retryafter/`, `retryafter_bridge.go`, plus edits to `protocol/`, `errors.go`, `retry.go` |
| Tests added | 855 | 34 unit (`test/llm/retryafter/`) + 11 integration + e2e (`test/llm/retry_*`) |
| Docs added | 271 | `sprint-2-retry-after-event-plumbing.md` (82) + `sprint-2-alignment-audit.md` (65) + this file (124) |

## Eight commits (in order)

| SHA | Type | What |
|---|---|---|
| `9600562` | code | Port `retryafter` primitive (subpackage mirroring jcode's `retry_after.rs`) |
| `a9bd3a7` | code | Wire `HTTPError.RetryAfter` + `llm.Error.RetryAfter` + `RetryCore.computeDelay` + `RetryAfterFromAny` bridge |
| `ef7ea3b` | test | Cover mid-stream rollback hint path (`forwardWithMidRetry` 77.8% → 80.6%) |
| `73adfd4` | test | End-to-end through real public API: httptest → real `NewHTTPRestWithClient` → real `MiniMaxProvider` → `RetryCore`. **Observed 5.04s elapsed** for a `Retry-After: 5` |
| `935aa9b` | doc | Audit finding: `EventErr.RetryAfterSecs` is fully inert (no producer, no reader in production code) |
| `bd20ad7` | doc + nit | Post-phase alignment audit (vs jcode `retry_after.rs:219` LOC) + fix duplicate `httpDateLayouts` entries (RFC 7231 mandates only RFC1123 + RFC850) |
| `3ffe696` | doc | Sprint 3 cross-provider failover scout |
| `0b92dc5` | doc | Sprint 3 scout appendix: full `pick_next_fallback_route_with_options` algorithm captured (filter + score stages + 11 test cases) |

(Plus merge commit `5c8ffbd`.)

## Verification

- `go test -race -count=1 ./...` (16 packages) — green
- `internal/llm/...` coverage from `test/llm/` alone: **74.4%**; key functions `computeDelay` / `Classify` / `RetryAfterFromAny` / `StreamChat` / `forwardWithMidRetry` at 80–100%
- Mutation-tested twice: cap-branch removal fails `TestParseRetryAfter_SaturatesOnOverflowDigitString`; `computeDelay` bypass fails `TestRetryCoreHonorsServerHintInBackoff`
- `awp` binary builds (25 MB); `awp --help` works; `AWP_DEMO=1 awp demo` connects to real provider end-to-end
- `go mod verify` clean
- Remote `github.com/vanpipy/agentic-with-pi.git`; `origin/main` at `f77c18e` (pre-Sprint-2 baseline); local main ahead by 8 commits

## Surfaced findings (Sprint 2 follow-ups)

1. **`EventErr.RetryAfterSecs` is inert** — `internal/llm/events.go:33` declares the field but no production code reads or writes it. Only `test/llm/events_test.go:24,149` writes it. Only `typed_processor.go:71` reads (and drops) it. **Recommended:** delete the field per golden rule "no forward compatibility on internal/". Sprint 3 or a tiny closeout commit.
2. **`time.Time` vs `Instant`** — Sprint 2 uses wall-clock `time.Time` for the `RetryAfter` deadline; jcode uses monotonic `Instant`. Wall clock is vulnerable to NTP jumps. **Severity:** low; revisit when seconds-granularity hints start causing race conditions in practice.
3. **Two-track retry-after hint path** — `HTTPError.RetryAfter` field (pre-stream) and explicit `WithRetryAfter` wrap coexist; `RetryAfterFromAny` bridge prefers the wrap. Documented as intentional in `docs/architecture/sprint-2-alignment-audit.md`.

## Sprint 3 readiness

- Scout doc at `docs/architecture/sprint-3-failover-scout.md` (sections 1–7)
- Four open questions for user direction:
  1. Real second provider for e2e vs `EchoProvider` stub?
  2. Extend `Model` with auth-method tags vs new `ModelRoute` struct?
  3. `failover.classify` replace or layer with `errors.classify`?
  4. Automatic failover vs UI prompted failover?
- Entry criteria in scout §6: user confirms scope + answers 4 questions; new worktree `ws-llm-failover` cut from `main` at `5c8ffbd` (or current `0b92dc5`)