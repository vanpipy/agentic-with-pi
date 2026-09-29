# Sprint 5 scout — adaptive thinking + Anthropic context-mode classifier

## Goal

Extend `AnthropicProvider` so it speaks the modern Anthropic Messages
reasoning surface (`thinking: {type: adaptive}` + `output_config:
{effort}`) instead of the legacy `thinking: {type: enabled, budget_tokens}`
envelope currently emitted at `internal/llm/providers/anthropic.go:189-198`.
Replace the hardcoded per-model `anthropicModelSpecs` table with two
family/version classifiers that are the single source of truth for
capability flags:

- `anthropic_context_mode(model) -> Standard | OptIn1M | Native1M`
  (parallels jcode's
  `crates/jcode-provider-core/src/anthropic.rs:72-107`).
- `anthropic_reasoning_caps(model) -> AnthropicReasoningCaps{output_effort,
  adaptive_thinking, manual_thinking, xhigh_effort, max_effort}` (parallels
  `crates/jcode-provider-core/src/anthropic.rs:336-381`).

Both classifiers live in `internal/llm/protocol/anthropic/` (the package
that owns the Anthropic wire surface) and are consumed by every caller
that needs to know "what can this model do": the runtime
(`AnthropicProvider.ConvertRequest`), the catalogue
(`AnthropicProvider.ModelCapabilities`), and any future TUI effort cycler.
The classifier is the single source of truth shared by these three
callers, exactly as jcode shares
`AnthropicReasoningCaps` between the runtime and TUI effort cycler.

This delivers Sprint 5 candidates #1 and #2 from
`docs/architecture/sprint-4-ship-summary.md` (the recommended Sprint 5
start).

## Why now

- The current `AnthropicProvider.ConvertRequest` emits
  `thinking: {type: "enabled", budget_tokens: 8192}` for every thinking
  model. The Anthropic API rejects this for every model that has moved to
  adaptive thinking (Opus 4.7+, Sonnet 5, Fable 5, future generations).
  The current code silently degrades to "no thinking" on those models
  because the runtime has no self-heal path. This sprint adds adaptive
  thinking + the self-heal retry (jcode has both at
  `crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:95-115`).
- The hardcoded `anthropicModelSpecs` map (4 entries) cannot keep up with
  the release-cadence Anthropic ships new generations. A new
  `claude-opus-5-5` or `claude-haiku-5` ships, the model is unknown, the
  fallback is "200K context, no thinking" — silent under-reporting of the
  context meter and shrinking of compaction budgets ~5x with no
  diagnostic. The classifier fixes this for every future model without
  anyone touching the code.
- Sprint 4 added `BetaHeaders(model)` for `claude-opus-4-6[1m]` (closeout
  commit `2901dac`). The classifier will let `BetaHeaders` return the
  right value for `claude-opus-5`, `claude-sonnet-5[1m]`, etc. without
  table maintenance.
- Sprint 3's `FailoverCore` and Sprint 4's `BetaHeaders` thread are
  settled; the LLM V2 wire surface is stable enough that adding
  reasoning-shape changes does not destabilise either.

## Reference surface

| jcode file | LOC | awp file (target) | LOC |
|---|---|---|---|
| `crates/jcode-provider-core/src/anthropic.rs:62-145` (`anthropic_context_mode` + `_is_verified`) | 84 | `internal/llm/protocol/anthropic/anthropic_caps.go` (new, contains `ContextMode(model) AnthropicContextMode`) | ~80 |
| `crates/jcode-provider-core/src/anthropic.rs:225-381` (`AnthropicReasoningCaps` + `anthropic_reasoning_caps`) | 157 | same file, `ReasoningCaps(model) AnthropicReasoningCaps` | ~120 |
| `crates/jcode-provider-core/src/anthropic.rs:161-204` (`anthropic_max_output_tokens`) | 44 | already ported (Sprint 4 closeout) — replace per-model table lookup | n/a |
| `crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:1-296` (`adaptive_thinking`, `build_reasoning_request_parts_for_budget`, `recover_rejected_reasoning`) | 296 | `internal/llm/providers/anthropic.go` (rewrite `ConvertRequest` + new `reshape` helper) + `internal/llm/protocol/anthropic/anthropic_request.go` (add `AnthropicOutputConfig` wire type) | ~180 |
| `crates/jcode-provider-anthropic-runtime/src/lib.rs:780-922` (`default_reasoning_effort_for_model`, `build_reasoning_request_parts_with_effort`, `manual_thinking_budget`) | 142 | `internal/llm/providers/anthropic.go` (`DefaultReasoningEffort` method + ladder) | ~80 |

