# Sprint 3 — Cross-Provider Failover (scout)

**Status:** pre-sprint scout. Awaiting user green light to begin.
**Reference:** `~/Project/jcode/crates/jcode-provider-core/src/{failover,fallback_pick}.rs`
**Sprint 2 baseline:** Sprint 2 retry-after work merged to `main` at commit `5c8ffbd`. Retry-After honored through real public API end-to-end. `internal/llm` now exports `RetryCore` + `retryafter` package + `Classify` bridge.

## 1. What jcode ships

### `failover.rs` (182 LOC)

Three public symbols, all pure (no I/O, no clock):

| Symbol | Shape | Purpose |
|---|---|---|
| `ProviderFailoverPrompt` (failover.rs:6) | struct {from_provider, from_label, to_provider, to_label, reason, estimated_input_chars, estimated_input_tokens} + Serialize/Deserialize | Encodes a one-keypress "switch provider?" offer to the UI |
| `parse_failover_prompt_message(message: &str) -> Option<Self>` (failover.rs:26) | prefix-detect `[jcode-provider-failover]<json>\n<text>` | Round-trip for prompt embedded in an error message |
| `FailoverDecision` enum (failover.rs:33) | `None` / `RetryNextProvider` / `RetryAndMarkUnavailable` | What the retry loop should do after classifying the error |
| `classify_failover_error_message(message: &str) -> FailoverDecision` (failover.rs:69) | keyword classifier on lowercased message | Three buckets: context-too-long → RetryNextProvider; rate/quota/auth → RetryAndMarkUnavailable; otherwise None |

Note: `contains_independent_status_code` (failover.rs:57) is a subtle helper — matches `"413"` but not `"4130"` or `"14130"`. The `iter().any()` with `match_indices` is the only safe way.

### `fallback_pick.rs` (362 LOC)

Picks the index of the best alternative route from a static `&[ModelRoute]` list. Pure function. Ranking (failover.rs:95-99, lines 80-89 in fallback_pick.rs):

1. **Same model, different auth method** (e.g. claude-api fails → claude-oauth works for same model) — least disruptive
2. **Same provider, different model** — sibling model on the provider
3. **Different provider** — last-resort cross-provider hop

Tie-breakers (in order): keep OAuth/subscription logins preferred, then preserve stable catalog order.

Helpers:
- `api_method_is_oauth` (fallback_pick.rs:17) — OAuth logins are flat-rate and most likely to "just work"
- `error_looks_like_credential_failure` (fallback_pick.rs:52) — markers like "token refresh failed", "invalid api key"; used to widen the exclusion (all same-credential routes are broken)
- `pick_next_fallback_route_with_options` — main entrypoint

## 2. What awp already has

| Surface | State | Notes |
|---|---|---|
| `internal/llm/providers/minimax.go` | ✅ exists, 14-method `Provider` interface implemented | Only provider. **Sprint 3 needs at least a stub second provider** to test failover realistically. |
| `internal/llm/errors.go` `IsRetryable`/`IsAuth`/`IsRateLimit` (lines 183-197) | ✅ exists | Partial overlap with `classify_failover_error_message`. **Question: do we replace, layer, or coexist?** |
| `internal/llm/retry.go` `RetryCore.StreamChat` | ✅ exists, hints-aware after Sprint 2 | Doesn't yet know about cross-provider attempts. |
| `internal/llm/protocol/HTTPRest.Stream` | ✅ exists | Provider-agnostic. No provider-specific retry surface. |
| Any failover surface in awp? | ❌ none | Zero `failover`/`fallback` references in `internal/llm/` or `internal/agent-core/` (the only matches are AFT tool fallback). |
| Multi-provider registry? | ❌ none | `cmd/awp/main.go loadAgent()` constructs a single provider per session. |

## 3. Sprint 3 candidate scope

Three atomic commits, like Sprint 2:

