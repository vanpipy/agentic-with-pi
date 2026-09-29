# Sprint 5 — Anthropic capability classifiers + adaptive thinking

Sprint 5 replaces the legacy hardcoded Anthropic model table with a family/version classifier package that derives `context_mode`, `reasoning_caps`, `max_output_tokens`, `beta_headers`, and the available reasoning-effort ladder from the model id. Adaptive thinking (`thinking: {type: adaptive}`) replaces the manual-budget envelope for every Claude generation that supports it; `output_config: {effort}` replaces per-request effort configuration where the model accepts it.

The wire shape is now driven from a single source of truth that the runtime, the TUI effort cycler, and the future model catalogue will all share — no drift between layers, no spec-table thrash every time Anthropic ships a new family.

## What's new

### Classifiers (`internal/llm/protocol/anthropic/anthropic_caps.go`)

A single package that answers five questions about a Claude id:

| Function | Question | Reference |
|---|---|---|
| `ContextMode(model)` | Standard / OptIn1M / Native1M? | jcode `anthropic.rs:62-107` |
| `ReasoningCaps(model)` | Full / NoXhigh / ManualWithEffort / ManualOnly / None? | jcode `anthropic.rs:336-381` |
| `MaxOutputTokens(model)` | 128K / 64K / 32K? | jcode `anthropic.rs:169-204` |
| `AvailableReasoningEfforts(model)` | The effort ladder filtered by caps | (derived) |
| `BetaHeaders(model)` | The `context-1m-2025-08-07` beta header when `[1m]` suffix present | jcode `anthropic.rs:111-113` |
| `Strip1mSuffix(model)` | The bare API id | jcode `model_id.rs:43-50` |

The classifiers normalize the model id (`[1m]` suffix stripped, lowercase, dots → dashes, `-YYYYMMDD` date suffix stripped) before parsing. They classify opus 3.5–5.5+, sonnet 3.7–5, haiku 4.5–5, fable 5–5.1, mythos, and any unknown version-5+ family by optimistic default.

### Wire shape (`internal/llm/protocol/anthropic/anthropic_request.go`)

`AnthropicThinking.Type` is now "enabled" (legacy budget envelope) **or** "adaptive" (no `budget_tokens`, model decides). `AnthropicOutputConfig{Effort}` is the per-request effort block; the wire shape omits it when the model doesn't support it or the caller requested "none".

### Provider (`internal/llm/providers/anthropic.go`)

`ConvertRequest` now drives the wire shape from `anthropic.ReasoningCaps(req.Model)` instead of the hardcoded 4-entry spec table. New methods:

- `DefaultReasoningEffort(model)` — opus-5-5 → "medium", opus-5 → "low", opus with xhigh → "xhigh", opus without → "high", fable-5 → "high", else empty. Empty means "no forced default".
- `manualThinkingBudget(effort, maxTokens)` — bucket-based budget for manual-thinking generations with maxTokens-1 clamp and 1024-token floor.

`AvailableReasoningEfforts`, `BetaHeaders`, `MaxOutputTokens` delegate to the classifier for unknown generations; the spec table is the source of truth only for known models.

`[1m]` suffix is stripped from the wire model field — the API expects the bare id and infers 1M-mode from the `anthropic-beta` header that `core.headersFor` adds via `provider.BetaHeaders`.

### Capability surface (`internal/llm/capability.go`)

`ModelCapabilities` now exposes `OutputEffort`, `AdaptiveThinking`, `ManualThinking` so callers can introspect the cap subset without re-parsing the classifier.

### Wire-in (`internal/llm/types.go`)

`ChatRequest.ReasoningEffort` is the new caller-facing field. Empty (the common case) means "use provider default". Non-empty values pass through to the wire as `output_config.effort` for adaptive-thinking models and as the budget bucket for manual-thinking generations. "none" suppresses `output_config` but keeps the adaptive envelope.

## Migration impact

- **Internal callers**: providers now delegate capability lookups to the classifier. The `modelSpec` table is the source of truth only for known models' physical limits (context window, max output).
- **External callers**: zero. `ChatRequest` gets one new additive field with `omitempty`.
- **Wire shape change**: opus-4-6, opus-4-7, opus-5, sonnet-5, fable-5, mythos all switch from `thinking: {type: enabled, budget_tokens: N}` to `thinking: {type: adaptive}` + `output_config: {effort: "..."}`. opus-4-5 keeps the legacy envelope. Opus-3.7-sonnet keeps the legacy envelope with no `output_config`. Haiku-4.5 sends no thinking block.
- **API surface**: callers that pre-set `MaxTokens=4096` on opus-4-6 will now get `MaxTokens=128000` (default = spec table value). This is intentional — agentic turns on opus-4-6 were silently truncating mid-tool-call.

## Audit

