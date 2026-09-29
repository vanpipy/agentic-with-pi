# Sprint 3 alignment audit — failover primitive + FailoverCore

Post-phase audit per `~/.jcode/prompt-overlay.md §1` against the
reference implementation at `~/Project/jcode`.

## Phase surface

| jcode file | LOC | awp file(s) | LOC | status |
|---|---|---|---|---|
| `crates/jcode-provider-core/src/failover.rs` | 182 | `internal/llm/failover/failover.go` | 219 | ✓ ported |
| `crates/jcode-provider-core/src/fallback_pick.rs` (impl) | 197 | `internal/llm/failover/pick.go` | 126 | ✓ ported |
| `crates/jcode-provider-core/src/fallback_pick.rs` (tests) | 154 | `test/llm/failover/pick_test.go` | 258 | ✓ ported + extended |
| `crates/jcode-provider-core/src/lib.rs:676-684` (ModelRoute) | 9 | `internal/llm/failover/route.go:7-13` | 7 | ✓ ported (subset) |
| `crates/jcode-provider-core/src/lib.rs:857-918` (ModelRouteApiMethod) | 62 | `internal/llm/failover/route.go:22-91` | 70 | ✓ ported (subset) |
| `crates/jcode-provider-core/src/lib.rs:985-1010` (provider label match) | 26 | `internal/llm/failover/route.go:94-133` | 40 | ✓ ported |
| `crates/jcode-provider-core/src/fallback_pick.rs:52-74` (credential classifier) | 23 | `internal/llm/failover/route.go:135-167` | 33 | ✓ ported |
| jcode cross-provider failover loop (provider-side) | (no canonical file; embedded in provider trait) | `internal/llm/failover_core.go` | 207 | ✓ new addition |

## Audit findings

### Blockers
None.

### Should-fix
None.

### Nits / intentional divergences

1. **`ProviderFailoverPrompt.EstimatedInputTokens/Chars` always 0 in Commit C**
   jcode's `to_error_message` formats these values to inform the user how
   much re-send cost a switch will incur. In awp's `notifySwitch` we
   hardcode 0 because `FailoverCore` does not see token counts. Resolved
   by: design — FailoverCore's job is the retry+route loop, not the
   cost surface. The TUI consumer of the prompt can compute an
   estimate from `ChatRequest.Messages` if it cares. (action: accept-with-note)

2. **`FailoverConfig.OnDecision` fires *before* the prompt-emitting error event.**
   The hook fires first, then `notifySwitch` emits
   `LegacyStreamEvent{Err: &failoverFailoverError{...}}`. A consumer
   reading the channel will see the synthetic error but will miss the
   hook unless it wires both. Resolved by: hook is optional. Action:
   document in Commit C follow-up.

3. **`tryRoute` drains the inner RetryCore channel before forwarding.**
   jcode's provider-trait failover hooks at the request boundary
   (each `StreamChat` call is a fresh attempt). awp's `FailoverCore`
   needs to drain so it knows whether the route eventually succeeded.
   This means a mid-stream Rollback emitted by `RetryCore.forwardWithMidRetry`
   is collapsed inside `tryRoute`; the outer FailoverCore never sees it.
   That is acceptable because `MaxRoutes == 1` (the per-route retry
   budget) handles mid-stream retries; FailoverCore only fires when
   retries are exhausted. (action: accept-with-note)

4. **`APIMethod` enum is flat; `OpenAiCompatible { profile_id: Option<String> }`
   is flattened.**
   jcode's `ModelRouteApiMethod::OpenAiCompatible` carries an optional
   profile id; awp's `APIMethodOpenAICompatible` is a single variant and
   the `profile_id` is dropped (it's a string the picker never consults).
   The picker only reads `IsOAuth()`, so the simplification is safe.
   (action: accept-with-note)

5. **`ProviderLabelsMatch` aliases are a subset of jcode's.**
   jcode recognises claude/anthropic, openai, gemini/google,
   antigravity, copilot/copilotcode/githubcopilot, plus a small set of
   headless-provider aliases for `bedrock` / `vertex` /
   `azure-openai` (see `lib.rs:1010+`). awp's port is limited to the
   five canonical aliases above. Adding more is trivial but the
   providers are not in awp yet. (action: re-scope to a later sprint
   when real providers land)

6. **`MaxRoutes` defaults to "all routes" (no upper cap).**
   jcode's picker is unbounded; the loop terminates when
   `pick_next_fallback_route` returns `None`. awp's `FailoverCore`
   does the same but exposes `MaxRoutes` so a paranoid caller can
   bail early. Defaults to len(routes) so behavior is identical.
   (action: accept-with-note)

## Verification matrix

| Surface | jcode test count | awp test count | Coverage |
|---|---|---|---|
| `failover.rs` (classify, decision, prompt round-trip) | 0 (pure code) | 18 | 95.9% |
| `fallback_pick.rs` impl | 11 (in-file) | 11 + 5 (label/parse/IsOAuth) = 16 | 92.5% |
| `failover_core` (new) | n/a | 2 e2e + RetryCore reuse | 60-100% per-fn |
| `RetryCore` (reused, not changed) | (Sprint 2) | (Sprint 2) | 85.5% |

## Conclusion

Sprint 3 ships a faithful translation of the jcode failover surfaces
into awp, with one structural addition (FailoverCore) that the jcode
reference embeds in its provider trait. Open follow-ups:

- Real second provider in `providers/` (Sprint 4+)
- TUI one-key switch consumer for `ProviderFailoverPrompt` (Sprint 4+)
- Extension of `ProviderLabelsMatch` aliases as more providers land
- Optional: drop `tryRoute` draining once `RetryCore` exposes a
  "stream eventually failed" channel via a sealed error type
