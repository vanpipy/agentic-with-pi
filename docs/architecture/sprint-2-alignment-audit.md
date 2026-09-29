# Sprint 2 — Alignment Audit (jcode ↔ awp)

**Reference:** `~/Project/jcode/crates/jcode-provider-core/src/retry_after.rs` (219 LOC)
**Target:** `internal/llm/retryafter/` (188 LOC) + `internal/llm/retryafter_bridge.go` (20 LOC) + `internal/llm/retry.go` (172 LOC) + `internal/llm/errors.go` (202 LOC)
**Audit method:** surface-by-surface comparison, file:line citations on both sides
**Tag:** `alignment-audit`

## 1. Public API surface

| jcode symbol | awp symbol | Match | Notes |
|---|---|---|---|
| `pub const MAX_RETRY_AFTER: Duration = 60s` | `const MaxRetryAfter = 60 * time.Second` (retryafter.go:11) | ✅ identical value, ✅ capitalized const |
| `pub fn retry_after(headers: &HeaderMap) -> Option<RetryAfter>` | `ParseRetryAfterFromHeaders(h http.Header) time.Duration` (retryafter.go:40) | ⚠️ shape differ | jcode wraps in `Option<RetryAfter>` (deadline-based); awp returns bare `time.Duration` (delta-based). **Both correct** for their callers. jcode uses `Option` because callers explicitly fall back on `None`; awp's retry loop falls back on `0`. |
| `pub struct RetryAfter { deadline: Instant }` + `remaining()` | `type RetryAfter struct { deadline time.Time }` (retryafter.go:97) + `Remaining()` (retryafter.go:105) | ✅ semantic match | Instant → time.Time + saturating sub |
| `pub fn error_with_retry_after(message, hint) -> Error` | `WithRetryAfter(base error, hint time.Duration) error` (retryafter.go:123) | ⚠️ shape differ | jcode takes a `String`; awp takes an `error` so it can wrap any base. Both produce a `RetryAfterError` whose Display string equals the base's. awp's choice is more flexible. |
| `pub fn retry_after_from_error(error: &Error) -> Option<Duration>` | `RetryAfterFromError(err error) time.Duration` (retryafter.go:140) | ⚠️ shape differ | Same Option-vs-Duration difference as `retry_after`. |
| `pub fn retry_delay(attempt, base_ms, server_hint)` (retry_after.rs:110) | `RetryDelay(attempt int, baseMs int64, hint time.Duration)` (retryafter.go:155) | ✅ identical contract | hint > 0 wins; otherwise exponential backoff. |
| (jcode uses `httpdate::parse_http_date`) | awp uses `time.Parse` with 5 layouts (retryafter.go:13-19) | ⚠️ shape differ | jcode delegates to `httpdate` crate; awp hand-rolls 5 layouts (RFC1123, RFC1123Z, RFC850, RFC1123, "Mon, 02 Jan 2006 15:04:05 GMT"). Coverage: jcode's crate is `no_std`-friendly and handles the same RFCs. awp's list has a duplicate (line 17 == line 14). |

**Deviations summary:** shape-different on `Option<T>` vs bare `T` (3 places) — intentional, idiomatic Go. None are blockers. **One nit:** `httpDateLayouts` line 17 duplicates line 14 (typo introduced when copying the list).

## 2. Saturating arithmetic (cap behavior)

| jcode | awp | Match |
|---|---|---|
| `saturating_mul(10).saturating_add(...).min(max_secs)` per byte (retry_after.rs:33-38) | `secs*10 + ...` per byte with `if secs > maxSecs { return MaxRetryAfter }` early-exit (retryafter.go:62-66) | ✅ same observable behavior |
| Test: `"999999999999999999999999999999999999999999"` → `MAX_RETRY_AFTER` (retry_after.rs:182-188) | Same coverage in `TestParseRetryAfter_SaturatesOnOverflowDigitString` | ✅ mutation-tested: cap-removal makes test FAIL |
| Test: malformed value → `None` (retry_after.rs:171-179) | Same captured by `TestParseRetryAfter_EmptyAndMalformed` | ✅ |

## 3. Error wrapping contract