## Commit plan

### Commit A — `AnthropicReasoningCaps` + context-mode classifier

New file `internal/llm/protocol/anthropic/anthropic_caps.go`
(~200 LOC). Contains:

- `AnthropicReasoningCaps` struct: `OutputEffort`,
  `AdaptiveThinking`, `ManualThinking`, `XHighEffort`, `MaxEffort` bools.
- `ContextMode(model string) AnthropicContextMode`:
  family/version parser that resolves to
  `ContextStandard | ContextOptIn1M | ContextNative1M`. Mirrors
  jcode's `anthropic_context_mode` at
  `crates/jcode-provider-core/src/anthropic.rs:62-107`. Handles dotted
  (`claude-opus-4.6`) and dashed (`claude-opus-4-6`) forms plus
  `[1m]`/`-YYYYMMDD` suffix stripping. Optimistic default for unknown
  generations (5.x → Native1M; <5.x → Standard).
- `ReasoningCaps(model string) AnthropicReasoningCaps`: family/version
  parser that resolves to one of four caps presets (`Full`,
  `EffortNoXhigh`, `ManualWithEffort`, `ManualOnly`, `None`). Mirrors
  jcode's `anthropic_reasoning_caps` at
  `crates/jcode-provider-core/src/anthropic.rs:336-381`.
- `MaxOutputTokens(model string) int`: lifted from
  `anthropic.go:120-125`'s default branch into the package. Reuses the
  `LARGE_OUTPUT_PREFIXES` table from jcode
  `anthropic.rs:174-189` verbatim. Already aligned to published limits
  (Sprint 4 closeout `2901dac`); the move is structural, not behavioural.
- `Strip1mSuffix(model string) string` — already exists as part of the
  dotted-version handling.
- `BetaHeaders(model string) []string`: returns
  `["context-1m-2025-08-07"]` when
  `ContextMode(model) == ContextOptIn1M || ContextNative1M`,
  nil otherwise. Replaces the per-model `betaHeaders` field on
  `anthropicModelSpecs`.

Adds `internal/llm/protocol/anthropic/anthropic_caps_test.go` (~200 LOC)
— table-driven tests covering:

- context mode for every Claude generation (opus-3-5, sonnet-4-5,
  opus-4-6, opus-4-7, opus-5, sonnet-5, haiku-4-5, haiku-5, fable-5-1,
  mythos, dotted variants, `[1m]`-suffixed variants, dated variants).
- reasoning caps presets for known generations.
- optimistic-default rule (claude-sonnet-6, claude-nova-5 → Full).
- MaxOutputTokens matches the published limits table.
- suffix stripping normalises `[1m]` and `-YYYYMMDD`.

No behaviour change yet for `AnthropicProvider` — Commit A is purely
additive (new package, no caller wired up). All existing
`AnthropicProvider` tests stay green; per-sprint policy: zero regressions,
17+ packages `-race` green.

### Commit B — `AnthropicProvider.ConvertRequest` emits adaptive thinking

Rewrites `internal/llm/providers/anthropic.go:189-198` to emit:

- `thinking: {type: "adaptive"}` when
  `caps.AdaptiveThinking && (effort.IsSome() || alwaysOn || showThinking)`.
- `output_config: {effort}` when `caps.OutputEffort && effort.IsSome()`.
- Falls back to manual `thinking: {type: "enabled", budget_tokens}` for
  `caps.ManualThinking` generations.
- Strips `[1m]` from `request.Model` before sending (jcode does this in
  `RetrySettings::reshape` at
  `crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:65-89`).
- Adds `DefaultReasoningEffort(model) string`: `claude-opus-5-5` →
  `"medium"`, `claude-opus-5` → `"low"`, `claude-opus` →
  `"xhigh"` or `"high"` depending on `caps.XHighEffort`,
  `claude-fable-5` → `"high"`, others → `""` (caller default).
  Mirrors jcode at
  `crates/jcode-provider-anthropic-runtime/src/lib.rs:788-807`.
- Adds `MaxTokensFor(model) int` returning the
  `anthropic_caps.MaxOutputTokens` value instead of the per-model table.
- Implements `recover_rejected_reasoning` self-heal: when
  `protocol.Classify` returns a `400`-classed error mentioning
  `output_config` / `thinking` / `effort`, drop the unsupported field
  once and retry. Mirrors jcode at
  `crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:95-115`.

