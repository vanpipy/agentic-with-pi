# Sprint 4 alignment audit — AnthropicProvider + shared Anthropic protocol

Reference: `~/Project/jcode` (Rust, `jcode-base` + `jcode-provider-core` +
`jcode-provider-anthropic-runtime`).
Subject: `internal/llm/providers/anthropic.go`, the shared
`internal/llm/protocol/anthropic` package after Sprint 4 Commits A–C.

Audit method: file:line citations on both sides, severity (blocker /
should-fix / nit), recommended action (amend / accept-with-note /
re-scope-phase).

## Provider struct shape

| Surface | jcode | awp |
|---|---|---|
| Struct + new() | `jcode-provider-anthropic-runtime/src/lib.rs:465` (`AnthropicProvider`, 20 fields incl. `client`, `model`, `credentials`, `oauth_session_id`, `direct_transport`, `profile_models`) | `internal/llm/providers/anthropic.go:10-19` (`AnthropicProvider{APIKey}`, 1 field) |
| New() | `jcode-provider-anthropic-runtime/src/lib.rs:598` (`pub fn new() -> Self`, sets up env-derived OAuth/API-key state, cached creds, profile model lists) | `internal/llm/providers/anthropic.go:14-18` (`NewAnthropicProvider(apiKey string)`) |
| Pin credential mode | `jcode-provider-anthropic-runtime/src/lib.rs:561` | n/a |
| Per-model effort / service tier / model-scoped quota fallback | `jcode-provider-anthropic-runtime/src/lib.rs:712-836` | n/a (Sprint 5+ scope) |
| Total provider LOC | 2660 | 215 |

Deviation type: extra (jcode has lots more state).
Severity: nit.
Recommended action: accept-with-note. awp Sprint 4 ships the
provider's wire-shape surface (Name / BaseURL / Path / Headers /
ConvertRequest / ConvertResponse / Models / *Caps / BetaHeaders /
CompleteSplit) — every method the Provider interface requires.
OAuth, profile pinning, quota fallback, model-scoped usage, and
the doctor's `pin_credential_mode_for_doctor` are explicitly Sprint
5+ per scout Q3 and the sprint-4 scout's open questions.

## Models list

| Surface | jcode | awp |
|---|---|---|
| Catalogue | `crates/jcode-base/src/provider/anthropic.rs:73-86` (`AVAILABLE_MODELS`, 12 entries: claude-opus-5, claude-fable-5, claude-opus-4-8, claude-opus-4-6, claude-opus-4-6[1m], claude-sonnet-5, claude-sonnet-4-6, claude-sonnet-4-6[1m], claude-haiku-4-5, claude-opus-4-5, claude-sonnet-4-5, claude-sonnet-4-20250514) | `internal/llm/providers/anthropic.go:48-78` (`anthropicModelSpecs`, 4 entries: opus-4-6, opus-4-6[1m], sonnet-4-6, haiku-4-5) |

Deviation type: missing.
Severity: nit.
Recommended action: accept-with-note. Per scout Q2, Sprint 4 ships
the 4 most common current-generation models. Pre-4-6 generations
(opus-4-5, sonnet-4-5) and the 5-generation are Sprint 5+ when the
runtime's OAuth path exists.

## Max output tokens

| Surface | jcode | awp |
|---|---|---|
| Per-model table | `crates/jcode-provider-core/src/anthropic.rs:154-188` (`anthropic_max_output_tokens`, prefix-matched: opus-5/4.8/4.7/4.6/sonnet-5/sonnet-4.6/fable/mythos → 128_000; haiku-4.5 → 64_000; older → 32_768) | `internal/llm/providers/anthropic.go:51-79` (opus-4-6 → 32_000, sonnet-4-6 → 16_000, haiku-4-5 → 8_192) |

Deviation type: semantic-difference (awp values are too low for
opus-4-6 and sonnet-4-6; live API supports 128K and 64K
respectively).
Severity: should-fix.
Recommended action: amend in Sprint 5 closeout (not blocking —
agent turns work today, but long agentic runs truncate early). See
audit Option 1 below.

## Context mode classification