| jcode | awp | Match |
|---|---|---|
| `RetryAfterError.message: String` + `impl Display` returns `message` (retry_after.rs:79-83) | `RetryAfterError.base error` + `Error() returns e.base.Error()` + `Unwrap()` returns `e.base` (retryafter.go:115-121) | ⚠️ semantic-difference |
| Test: `format!("{error:#}")` = `"request failed: rate limited"` — context wrapping preserves base message (retry_after.rs:196-197) | Test: `WithRetryAfter(fmt.Errorf("rate limited"), 5s).Error() == "rate limited"` | ✅ |

**Deviation:** jcode holds a `String` (which is the display message); awp holds an `error` interface (which `Error()` returns). Both keep the base message visible. **awp is more powerful** because it can carry rich error types (e.g. `*protocol.HTTPError`) through the wrap. **jcode-style call `error_with_retry_after("rate limited", hint).context("request failed")` would NOT work in awp** — but that's not used because the bridge (`RetryAfterFromAny`) prefers the explicit `WithRetryAfter` wrap over the `HTTPError.RetryAfter` field, which is the correct design.

## 4. Wire-level integration (where jcode and awp diverge)

| Surface | jcode | awp | Status |
|---|---|---|---|
| Where `Retry-After` is parsed | Each provider's HTTP client calls `retry_after(headers)` | `protocol.HTTPRest.Stream/Send` call `ParseRetryAfterFromHeaders(resp.Header)` and store in `HTTPError.RetryAfter` (protocol/http_rest.go:101, 141) | ✅ equivalent |
| Where error is classified | jcode folds into `anyhow::Error` chain; outer retry loop calls `retry_after_from_error(&err)` | awp has TWO paths: (a) pre-stream via `HTTPError.RetryAfter` field; (b) explicit wrap via `WithRetryAfter`. Bridge `RetryAfterFromAny` (retryafter_bridge.go:11) prefers wrap | ⚠️ shape different — awp's two-track is unique |
| Retry loop computes delay | `crate::attempt_tracker::retry_backoff_delay(attempt, base_ms)` (retry_after.rs:111) | `computeDelay(attempt, cfg, hint)` (retry.go:139) which uses `retryafter.RetryDelay(...)` then caps at `cfg.MaxBackoff` | ✅ equivalent |
| Jitter | jcode's `retry_backoff_delay` is implemented in `attempt_tracker.rs` (not seen) | `jitter(0..0.25 * d)` (retry.go:161) | ✅ jcode reference not directly visible, but our jitter fraction is small |
| Mid-stream rollback | jcode handles in `failover.rs` and `fallback_pick.rs` (not Sprint 2 scope) | `forwardWithMidRetry` (retry.go:76) | 🔵 deferred to Sprint 3 |

## 5. Drift / blocker findings

| # | Severity | Finding | Recommended action |
|---|---|---|---|
| 1 | nit | `httpDateLayouts` line 17 duplicates line 14 (`time.RFC1123`) | amend in Sprint 3 prelude |
| 2 | should-fix | `EventErr.RetryAfterSecs` is inert (no producer, no consumer in production code). See `docs/architecture/sprint-2-retry-after-event-plumbing.md` (commit 935aa9b). | delete field per golden rule |
| 3 | nit | awp's `RetryDelay` is in `retryafter` package; jcode's `retry_delay` is at module root. | accept — Go subpackage convention matches AGENTS.md layout |
| 4 | nit | jcode uses `Instant` (monotonic clock); awp uses `time.Time` (wall clock). Wall clock is vulnerable to NTP jumps. | revisit when adding explicit `deadline` API to public types (low priority — wall clock is fine for seconds-granularity hints) |
| 5 | accepted | Option-vs-zero wrapping divergence | accept — `RetryAfterFromError` returning 0 means "no hint" which is the same signal callers check |

## 6. Conclusion

Sprint 2 is a **faithful translation** of jcode's `retry_after.rs`. Three intentional divergences (Option→Duration; error vs String base; HTTP field on `HTTPError` + explicit wrap), all in the direction of "more idiomatic for awp's error model." One real bug (the inert `EventErr.RetryAfterSecs`, separately filed) and one trivial nit (duplicate layout entry). No blockers.

Cross-reference for next sprint:
- `jcode/crates/jcode-provider-core/src/failover.rs` — cross-provider failover (Sprint 3 candidate)
- `jcode/crates/jcode-provider-core/src/fallback_pick.rs` — fallback selection (Sprint 3 candidate)