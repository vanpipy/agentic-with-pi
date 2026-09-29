# Sprint 3 ship summary — cross-provider failover

**Branch**: `ws-llm-failover` · **Base**: `main @ 18b0e18` · **Tip**: see `git log ws-llm-failover`
**Local state**: 4 commits, 16 packages green `-race`. Push remains blocked per overlay §2.

## What landed

| Commit | Subject | Files | LOC |
|---|---|---|---|
| `ba23410` | port jcode failover primitive | `internal/llm/failover/{doc,failover}.go`, `test/llm/failover/failover_test.go` | +426 |
| `d6f9fe5` | port fallback_pick primitive | `internal/llm/failover/{route,pick}.go`, `test/llm/failover/pick_test.go` | +557 |
| `1986868` | add FailoverCore (Commit C — wire + e2e) | `internal/llm/failover_core.go`, `test/llm/failover_core_e2e_test.go` | +364 |
| `33c0518` | alignment audit doc | `docs/architecture/sprint-3-alignment-audit.md` | +97 |

**Total**: 4 commits, +1,444 LOC, 0 LOC deleted, 0 backwards-compat shims.

## Verification matrix

| Check | Result |
|---|---|
| `gofmt -l .` | clean |
| `go vet ./...` | clean |
| `go test -race -count=1 ./...` | 17 packages green |
| `internal/llm/failover/` coverage | 92.5% (commit A) + extensions (commit B) |
| failover unit tests | 33 (failover_test.go) + 16 (pick_test.go) = 49 tests |
| FailoverCore e2e tests | 2 (real HTTPRest + MiniMaxProvider + httptest) |
| RetryCore coverage (reused) | 85.5% (unchanged from Sprint 2) |
| jcode alignment | 6 documented divergences, 0 blockers |

## Open questions for user (still unresolved from scout)

1. **Real second provider for e2e** — used `MiniMaxProvider` on both routes
   for the e2e test; real `AnthropicProvider` / `OpenAIProvider` land in
   a later sprint. Action: accept with note.
3. **`ModelRoute` struct vs extend `Model`** — used new struct. The picker
   consults `Provider / APIMethod / Available`, which don't exist on
   `Model`. (Sprint 4+ if the registry grows.)
4. **`failover.classify` replace vs layer with `errors.classify`** —
   layered. `errors.Classify` keeps its Retryable / Kind responsibility;
   `failover.ClassifyErrorMessage` answers "should we switch providers?".
5. **Automatic vs UI prompted** — automatic in `FailoverCore` (this
   sprint); UI prompt emitted as a synthetic `LegacyStreamEvent{Err}`
   for a TUI handler in a later sprint.

## Next sprint pre-work (Sprint 4 candidate)

| Surface | Status |
|---|---|
| `anthropic` provider in `providers/` | not started |
| TUI one-key switch consumer for `ProviderFailoverPrompt` | not started |
| Provider-label alias extensions (bedrock / vertex / azure-openai) | not started |
| `tryRoute` to drop channel drain once `RetryCore` exposes a sealed "eventually failed" type | not started |
| `ModelRoute.Available` registry (per-provider health tracker) | not started |

## How to verify

```bash
cd /home/leroy/Project/ws-llm-failover
gofmt -l . && go vet ./... && go test -race -count=1 ./...
```

Push remains blocked pending explicit "yes" per overlay §2.
