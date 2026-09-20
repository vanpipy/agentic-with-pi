# AGENTS.md

Go binary (`awp`) — LLM agent over JSON-RPC 2.0 (Unix socket) with a bubbletea TUI.

## Layout

- `cmd/awp` — entrypoint, subcommands (`serve` / `connect` / `resume` / `demo`).
- `internal/agent` + `internal/agent/tools` — ReAct loop, built-in tools (file / shell / search).
- `internal/llm` + `internal/llm/protocol` + `internal/llm/providers` — `Provider` / `Protocol` / `Core` / `RetryCore` / `Registry`. `minimax` is the default vendor (Anthropic-compat).
- `internal/server` — accept loop, JSON-RPC 2.0 dispatch, JSONL session store.
- `internal/protocol` — JSON-RPC 2.0 message types, shared by `server` and `client-sdk`.
- `internal/client-sdk` — Go client SDK (`Dial` / `Prompt` / `Resume` / `Ping`).
- `internal/storage` — XDG paths. `internal/log` — slog rotation. `internal/transport` — Unix domain socket.
- `internal/tui` — bubbletea frontend (default `awp` mode).
- `test/` — external tests, mirror `internal/` (`package <pkg>_test`).

## Commands

- `awp` — TUI (default, requires terminal).
- `awp serve` — background server on `$AWP_SOCKET` (default `~/.awp/runtime/awp.sock`).
- `awp connect <prompt>` — spawn server + send prompt + print events.
- `awp resume <session_id>` — replay session events from JSONL.
- `awp demo` — in-process demo (no server).

## Build & verify

- `go build ./...` · `go vet ./...` · `go test -race ./test/...`
- `gofmt -w` before commit. `make build / make test / make lint` (see Makefile).

## Architectural rules

1. **No cycles.** `llm` is the leaf for vendors and the agent; `internal/llm/protocol` depends on nothing internal.
2. **Interfaces live with their consumer** — small surfaces, strong abstractions.
3. **Transport boundary**: `protocol` returns `*protocol.HTTPError`; `llm.Classify` upgrades to `*llm.Error` with a `Kind`.
4. **LLM layer is single-call.** `Core.StreamChat` is the only method; multi-turn, tools, history live in `internal/agent`.
5. **Streaming**: one channel of `StreamEvent{Chunk, Err}`. Tool calls ride the last chunk on `message_stop`. `RetryCore` retries only `StreamChat` on transient kinds (`RateLimit` / `Server` / `Network`).
6. **Server ↔ client-sdk**: share `internal/protocol/` only. Never duplicate. Server also owns the JSONL session store (`Store` / `Load` / `SessionMeta` / `NewID`).
7. **TUI**: depends on `internal/client-sdk` + `internal/storage`. Never `internal/server` or `internal/agent` directly.
8. **Adding a vendor**: implement `Provider` in `internal/llm/providers/<name>.go`, register in `cmd/awp/main.go loadAgent()`. Field-name quirks stay inside the provider.
9. **Adding a tool**: build `llm.ToolDef` with JSON Schema, register via `ag.WithTool(agent.Tool{...})`, parse `argsJSON` locally with `json.Unmarshal`. Wire up in `cmd/awp/main.go loadAgent()`.

## Code style

- **No comments in code.** `//` and `/* */` are stripped on sight. Behavioural explanation lives in test names and commit messages.
- **One package per directory.** Never split across siblings.
- **Errors**: `fmt.Errorf("...: %w", err)` + `errors.Is` / `errors.As`. Wrap vendor / transport / network failures in `*llm.Error` with a `Kind` (`Auth / RateLimit / Client / Server / Network / Vendor`).
- **Context first**: `func(ctx context.Context, ...)`. Propagate to `http.NewRequestWithContext`.
- **Tests** are external (`package <pkg>_test`); mirror `internal/`. White-box unexported access is not used — export helpers instead.