### Commit A — port `failover` primitive (small)
- `internal/llm/failover/failover.go` (~200 LOC)
- Mirrors jcode's three symbols + classifier
- Plus a Go-native `classify` that takes an `error` (jcode takes `&str`; awp's error model already wraps rich types via `errors.As`)
- 20+ unit tests mirroring jcode's test set

### Commit B — port `fallback_pick` primitive (medium)
- `internal/llm/failover/fallback_pick.go` (~400 LOC)
- Mirrors jcode's ranking algorithm
- Needs a Go-native `ModelRoute`/`ProviderRoute` struct (currently awp's `Provider` interface is monolithic, no per-model metadata)
- Likely needs `Provider.Models()` output to be extended with auth-method tags
- 25+ unit tests (table-driven across all ranking permutations)

### Commit C — wire into RetryCore + second provider (medium)
- Add stub `EchoProvider` (returns canned errors on demand) for testing
- Extend `RetryCore.StreamChat` to take `[]Provider` and use `failover.classify` to decide whether to retry with next provider vs. same-provider exponential backoff
- Plus integration tests covering: rate-limit → failover; auth → failover; context-too-long → failover; transient server error → no failover
- E2E test through real public API: failover driven by classified error, observed via timing

## 4. Open questions for user

1. **Second provider** — is there a real second provider we should add (claude/openai/etc.) for actual end-to-end testing, or is `EchoProvider` stub sufficient?
2. **`ModelRoute` struct** — currently awp's `Provider.Models() []Model` is one model per entry. jcode's `ModelRoute` carries `api_method: ModelRouteApiMethod`. Do we extend `Model` or co-design a richer route struct?
3. **Replace vs layer** — does `failover.classify` replace `errors.classify` for retry decisions, or coexist? Sprint 2 already shipped `llm.Error.RetryAfter` for hint-aware retry; the question is whether failover uses the same error envelope.
4. **Provider-failover prompt UX** — jcode emits a `[jcode-provider-failover]` error message that the UI parses to offer a one-key switch. Does awp's TUI want the same affordance, or is automatic failover (no prompt) the chosen UX?

## 5. Risks

- **Single-provider reality** means Sprint 3 cannot be fully validated against real providers until at least two real providers exist. The mitigation is `EchoProvider` + the same end-to-end testing pattern as Sprint 2 (httptest + real RetryCore + stub provider).
- **`classify_failover_error_message` is keyword-based** — new provider error messages may need new keywords. Awp's `*protocol.HTTPError` already carries `StatusCode`; we can use HTTP status as the primary signal and keywords as fallback (or the inverse; needs decision).
- **Cross-provider context-window differences** — a model that works on provider A may have a different context window than provider B. The `Model.ContextWindow()` method already exists; `pick_next_fallback_route` should consult it.

## 6. Sprint 3 entry criteria

- [ ] User confirms Sprint 3 scope (one of the three commit shapes; commit granularity TBD)
- [ ] User picks one of the four open questions above
- [ ] New worktree `ws-llm-failover` cut from `main` at `5c8ffbd`
- [ ] `go.sum` copy hygiene per workspace-gotchas memory

## 7. Appendix: `pick_next_fallback_route_with_options` algorithm detail

Captured from `fallback_pick.rs:108-162` so the Sprint 3 implementer has the full reference inline. The function signature:

```rust
pub fn pick_next_fallback_route_with_options(
    routes: &[ModelRoute],
    current_model: &str,
    current_provider: &str,
    current_api_method: &str,
    options: FallbackPickOptions,
) -> Option<usize>
```

Returns the index into `routes` of the best fallback, or `None`.

### Filter stage

For each `route` in `routes` where `route.available`:

1. Skip if `same_model && same_provider && (same_method || unknown_method)` (line 133). The unknown-method case is for remote sessions that didn't track the failed route's auth method — any same-model same-provider route could be the exact failed one, so it's excluded to avoid offering a guaranteed-identical failure.

2. Skip if `options.credential_failure && same_provider && (same_method || unknown_method)` (line 140). A broken OAuth session breaks every model behind it, not just the failed one — exclude all same-credential routes so the offer is one that can plausibly work.

### Score stage

After filtering, score each remaining route as `(tier, prefers_oauth, index)` where:

| `tier` value | Condition | Meaning |
|---|---|---|
| `0` | `same_model && !same_method` | Same model, different auth method (least disruptive) |
| `1` | `same_provider` | Same provider, different model |
| `2` | else | Different provider (last resort) |

`prefers_oauth = 1` (high) when the route's API method is *not* OAuth; `prefers_oauth = 0` (low) when OAuth. Sorting by `(tier ASC, prefers_oauth ASC, index ASC)` puts OAuth candidates first within a tier, with catalog order preserved as the final tiebreaker.

### Return

The first `(tier, prefers_oauth, index)` after `.min()`. Returns `None` when no route survives the filter.

### Test cases (all in fallback_pick.rs:165-361)

| Test | Validates |
|---|---|
| `prefers_same_model_oauth_when_api_key_broken` (line 180) | Tier 0 win: claude-api fails → claude-oauth for same model |
| `falls_back_to_same_provider_sibling_model` (line 194) | Tier 1 win: opus fails → sonnet on same provider |
| `falls_back_cross_provider_as_last_resort` (line 208) | Tier 2 win: only cross-provider option available |
| `skips_unavailable_routes` (line 220) | `route.available == false` is filtered |
| `returns_none_when_only_current_route_exists` (line 235) | Single-route case → None |
| `cross_provider_prefers_oauth_over_api_key` (line 241) | Tier 2 + prefers_oauth tiebreaker |
| `unknown_method_never_offers_same_model_same_provider` (line 255) | Remote-session `current_api_method=""` case |
| `credential_failure_skips_sibling_models_on_same_credential` (line 269) | `options.credential_failure=true` widens exclusion |
| `credential_failure_still_offers_other_method_same_provider` (line 293) | OAuth broken → API key for same model still tier 0 |
| `credential_failure_with_unknown_method_skips_whole_provider` (line 315) | OAuth broken + unknown method → hop provider entirely |
| `classifies_credential_failures` (line 333) | `error_looks_like_credential_failure` markers ("token refresh failed", "invalid_grant", "401 Unauthorized", "Please log in again", etc.); non-credential failures (`429 rate limit`, `500 internal server error`) return false |

### Algorithm summary for Sprint 3 implementer

The Go port should be a single `func PickNextFallbackRoute(routes []ModelRoute, current ...) (int, bool)` returning `(index, ok)`. The filter stage is a single pass; the score stage can fold into the same pass by emitting `(tier, oauthPenalty, index)` tuples and taking the min. The test cases above translate 1:1 into Go table-driven tests under `test/llm/failover/fallback_pick_test.go`.