Wires `internal/llm/protocol/anthropic.BuildAnthropicRequest` to accept
the new `AnthropicOutputConfig{Effort string}` wire type
(`output_config: {effort: "..."}`).

Adds `ChatRequest.ReasoningEffort string` field
(`internal/llm/types.go:31-40`) so callers can pass the requested
effort down. Defaults to empty (caller passes `""` → use
`DefaultReasoningEffort`). Empty means "model default", never disables
thinking on always-on models.

Extends `internal/llm/protocol/anthropic/anthropic_request.go` with
`AnthropicOutputConfig` wire type. Extends `AnthropicThinking` to support
both `Type: "adaptive"` (no `BudgetTokens`) and
`Type: "enabled"` (with `BudgetTokens`).

Updates `internal/llm/providers/anthropic.go::AvailableReasoningEfforts`
to return the filtered ladder (`["none","low","medium","high","xhigh","max"]`
clipped to `caps.XHighEffort`/`caps.MaxEffort`).

Updates `internal/llm/providers/minimax.go` to mirror the same wire
changes (the shared `BuildAnthropicRequest` makes this trivial:
`MiniMaxProvider.ConvertRequest` delegates).

Tests:
- `test/llm/providers/anthropic_test.go`: extend with
  `TestAnthropicProviderConvertRequestAdaptiveThinking`,
  `TestAnthropicProviderConvertRequestOutputConfigEffort`,
  `TestAnthropicProviderConvertRequestManualThinkingFallback`,
  `TestAnthropicProviderConvertRequestStrips1mSuffix`,
  `TestAnthropicProviderDefaultReasoningEffort`,
  `TestAnthropicProviderAvailableReasoningEffortsFiltersLadder`.
- `test/llm/providers/anthropic_test.go`: extend with
  `TestAnthropicProviderContextWindowViaClassifier` (unknown model
  resolves to expected ContextMode via classifier).

### Commit C — end-to-end tests

`test/llm/anthropic_thinking_e2e_test.go` (~200 LOC):

- httptest server returning a valid Anthropic SSE stream for a
  request that contains `thinking: {type: "adaptive"}` and
  `output_config: {effort: "high"}`. Asserts the wire body contains
  both fields and that the response is parsed correctly.
- httptest server returning a 400 with the message
  `{"type":"error","error":{"type":"invalid_request_error","message":"..."}}`
  when `output_config` is set. Verifies the self-heal retry once and
  succeeds when the retry omits `output_config`.
- httptest server that records two requests and asserts:
  - first request has `thinking.type = "adaptive"` and
    `output_config.effort = "high"`,
  - second (retry) has neither,
  - the assembled StreamChunk carries the streamed text.

`test/llm/anthropic_classifier_e2e_test.go` (~150 LOC):

- httptest server returning a minimal SSE stream for each known Claude
  generation; asserts `ModelCapabilities(ctx).ContextMode` matches
  expected per family/version.
- httptest server returning a 400 for `claude-opus-5` with
  `output_config.effort = "xhigh"`. Verifies the provider downgrades
  to `"high"` automatically (via caps filter).

### Commit D — alignment audit (overlay §1)

`docs/architecture/sprint-5-alignment-audit.md`. Compare the new
classifiers and request-building against jcode:

