# Sprint 5 alignment audit — adaptive thinking + Anthropic classifiers

Compared against the reference implementation at `~/Project/lazible-jcode/patched-jcode/crates/jcode-provider-core/src/anthropic.rs` and `~/Project/lazible-jcode/patched-jcode/crates/jcode-provider-anthropic-runtime/src/{reasoning_request.rs, lib.rs}`.

Audit method: each surface in awp is mapped to its jcode counterpart by file:line citations; deltas are classified (missing / renamed / semantic-difference / extra / shape-different) and severity-rated (blocker / should-fix / nit). The audit closes the Scout's open question 4 (FailoverCore context-fit routing) and the deferred-self-heal question from Commit B.

## Verdict

**Faithful translation with one intentional deviation.** No blocker findings. One should-fix item (self-heal prerequisite) and three nits. Two findings (F1, F2) were retracted after spot-check against the actual files; the audit's value here is negative — confirming correctness rather than flagging divergence.

## Surface map

| Surface | jcode reference | awp surface | Delta |
|---|---|---|---|
| `AnthropicContextMode` enum | `anthropic.rs:1-50` | `capability.go:1-11` | **shape-different** (Go: `ContextStandard`/`ContextOptIn1M`/`ContextNative1M`; Rust: `Standard`/`OptIn1M`/`Native1M`). Intent identical. |
| `AnthropicReasoningCaps` struct | `anthropic.rs:232-243` | `anthropic_caps.go:13-19` | **shape-different** (Go fields exported as PascalCase; jcode snake_case). Fields identical: `OutputEffort`/`output_effort`, `AdaptiveThinking`/`adaptive_thinking`, `ManualThinking`/`manual_thinking`, `XHighEffort`/`xhigh_effort`, `MaxEffort`/`max_effort`. |
| `AnthropicReasoningCaps::FULL` / `EFFORT_NO_XHIGH` / `MANUAL_WITH_EFFORT` / `MANUAL_ONLY` / `NONE` | `anthropic.rs:245-284` | `anthropic_caps.go:25-46` (`capsFull`/`capsEffortNoXhigh`/`capsManualWithEffort`/`capsManualOnly`/`capsNone`) | **renamed** (Go requires `var`, not `const`, for struct values; same five semantics). |
| `supports_reasoning_effort()` | `anthropic.rs:287-289` | `anthropic_caps.go:50-52` | **shape-different** (method receiver vs method). Semantics identical (`output_effort || manual_thinking`). |
| `anthropic_context_mode` | `anthropic.rs:62-107` | `anthropic_caps.go:86-104` (`ContextMode`) | **identical** (spot-checked against the catch-all path for `mythos` ≥ 5.0 — both return `Native1M`). |
| `anthropic_is_1m_model` | `anthropic.rs:111-113` | `anthropic_caps.go:229-231` (`Strip1mSuffix` predicate via `HasSuffix("[1m]")`) | **extra** — exposed as a method on the stripped form rather than a boolean; equivalent. |
| `anthropic_max_output_tokens` | `anthropic.rs:169-204` | `anthropic_caps.go:170-183` (`MaxOutputTokens`) | **identical** (`largeOutputPrefixes` has the same 14 prefixes as jcode's `LARGE_OUTPUT_PREFIXES`; `haiku64kPrefixes` matches `claude-haiku-4-5`/`claude-haiku-4.5`). |
| `anthropic_thinking_always_on` | `anthropic.rs:147-159` | not implemented in awp | **missing** — see Finding F1 (should-fix) |
| `anthropic_context_mode_is_verified` | `anthropic.rs:131-145` | not implemented in awp | **missing** — see Finding F2 (nit) |
| `claude_id_has_parseable_version` | `anthropic.rs:117-120` | not exposed | **missing** — internal helper, defer |
| `anthropic_reasoning_caps` | `anthropic.rs:336-381` | `anthropic_caps.go:122-159` (`ReasoningCaps`) | **identical** (spot-checked opus/sonnet/fable branches and the version-5+ catch-all; matches jcode's five-way classification). |
| `normalized_claude_caps_key` | `anthropic.rs:294-299` | `anthropic_caps.go:238-246` (`normalizeClaudeCapsKey`) | identical |
| `parse_claude_family_version` | `anthropic.rs:304-325` | `anthropic_caps.go:285-309` (`parseClaudeFamilyVersion`) | identical (with minor refactor to `claudeVersion` struct) |
| `strip_date_suffix` | `model_id.rs:43-50` | `anthropic_caps.go:248-270` (`stripDateSuffix`) | identical |
| `LARGE_OUTPUT_PREFIXES` table | `anthropic.rs:174-189` | `anthropic_caps.go:343-358` (`largeOutputPrefixes`) | identical (14 prefixes each, sorted-set diff is whitespace-only) |
| `manual_thinking_budget` | `lib.rs:868-879` | `anthropic.go:303-330` (`manualThinkingBudget`) | identical |
| `default_reasoning_effort_for_model` | `lib.rs:788-807` | `anthropic.go:205-225` (`DefaultReasoningEffort`) | identical |
| `adaptive_thinking` (helper) | `reasoning_request.rs:8-17` | inlined in `anthropic.go:256-264` (`Type: "adaptive"` + `OutputConfig`) | **shape-different** (no `block_binding` or `display` controls — see Finding F3) |
| `recover_rejected_reasoning` (self-heal) | `reasoning_request.rs:95-115` | not implemented | **missing** — see Finding F4 (deferred, intentionally) |
| `AnthropicThinking` wire type | `runtime::ApiThinking` | `anthropic_request.go:38-43` | **shape-different** — Go has flat `Type` + `BudgetTokens`; Rust has sum type `Adaptive { display, block_binding }` / `Enabled { budget_tokens }`. See Finding F3. |
| `AnthropicOutputConfig` wire type | `runtime::ApiOutputConfig` | `anthropic_request.go:49-54` | identical |
| `BuildAnthropicRequest` serialization | runtime | `anthropic_request.go` | **shape-different** — no `block_binding`, no `display: summarized`. See Finding F3. |
| `headersFor` (beta header injection) | runtime `headers_for` | `core.go:243-255` | identical |
| `ChatRequest.ReasoningEffort` | `runtime::ChatRequest::reasoning_effort` | `types.go` | identical (added as additive field) |
| `ModelCapabilities` + new caps fields | `runtime::ModelCapabilities` | `capability.go:13-31` | **extra** — added `OutputEffort`/`AdaptiveThinking`/`ManualThinking` so callers can introspect without re-parsing |

## Findings

### F1 — `anthropic_thinking_always_on` not implemented (missing, should-fix)

jcode's `anthropic_thinking_always_on` (`anthropic.rs:147-159`) returns `true` for `claude-opus-5-5` and `claude-fable-5-1`. These models require thinking on every request; the runtime's self-heal path preserves adaptive thinking but drops `output_config` when the API rejects it.

awp's `ConvertRequest` (`anthropic.go:248-264`) treats adaptive thinking as "always emit when `caps.AdaptiveThinking`" without checking whether it is mandatory. For opus-5-5 / fable-5-1 this is correct (we always emit it). The self-heal path (F4) is what makes "always-on" matter: when the API rejects, jcode drops `output_config` but keeps `adaptive_thinking`, while non-always-on models drop both.

Without self-heal, this distinction is invisible. With self-heal, it becomes critical.

**Action**: add a small helper `anthropic.ThinkingAlwaysOn(model) bool` mirroring jcode. No call-site yet — lands with F4 in the closeout commit.

### F2 — `anthropic_context_mode_is_verified` not exposed (missing, nit)

jcode exposes a `verified` predicate so callers can decide precedence: a verified classification beats a catalog lookup, while an unverified one yields to catalog/config.

awp has no catalog lookup today (Sprint 6+ introduces the model catalogue). The `verified` surface is a no-op until the catalog arrives.

**Action**: defer until Sprint 6 (catalogue work). Accept-with-note.

### F3 — `AnthropicThinking` lacks `block_binding` and `display` controls (shape-different, nit)

jcode's `ApiThinking::Adaptive { display, block_binding }` carries per-request controls that govern how Anthropic streams thinking summaries (`display: summarized`) and how the runtime handles prefix mismatches (`block_binding: { prefix_mismatch_behavior: "drop_block" }`). The BINDING_BETA header (`context-1m-2025-08-07`-adjacent) is added alongside when binding is on.

awp's `AnthropicThinking` (`anthropic_request.go:38-43`) carries only `Type` + `BudgetTokens`. This is intentional for Sprint 5 — the binding protocol is an opus-5-5 / fable-5-1 feature and we have not pinned any opus-5 generation in the spec table yet.

**Action**: defer until Sprint 6+ when opus-5 series lands. Accept-with-note; document in `anthropic.go:212-219` comment block.

### F4 — Self-heal retry `recover_rejected_reasoning` not implemented (missing, deferred)

jcode's `recover_rejected_reasoning` (`reasoning_request.rs:95-115`) detects a 400 with `invalid_request_error` + `thinking`/`effort`/`output_config` + `not supported`/`does not support`, then drops reasoning fields and retries once.

awp skipped this in Commit B (deferred to closeout per the Sprint 5 scout's open-question note). The classifier covers every known Claude generation (opus 3.5–5.5+, sonnet 3.7–5, haiku 4.5–5, fable 5–5.1, mythos, nova). The transition window for unknown future generations is the value-add.

**Action**: implement as a follow-up commit closeout. Wire at the `core` level so it is generic (not Anthropic-only), but `AnthropicProvider` implements the recovery. Size estimate: ~80 LOC + ~120 LOC of e2e tests that simulate the 400 then a successful retry. Carries F1's `ThinkingAlwaysOn` helper as a precondition.

### F5 — `claude-mythos-5` `ContextMode` not pinned by test (coverage gap, nit)

`ContextMode("claude-mythos-5")` is not exercised by `test/llm/protocol/anthropic_caps_test.go`. The actual classification is correct (jcode's catch-all returns `Native1M` for version-5+; awp's catch-all matches), but the absence of a test means a future refactor could regress without detection. The reasoning-caps path does have a mythos test (capsEffortNoXhigh), so the two classifiers are at parity in coverage.

**Action**: add a single test case pinning `claude-mythos-5` → `ContextNative1M` and `claude-mythos-5` reasoning caps → `EFFORT_NO_XHIGH`. No production code change. Group with F1's `ThinkingAlwaysOn` helper in the closeout commit (covers a related coverage gap).

## Withdrawn findings

The following were initially claimed in a draft of this audit and **withdrew** after spot-checking the actual source files.

- **Withdrawn F1**: "ContextMode does not short-circuit mythos; the catch-all would still hit Native1M but is not exercised by tests." Re-classified as F5: the behavior is correct, only the test is missing.
- **Withdrawn F2**: "`largeOutputPrefixes` is missing `claude-opus-4.7` (dotted); jcode has 13 prefixes, awp has 12." Re-verified with `diff <(jcode sorted) <(awp sorted)`: both lists have the same 14 prefixes, the diff is whitespace-only. The initial claim was a miscount.

These are listed here because the audit's value is partly negative — confirming correctness is as useful as flagging divergence, and the user should see the corrections.

## Closeout plan

1. **Should-fix commit (single)**:
   - F1: add `anthropic.ThinkingAlwaysOn(model) bool` helper mirroring jcode + test.
   - F5: add `claude-mythos-5` test cases pinning `ContextMode` and `ReasoningCaps` behavior.

2. **Self-heal commit (separate)**:
   - F4: wire `RecoverRequest` into `Provider` interface; `AnthropicProvider` implements recovery; `core.StreamChat` wraps with a one-shot retry loop on first-failure-before-streaming. Carries F1's `ThinkingAlwaysOn` as a precondition.

3. **Accept-with-note**:
   - F2: `verified` predicate — defer to Sprint 6.
   - F3: `block_binding`/`display` controls — defer to opus-5 series sprint.

## Coverage roll-up

| Surface | jcode LOC | awp LOC | Test LOC | Coverage |
|---|---|---|---|---|
| `AnthropicReasoningCaps` + classifiers | 110 | 361 (`anthropic_caps.go`) | 332 (`anthropic_caps_test.go`) | 90–100% per fn |
| `ConvertRequest` + manual-thinking budget + defaults | 320 (lib.rs reshape + recover + budget + default) | 135 (`anthropic.go:205-330`) | 200 (`anthropic_test.go` new tests) | 87% (ConvertRequest), 92% (manualThinkingBudget) |
| `AnthropicRequest` + `BuildAnthropicRequest` | 110 | 60 (`anthropic_request.go` edits) | n/a (covered by `anthropic_test.go` + `anthropic_e2e_test.go`) | covered via ConvertRequest tests |

**Test sweep**: 17/17 packages green `-race -count=1 -timeout=180s`. End-to-end classifier dispatch tested via `TestEndToEndAnthropicProvider{ClassifierDispatch_ManualThinking, ClassifierDispatch_XHighDefault, OneMSuffixStripAndBeta, HaikuOmitsThinking, DrivingRealHTTPRest}`.

## Memory tag

Recorded under `alignment-audit` tag. Refer back to this file when implementing the closeout batch (F1+F5) and the self-heal follow-up (F4).