`docs/architecture/sprint-5-audit.md` maps every Sprint 5 surface against the jcode reference. Verdict: faithful translation with one intentional deviation (the missing `block_binding` / `display` controls on adaptive thinking, deferred to opus-5 series). Two initial audit claims were withdrawn after spot-check (largeOutputPrefixes is identical; mythos classification is identical). Three nits recorded for Sprint 6+ work.

## Deferred to closeout

The audit recommends a single follow-up commit closeout:

1. **`ThinkingAlwaysOn(model) bool` helper** — jcode's `anthropic.rs:147-159`. Needed by self-heal. Also pins `claude-mythos-5` `ContextMode` / `ReasoningCaps` test coverage.
2. **Self-heal retry `RecoverRequest`** — detect a 400 with `invalid_request_error` + `thinking`/`effort`/`output_config` + `not supported`/`does not support`, drop the offending reasoning fields, retry once. Wires at the `core` level (generic) with `AnthropicProvider` implementing recovery.

The classifier covers every known Claude generation; self-heal is defense for the unknown-future-generation transition window. Without self-heal, an unrecognized reasoning capability surfaces as a 400 instead of a graceful fallback.

## Test surface

| Test | What it pins |
|---|---|
| `TestAnthropicContextMode` (13 cases) | All known families + catch-all + dotted + `[1m]` + `-YYYYMMDD` + mixed-case + non-Claude |
| `TestAnthropicReasoningCaps` | Opus 3.5–5.5+, Sonnet 3.7–5, Haiku 4.5–5, Fable, Mythos, Nova, unknown |
| `TestAnthropicMaxOutputTokens` | 14 LARGE_OUTPUT_PREFIXES + 2 haiku 64K + 32K fallback + dotted forms |
| `TestAnthropicBetaHeaders` | suffix-only beta emission; no beta for Native1M / Standard models |
| `TestAnthropicStrip1mSuffix` | id preservation across all input forms |
| `TestAnthropicParseClaudeFamilyVersion` | version-last + version-first + dotted + date suffix |
| `TestAnthropicOptimisticDefaultRule` | version-5+ defaults to Full |
| `TestAnthropicNeverUndercuts32k` | legacy 32K regression guard |
| `TestAnthropicProviderConvertRequest{Adaptive,Manual,Floor,…}` (10 cases) | All wire shapes for all cap buckets |
| `TestAnthropicProviderDefaultReasoningEffort` | opus-5-5/opus-5/opus-xhigh/opus-no-xhigh/fable-5/empty ladder |
| `TestAnthropicProviderMaxOutputTokens` / `ContextWindow` | Spec-table + classifier fallback |
| `TestEndToEndAnthropicProvider{DrivingRealHTTPRest, HaikuOmitsThinking, ClassifierDispatch_ManualThinking, ClassifierDispatch_XHighDefault, OneMSuffixStripAndBeta, BetaHeaderForOneMContext, ToolUseRoundTrip}` | 7 e2e httptest pinpoints covering all four caps buckets |

**Coverage**: 17/17 packages green `-race -count=1 -timeout=180s`. `internal/llm/providers/` reaches 93.4% statement coverage. `internal/llm/protocol/anthropic/` reaches 90–100% per function on the new classifier code.

## Files touched

```
A  docs/architecture/sprint-5-scout.md         (299 LOC, scout)
A  docs/architecture/sprint-5-audit.md         (117 LOC, audit)
A  internal/llm/protocol/anthropic/anthropic_caps.go        (361 LOC, classifiers)
A  test/llm/protocol/anthropic_caps_test.go                 (332 LOC, classifier tests)
M  internal/llm/capability.go                 (+3 fields: OutputEffort, AdaptiveThinking, ManualThinking)
M  internal/llm/protocol/anthropic/anthropic_request.go     (+AnthropicOutputConfig wire type, adaptive envelope)
M  internal/llm/providers/anthropic.go        (ConvertRequest rewrite, classifier delegations, manualThinkingBudget, DefaultReasoningEffort)
M  internal/llm/types.go                       (+ChatRequest.ReasoningEffort)
M  test/llm/anthropic_e2e_test.go              (3 new e2e: Manual, XHigh, 1m+beta)
M  test/llm/providers/anthropic_test.go        (10 new tests, 3 updated for adaptive envelope)
```

## Commits

```
6d813a3  docs: Sprint 5 scout — adaptive thinking + Anthropic context-mode classifier
a932534  feat(llm): add Anthropic capability classifiers as single source of truth
90d62f7  llm(anthropic): adaptive thinking + output_config effort + 1m strip
476f42d  test(llm): classifier-dispatch e2e for adaptive + manual + 1m beta
4bc20a1  docs: Sprint 5 alignment audit vs jcode
```

Five commits as planned. Branch `ws-llm-thinking` ready to merge.