| Surface | jcode | awp |
|---|---|---|
| Enum | `crates/jcode-provider-core/src/anthropic.rs:14-25` (`AnthropicContextMode { Native1M, OptIn1M, Standard }`) | `internal/llm/capability.go:5-11` (`AnthropicContextMode { ContextStandard, ContextOptIn1M, ContextNative1M }`) |
| Lookup | `crates/jcode-provider-core/src/anthropic.rs:72-107` (`anthropic_context_mode`, parses family + version, classified by generation) | n/a — uses per-model table lookup (modelSpec.contextMode field) |

Deviation type: shape-different (lookup-by-table vs
lookup-by-family-version-parser).
Severity: nit.
Recommended action: accept-with-note. jcode's parser is the
canonical long-context classifier (it's also a documented source of
truth for the Anthropic runtime and the TUI effort cycler, jcode
comment: "the single source of truth shared by the Anthropic
runtime (request building, `set_reasoning_effort` validation) and
the TUI effort cycler, so new models cannot drift between the
two"). awp's table works today for the 4 shipped models but won't
auto-classify future generations; that's a Sprint 5+ port.

## Beta headers

| Surface | jcode | awp |
|---|---|---|
| OAuth beta header | `crates/jcode-provider-core/src/anthropic.rs:1-2` (string constant, full Claude Code subscription beta set + 1M variant with `context-1m-2025-08-07`) | `internal/llm/providers/anthropic.go:77` (returns `[]string{"context-1m-2025-08-07"}` for opus-4-6[1m] via Provider.BetaHeaders) |
| API key path | (not in scope of jcode's anthropic.rs; jcode runtime always uses OAuth) | n/a — the request path doesn't thread BetaHeaders yet |

Deviation type: missing (BetaHeaders not threaded into the request).
Severity: should-fix.
Recommended action: re-scope-phase. The 1M-context model
(`claude-opus-4-6[1m]`) currently advertises the beta header but
the request body doesn't carry it. This is a follow-up wiring
issue, not a Sprint 4 scope hole — BetaHeaders is on the Provider
interface specifically for Sprint 5 to thread it through
`protocol/anthropic.BuildAnthropicRequest`. Documented as
remaining work.

## Reasoning / thinking shape

| Surface | jcode | awp |
|---|---|---|
| Caps | `crates/jcode-provider-core/src/anthropic.rs:215-274` (`AnthropicReasoningCaps { output_effort, adaptive_thinking, manual_thinking, xhigh_effort, max_effort }`, classified per-model via `anthropic_reasoning_caps`) | `internal/llm/capability.go:13-27` (`ModelCapabilities { SupportsThinking, ReasoningEfforts, ServiceTiers, BetaHeaders }`) |
| Adaptive thinking | `crates/jcode-provider-core/src/anthropic.rs:219-220` (`adaptive_thinking` flag, sent as `thinking: {type: adaptive}` on modern models) | n/a — always uses `thinking: {type: enabled, budget_tokens}` |
| Manual thinking budget | `crates/jcode-provider-anthropic-runtime/src/lib.rs:841-924` (`manual_thinking_budget`, `build_reasoning_request_parts`, caps per model) | `internal/llm/providers/anthropic.go:170-182` and `internal/llm/providers/minimax.go:238-249` (each provider computes its own budget formula: `MaxTokens/2`, cap varies) |
| Reasoning effort surface | jcode runtime supports `output_config: {effort: low..xhigh/max}` and validates against per-model caps | n/a (`AvailableReasoningEfforts` returns nil) |

Deviation type: extra (jcode has more knobs).
Severity: should-fix.
Recommended action: re-scope-phase. Modern Anthropic models
(opus-4-6+, sonnet-4-6+) prefer `thinking: {type: adaptive}` over
manual budget_tokens, and `output_config.effort` is the supported
control plane. Sprint 4 ships manual budget_tokens because (a) the
shared `AnthropicRequest.Thinking` is the existing primitive and
(b) the 4 shipped models all support manual thinking. Adaptive
thinking + effort validation is Sprint 5 work; flagged for the
next sprint's audit.

## OAuth attribution headers

| Surface | jcode | awp |
|---|---|---|
| Full set | `crates/jcode-base/src/provider/anthropic.rs:38-69` (claude-cli/2.1.123 user agent, x-client-request-id, x-app, X-Claude-Code-Session-Id, X-Stainless-{Arch,Lang,OS,Package-Version,Retry-Count,Runtime,Runtime-Version,Timeout}, anthropic-dangerous-direct-browser-access=true) | n/a |

Deviation type: missing.
Severity: should-fix (only matters for OAuth subscription paths;
API-key path works without them).
Recommended action: re-scope-phase. Per scout Q3, OAuth attribution
is Sprint 5+ when the OAuth subscription flow is added to
AnthropicProvider. API-key calls (which is what `awp` exercises in
Commit C e2e) don't need these.

## Request-building shared surface

| Surface | jcode | awp |
|---|---|---|
| Module | `crates/jcode-provider-core/src/anthropic.rs` (611 LOC) — 14 free functions covering context mode, max output, reasoning caps, oauth beta, tool-name mapping, stainless arch/os | `internal/llm/protocol/anthropic/anthropic_request.go` (393 LOC after Sprint 4 Commit A) + `anthropic_sse.go` (455 LOC pre-existing) + `anthropic_event.go` (166 LOC new in Commit B) |
| Wire types | jcode uses serde-derive structs (one per wire shape, with `#[serde(rename = ...)]`) | `internal/llm/protocol/anthropic/anthropic_request.go:54-100` (`wireCacheControl`, `wireSystemBlock`, `wireTextBlock`, `wireImageBlock`, `wireToolUseBlock`, `wireToolResultBlock`, `wireThinkingBlock`, `wireMessage`, `wireTool`, `wireBody`, `wireToolChoice`, `wireThinking`) — same per-block decomposition |
| Map from generic request | jcode runtime's `format_messages` + `format_content_blocks` (`lib.rs:1101-...`) walk messages and emit blocks per type | `internal/llm/protocol/anthropic/anthropic_request.go:290-380` (`MapToAnthropicRequest`, returns `AnthropicRequest` with `Messages` / `System` / `Tools` / `ToolChoice` / `Thinking` populated) |
| Build request body | jcode runtime's `build_reasoning_request_parts` + serde serialise | `internal/llm/protocol/anthropic/anthropic_request.go:216-280` (`BuildAnthropicRequest`, serialises `AnthropicRequest` to wire JSON) |
| Tool name mapping (OAuth ↔ API) | `crates/jcode-provider-core/src/anthropic.rs:367-396` (`anthropic_map_tool_name_for_oauth`, `anthropic_map_tool_name_from_oauth`) | n/a (only matters for OAuth path) |
| System-prompt split | (jcode handles cache_control inline per call) | `internal/llm/protocol/anthropic/anthropic_request.go:382-...` (`CompleteAnthropicSystemSplit`, midpoint split + cache_control on prefix) |

Deviation type: shape-different (free-function table vs typed
struct + module call path). Same surface intent.
Severity: nit.
Recommended action: accept-with-note. jcode's helpers are spread
across two crates (`provider-core::anthropic` + `provider-base::anthropic`)
because of Rust's compile-time crate boundaries; awp's helpers
live in one package (`internal/llm/protocol/anthropic`). The
shared module absorbs all the Sprint 4 conversion logic, and the
two providers (`AnthropicProvider`, `MiniMaxProvider`) delegate.

## Event parser

| Surface | jcode | awp |
|---|---|---|
| Per-event parser | (lives in the runtime; not surfaced as a public function) | `internal/llm/protocol/anthropic/anthropic_event.go:20-150` (`ConvertAnthropicEvent`, returns `(*StreamChunk, done, error)`) |
| Stream parser | `crates/jcode-provider-anthropic-runtime/src/lib.rs:1972` (`async fn stream_response`, returns an `EventStream<AnthropicStreamEvent>`) | `internal/llm/protocol/anthropic/anthropic_sse.go:20` (`ParseAnthropicSSE`, returns `<-chan StreamEvent`) |
| Stream-event types | `crates/jcode-provider-anthropic-runtime/src/sse_types.rs:1-95` (95 LOC) | `internal/llm/protocol/anthropic/anthropic_sse.go:233-...` + `anthropic_event.go:153-...` (legacy + new types) |

Deviation type: shape-different (sealed stream channel vs per-
event callback in the Provider interface).
Severity: nit.
Recommended action: accept-with-note. The `Provider.ConvertResponse`
interface returns per-event `(*StreamChunk, done, error)` for
compatibility with `llm.core.StreamChat` — the SSE parser's stream
channel is for the higher-level `internal/agent-core` consumers.
Both are now in the shared package. Future Sprint 5+ work: replace
the legacy per-event interface with the stream channel shape.

## Failover cross-provider behaviour

| Surface | jcode | awp |
|---|---|---|
| Provider catalog / failover | `jcode-base::provider::failover` + `provider::account_failover` (separate subsystems) | `internal/llm/failover_core.go` (Sprint 3, merged) — uses `failover.PickNextFallbackRoute` |
| Distinct-provider e2e | n/a — jcode doesn't have a single shared FailoverCore surface | `test/llm/failover_distinct_provider_e2e_test.go:18-117` (`TestEndToEndFailoverAcrossDistinctProviders`, MiniMax→Anthropic) |

Deviation type: extra (awp ships distinct-provider e2e; jcode's
provider surfacing is different).
Severity: nit.
Recommended action: accept-with-note. Sprint 3's FailoverCore was
verified with two same-provider cores; Commit C adds the
distinct-provider e2e that Sprint 4 needs because Anthropic and
MiniMax are now both real providers in the registry.

## Audit summary

| Severity | Count |
|---|---|
| Blocker | 0 |
| Should-fix | 4 (max output tokens too low, BetaHeaders not threaded into request body, adaptive thinking not used, OAuth attribution headers missing) |
| Nit | 5 (provider struct shape, model list scope, context mode classifier, request-building surface shape, event-parser shape) |
| Extra | 1 (distinct-provider e2e) |
| Missing | 2 (model list, OAuth attribution) |

All 4 should-fix items are re-scope-phase (Sprint 5+), not
amend-now blockers. The shared Anthropic protocol package absorbs
the conversion logic cleanly; both providers delegate.

## Recommended closeout actions

Per the audit-driven closeout rule, executing the following as
Sprint 4 follow-up commits is acceptable autonomous work because
they are explicit audit recommendations, reversible, and don't
grow the public API surface:

- **Option 1 — max output tokens amendment** (amend-now,
  ~20 LOC). Bump opus-4-6 to 128_000, sonnet-4-6 to 64_000,
  haiku-4-5 to 64_000 (per jcode's `anthropic_max_output_tokens`
  table at `crates/jcode-provider-core/src/anthropic.rs:154-188`).
  Real Anthropic API supports these limits; long agentic runs
  currently truncate early on opus-4-6. No new types, no new
  tests required beyond bumping the existing
  `TestAnthropicProviderMaxOutputTokens` table.

- **Option 2 — BetaHeaders threading into request body** (amend-
  now, ~30 LOC). Add a `BetaHeaders []string` field to
  `protocol/anthropic.AnthropicRequest`, thread it through
  `BuildAnthropicRequest` → emit as the `anthropic-beta` header
  (or merged into the `extra-headers` slot). Caller in
  `AnthropicProvider.ConvertRequest` reads its own
  `p.BetaHeaders(req.Model)` and passes the result. New test in
  `test/llm/anthropic_e2e_test.go` asserts the header is sent
  for opus-4-6[1m] and not for the others.

- **Option 3 — distinct-provider failover audit doc** (already
  in place). Sprint 4 Commit C ships the test
  `TestEndToEndFailoverAcrossDistinctProviders` and the audit
  records its source-surface (Sprint 3's `FailoverCore` +
  Sprint 4's `AnthropicProvider`). No follow-up commit needed.

Recommended: **execute Option 1 + Option 2 as a single Sprint 4
closeout commit** (`docs/architecture/sprint-4-closeout.md` for
the audit-driven context, then code + tests). Both are
single-file changes, have explicit audit provenance, and are
fully reversible via `git revert`.