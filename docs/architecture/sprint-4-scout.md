# Sprint 4 scout — AnthropicProvider + shared Anthropic protocol

## Goal

Add a real `AnthropicProvider` (talks to `https://api.anthropic.com`)
as a sibling to `MiniMaxProvider`. The two providers share the same
Anthropic Messages wire format; refactor `MiniMaxProvider`'s
request/response conversion onto the existing-but-unused
`internal/llm/protocol/anthropic/` package and have `AnthropicProvider`
consume the same package.

This delivers the "real second provider" surface listed as the #1
Sprint 4 candidate in `docs/architecture/sprint-3-ship-summary.md`.

## Why now

- `internal/llm/protocol/anthropic/{anthropic_request.go,anthropic_sse.go}`
  already exists (316 + 455 LOC) and exposes `AnthropicRequest`,
  `ParseAnthropicSSE`, and the SSE-state machinery. It is currently
  dead code from a runtime perspective.
- `MiniMaxProvider` (578 LOC) re-implements the same wire format
  end-to-end inside `ConvertRequest` (185 LOC) and `ConvertResponse`
  (102 LOC). Removing the duplication halves the per-provider
  maintenance cost and makes adding a third provider (e.g. bedrock
  via SigV4, vertex via google-api) a 100-LOC file instead of 700.
- FailoverCore (Sprint 3) needs a second real provider to justify its
  existence beyond the e2e stub. Without it the e2e tests stay on
  `MiniMaxProvider` twice.

## Reference surface

| jcode file | LOC | awp file (target) | LOC |
|---|---|---|---|
| `crates/jcode-base/src/provider/anthropic.rs` | 140 | `internal/llm/providers/anthropic.go` (new) | ~200 |
| `crates/jcode-provider-core/src/anthropic.rs` (request/SSE impl) | 611 | `internal/llm/protocol/anthropic/{request,sse}.go` (existing; refactor MiniMax onto it) | 316 + 455 |
| `crates/jcode-provider-anthropic-runtime/` | n/a | n/a (jcode splits runtime into its own crate; awp keeps providers in-tree) | — |

## Commit plan

### Commit A — refactor `MiniMaxProvider` onto shared Anthropic protocol

Move `ConvertRequest`, `ConvertResponse`, `CompleteSplit` from
`MiniMaxProvider` into `internal/llm/protocol/anthropic/` as
exported functions (`BuildAnthropicRequest`,
`ParseAnthropicSSEResponse`, `CompleteAnthropicSystemSplit`).
Update `MiniMaxProvider` to delegate. All existing MiniMax-related
tests stay green; per-sprint policy: zero regressions, all 17 packages
`-race` green.

### Commit B — add `AnthropicProvider`

New `internal/llm/providers/anthropic.go` (~200 LOC):
- `AnthropicProvider { APIKey string; BetaAccess bool }`
- `Name() = "anthropic"`, `BaseURL() = "https://api.anthropic.com"`,
  `Path() = "/v1/messages"`, `Headers()` returns
  `{x-api-key, anthropic-version: 2023-06-01, content-type}`.
- `Models()`: claude-opus-4-6, claude-sonnet-4-6,
  claude-opus-4-6[1m], claude-haiku-4-5 (with the right
  context windows, output limits, capability flags).
- Per-model `modelSpec` table; `lookupSpec` helper.
- `BetaHeaders` returns the 1m-context beta header
  (`anthropic-beta: context-1m-2025-08-07`) when the model name
  carries the `[1m]` suffix.
- `ConvertRequest`, `ConvertResponse`, `CompleteSplit` delegate to
  the shared package.

### Commit C — end-to-end test through the new provider

`test/llm/providers/anthropic_e2e_test.go` (~150 LOC):
- httptest server returning valid Anthropic SSE
  (`message_start` → `content_block_delta` → `content_block_stop` →
  `message_delta` → `message_stop`).
- Real `protocol.NewHTTPRestWithClient(client)` + `AnthropicProvider`
  → asserts the StreamChunk decoded by the shared parser contains
  the expected text delta + stop.
- Second httptest server returning 401 to verify
  `errors.Classify` recognises the auth failure on the new
  provider's wire format.
- Third test: real `FailoverCore` wrapping
  `[AnthropicProvider → MiniMaxProvider]` using the
  Sprint-3 `FailoverCore` to exercise cross-provider failover with
  two distinct provider implementations (not the e2e stub pattern
  where both routes used `MiniMaxProvider`).

### Commit D — alignment audit (overlay §1)

`docs/architecture/sprint-4-alignment-audit.md`. Compare
`providers/anthropic.go` against
`jcode-base/src/provider/anthropic.rs:1-140` (CLAUDE_CLI_USER_AGENT,
beta headers, AVAILABLE_MODELS) and jcode's runtime-side request
shaping in `jcode-provider-anthropic-runtime/`. Document the
intentional divergences.

### Commit E — ship summary

`docs/architecture/sprint-4-ship-summary.md`. Capture commits,
verification matrix, open questions, candidate Sprint 5 surfaces.

## Verification matrix

| Check | Expected |
|---|---|
| `gofmt -l .` | clean |
| `go vet ./...` | clean |
| `go test -race -count=1 ./...` | 17+ packages green |
| MiniMaxProvider coverage | unchanged or improved (refactor) |
| AnthropicProvider coverage | ≥80% (per Open/Close on the public surface) |
| Anthropic protocol package coverage | unchanged or improved (refactor moves more code into tested package) |
| FailoverCore e2e with distinct providers | passes |

## Open questions for user

1. **Should the Sprint 4 scope also wire AnthropicProvider into
   `cmd/awp/main.go` loadAgent() (env var `ANTHROPIC_API_KEY`)?**
   - Pro: complete failover chain in production; the new provider is
     actually reachable.
   - Con: requires a FailoverCore integration in loadAgent() that
     exceeds the Sprint 3 stub patterns. Could be Sprint 5.
2. **Models list scope:** port jcode's full `AVAILABLE_MODELS` (≈12
   entries) or start with the 4 most common (opus-4-6, sonnet-4-6,
   haiku-4-5, opus-4-6[1m])?
3. **OAuth attribution headers** (Claude CLI user-agent,
   `anthropic-dangerous-direct-browser-access`): port now or defer
   to a future OAuth-support sprint?
4. **Should `AnthropicProvider` be in `providers/` (alongside
   MiniMaxProvider) or in a new `providers/anthropic/` subpackage
   to leave room for per-provider variants?**