| Surface | jcode file | awp file (target) |
|---|---|---|
| `anthropic_context_mode` | `crates/jcode-provider-core/src/anthropic.rs:62-107` | `internal/llm/protocol/anthropic/anthropic_caps.go` |
| `anthropic_context_mode_is_verified` | same file:131-145 | (deferred — Sprint 6+ when catalog data lands) |
| `anthropic_reasoning_caps` | `crates/jcode-provider-core/src/anthropic.rs:336-381` | same file |
| `adaptive_thinking` | `crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:8-17` | `internal/llm/providers/anthropic.go` |
| `build_reasoning_request_parts_for_budget` | `crates/jcode-provider-anthropic-runtime/src/lib.rs:924-975` | same file |
| `recover_rejected_reasoning` | `crates/jcode-provider-anthropic-runtime/src/reasoning_request.rs:95-115` | same file |
| `default_reasoning_effort_for_model` | `crates/jcode-provider-anthropic-runtime/src/lib.rs:788-807` | same file |
| `manual_thinking_budget` | `crates/jcode-provider-anthropic-runtime/src/lib.rs:868-879` | same file (only used when manual_thinking is the model's path) |
| `BetaHeaders(model)` returns context-1m | `crates/jcode-provider-core/src/anthropic.rs:217-223` + `crates/jcode-base/src/provider/anthropic.rs:46` | `internal/llm/protocol/anthropic/anthropic_caps.go` |
| OAuth attribution headers | `crates/jcode-base/src/provider/anthropic.rs:59-75` | (deferred — Sprint 6+) |

Document intentional divergences (MiniMax doesn't support adaptive
thinking, so the wire shape stays `thinking: {type: enabled}` for it).
Audit severity tally follows overlay §1 format.

### Commit E — ship summary

`docs/architecture/sprint-5-ship-summary.md`. Capture commits,
verification matrix, open questions, candidate Sprint 6 surfaces.

## Verification matrix

| Check | Expected |
|---|---|
| `gofmt -l .` | clean |
| `go vet ./...` | clean |
| `go test -race -count=1 ./...` | 17+ packages green |
| `internal/llm/protocol/anthropic` coverage | ≥90% (Commit A adds ~200 LOC of test) |
| `internal/llm/providers/anthropic.go` coverage | ≥85% (Commit B rewrites the request builder) |
| AnthropicProvider e2e thinking tests | pass |
| AnthropicProvider self-heal retry test | passes (records two requests, asserts shape of each) |
| AnthropicProvider classifier e2e tests | pass |
| Sprint 4 e2e tests (`anthropic_e2e_test.go`) | unchanged — still green |

## Scope discipline

Sprint 5 stays inside the LLM V2 wire surface and the Anthropic
classifiers. It explicitly does **not**:

- **Wire AnthropicProvider into `cmd/awp/main.go loadAgent()`** —
  deferred to a follow-up sprint. The default route remains
  `MiniMaxProvider`; `AnthropicProvider` is exercisable through tests
  and via direct provider wiring.
- **OAuth attribution headers** (12 headers from jcode's
  `crates/jcode-base/src/provider/anthropic.rs:59-75`) — deferred to
  Sprint 6+ (requires API-key-vs-OAuth branching in `AnthropicProvider`,
  which is a structural change).
- **TUI effort cycler** that consumes `AnthropicReasoningCaps` —
  deferred to the TUI sprint. Sprint 5 makes the caps available; the
  TUI integration is its own work.
- **`anthropic_context_mode_is_verified`** — deferred. Without a live
  catalog (Sprint 6+), the verified/unverified distinction has no
  consumer.
- **Replace legacy per-event `ConvertResponse` with sealed
  `StreamEvent` channel** — Sprint 6+ candidate.
- **OpenAI + other vendors** — Sprint 7+ candidate.
- **Models catalogue expansion from 4 → 12 jcode models** — Sprint 6+
  candidate (some models like `claude-fable-5-1` would now be
  correctly classified but not added to the table).

## Open questions for user

1. **`ChatRequest.ReasoningEffort` field on the public API.** Sprint 5
   adds a new field to `internal/llm/types.go`. Should the field live
   on `ChatRequest` directly (one-shot per-request) or as a
   per-provider setting on `AnthropicProvider` (sticky across requests)?
   jcode uses both (per-request `effort` overrides per-provider
   `reasoning_effort` setting). Sprint 5 will do both — `ChatRequest`
   field takes precedence, falls back to provider's default.
2. **Self-heal retry scope.** The self-heal path currently lives in
   `AnthropicProvider`. Should it move into `internal/llm/retryafter.go`
   (alongside the existing retry logic) so every provider benefits, or
   stay per-provider? Sprint 5 keeps it per-provider; future sprint
   can lift it.
3. **MiniMax provider thinking shape.** `MiniMaxProvider` currently
   emits `thinking: {type: "enabled", budget_tokens: ...}`. With
   Sprint 5's wire-type changes, should `MiniMaxProvider` also be
   migrated to adaptive thinking? (jcode's MiniMax doesn't exist —
   awp has no equivalent to compare against.) Default: **no**, keep
   `MiniMaxProvider` as-is; the wire type is shared but the request
   builder is per-provider.
4. **Wire the classifier into the FailoverCore failover decision.**
   Should `FailoverCore` prefer providers whose model classifier says
   `Native1M` over `OptIn1M` for long-context requests? Currently
   `FailoverCore` only knows "primary vs fallback", not "context fit".
   Sprint 5 keeps this out of scope; flag for Sprint 6.