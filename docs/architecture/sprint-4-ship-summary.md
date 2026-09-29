# Sprint 4 ship summary — AnthropicProvider + shared Anthropic protocol

Sprint 4 added a real AnthropicProvider and refactored MiniMaxProvider
onto the existing-but-unused shared Anthropic protocol package.
Sprint 4 ships in 5 source commits + 1 audit-driven closeout commit
(all local, autonomous per overlay §2):

| Commit | SHA | Title |
|---|---|---|
| A | `e6fde99` | Refactor MiniMaxProvider onto shared Anthropic protocol |
| B | `8680079` | Add AnthropicProvider and refactor MiniMaxProvider onto shared event parser |
| C | `807a171` | Add AnthropicProvider e2e and distinct-provider FailoverCore e2e |
| D | `6159a41` | Sprint 4 alignment audit vs jcode |
| Closeout | `2901dac` | Sprint 4 audit closeout: max output tokens + beta header threading |

## What landed

### Shared Anthropic protocol package (Commit A + B)

`internal/llm/protocol/anthropic/` grew three new surfaces:

- `CompleteAnthropicSystemSplit` (Commit A) — midpoint split + cache
  breakpoint on prefix.
- `MapToAnthropicRequest` extensions (Commit A) — extracts system
  messages, translates tool-role messages to user-role with
  tool_result content blocks, drops unknown roles, threads
  ToolChoice and Thinking fields.
- `ConvertAnthropicEvent` (Commit B) — per-event parser returning
  the legacy `(*StreamChunk, done, error)` shape, the per-event
  counterpart to the existing `ParseAnthropicSSE`.

### AnthropicProvider (Commit B)

`internal/llm/providers/anthropic.go` (215 LOC) — talks to
`https://api.anthropic.com`. Four models:

- claude-opus-4-6 (200K, 128K out, thinking, manual budget)
- claude-opus-4-6[1m] (1M, 128K out, thinking, beta header
  context-1m-2025-08-07)
- claude-sonnet-4-6 (200K, 64K out, thinking, manual budget)
- claude-haiku-4-5 (200K, 64K out, no thinking)

### MiniMaxProvider slimdown (Commit B)

`MiniMaxProvider.ConvertRequest`, `ConvertResponse`, and
`CompleteSplit` now delegate to the shared package. The
~115-line duplicate wire-type + stream-event parser is gone; the
file is 280 LOC instead of 530.

### End-to-end tests (Commit C)

- `test/llm/anthropic_e2e_test.go` — 4 tests: full pipeline
  (request body shape + SSE response), haiku omits thinking, tool
  use round trip, beta header for 1m context (added in closeout).
- `test/llm/failover_distinct_provider_e2e_test.go` — 2 tests:
  MiniMax→Anthropic failover, both-distinct-providers-fail.

### Audit (Commit D)

`docs/architecture/sprint-4-alignment-audit.md` (233 LOC) —
file:line citations on both sides (jcode Rust vs awp Go), severity
tally, recommended actions per surface. 0 blockers, 4 should-fix,
5 nits, 1 extra, 2 missing. All 4 should-fix items have explicit
file:line citations and recommended actions.

### Audit-driven closeout (commit `2901dac`)

Per the audit's explicit Option 1 + Option 2 recommendations:

- Max output tokens bumped to live API limits (opus-4-6 → 128K,
  sonnet-4-6 → 64K, haiku-4-5 → 64K). Per-model table updated
  per jcode's `crates/jcode-provider-core/src/anthropic.rs:154-188`.
- BetaHeaders now threaded into the request path via
  `(*core).headersFor(req)` which merges `Headers()` with
  `BetaHeaders(req.Model)` joined into the `anthropic-beta`
  header. New e2e test verifies opus-4-6[1m] sends the header
  and opus-4-6 doesn't.

Remaining should-fix items (audit Option 3 was already done in
Commit C; these are explicitly Sprint 5+ scope per the audit):

- Adaptive thinking (`thinking: {type: adaptive}`) — modern
  Anthropic models prefer this over manual budget_tokens.
- OAuth attribution headers (Claude CLI user agent, Stainless
  arch/os/package/runtime, anthropic-dangerous-direct-browser-
  access). Only matters for OAuth subscription paths; API-key is
  fine.

## Test sweep status

- 17 packages green `-race -count=1 -timeout=180s`
- Provider coverage: 90.6% (`-coverpkg=./internal/llm/providers/...`)
- Protocol/anthropic coverage: 67.4% (the gap is in
  `ParseAnthropicSSE`'s full SSE framing and the request-body
  edge cases exercised by `MapToAnthropicRequest`)
- Failover coverage: 92.5% (Sprint 3, unchanged)

## Sprint 5 candidate surfaces

Six candidates ranked by what they unlock:

1. **Adaptive thinking + reasoning effort** — modern Anthropic
   models (`opus-4-6+`, `sonnet-4-6+`) prefer
   `thinking: {type: adaptive}` and `output_config.effort` over
   manual budget_tokens. Reference:
   `~/Project/jcode/crates/jcode-provider-core/src/anthropic.rs:215-274`
   (`AnthropicReasoningCaps`). ~150 LOC + tests.
2. **Context-mode classifier by family/version** — jcode's
   `anthropic_context_mode(model)` parser
   (`crates/jcode-provider-core/src/anthropic.rs:72-107`) is the
   canonical long-context classifier. awp's table is brittle for
   new generations. ~100 LOC + table migration.
3. **OAuth subscription path on AnthropicProvider** — Claude
   Code OAuth beta headers, credential caching, attribution
   headers (Stainless arch/os/package/runtime). Reference:
   `~/Project/jcode/crates/jcode-base/src/provider/anthropic.rs:38-69`.
   ~300 LOC + tests; depends on whether the awp runtime decides
   to support subscription auth.
4. **Extend the shared `anthropicMap` to all 12 jcode
   catalogue models** — current Sprint 4 ships 4, jcode ships
   12. Add opus-4-5, sonnet-4-5, opus-5, fable-5, etc.
5. **Replace the legacy per-event `Provider.ConvertResponse`
   interface with the sealed `StreamEvent` channel shape** —
   `protocol/anthropic.ParseAnthropicSSE` already returns
   `<-chan StreamEvent`; the `Provider` interface still wants
   per-event callbacks for backward compatibility with
   `llm.core.StreamChat`. A sealed event type is Sprint 5+ but
   not strictly urgent.
6. **OpenAI + other vendors** — the second vendor after
   Anthropic is the natural next refactor. OpenAI is the
   biggest ecosystem. ~400 LOC + protocol package +
   FailoverCore e2e tests mirroring Sprint 4's
   distinct-provider pattern.

Recommended: **start Sprint 5 with candidates 1 + 2** (single
sprint, both touch `AnthropicProvider` + the shared module,
natural pairing). Defer 3 until we know whether subscription
auth is needed. 5 + 6 can be parallel work if the user wants.

## Push status

Local-only. All 6 commits are on `ws-llm-anthropic`, not yet
merged to `main`. Per overlay §2, `git push origin main` requires
explicit "yes" confirmation. Local commits, branch creation, and
merge to `main` are free.

Local main is currently 17 commits ahead of origin (post-Sprint 2
closeout). After merging `ws-llm-anthropic` to main it will be
23 commits ahead.