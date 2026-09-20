# AGENTS.md

A Go binary (`awp`) that talks to LLM APIs with tool orchestration, sessions, and a TUI.

## Layout

- `cmd/awp/main.go` — entrypoint, multi-subcommand dispatcher.
- `internal/agent/` — ReAct loop, events, message history, compact.
- `internal/agent/tools/` — built-in tools (read / write / edit / bash / grep / find / ls).
- `internal/llm/` — message types, `Provider` / `Protocol` interfaces, `Core`, `RetryCore`, error taxonomy, `Registry`.
- `internal/llm/protocol/` — transport (`HTTPRest`: HTTP + SSE). Leaf pkg.
- `internal/llm/providers/` — vendor adapters (`minimax` is Anthropic-compat).
- `internal/client-sdk/` — Unix socket client SDK (Dial / Prompt / Resume / Ping).
- `internal/server/` — accept loop + JSON-RPC 2.0 dispatch.
- `internal/protocol/` — JSON-RPC 2.0 message types (Request / Response).
- `internal/session/` — JSONL append-only store (header + events).
- `internal/storage/` — XDG paths (`Home` / `RuntimeDir` / `ConfigDir` / `LogsDir` / `SessionsDir` / `SocketPath`).
- `internal/log/` — `slog` multi-handler + file rotation + cleanup.
- `internal/transport/` — Unix domain socket (`Listen` / `Dial` / `IsRunning`).
- `internal/tui/` — bubbletea TUI (default `awp` mode).
- `test/` — external tests, mirror `internal/` structure (`package <pkg>_test`).

## Commands

- `awp` — launch TUI (default, requires terminal).
- `awp serve` — run as background server (Unix socket).
- `awp connect <prompt>` — spawn server + send prompt + print events.
- `awp resume <session_id>` — replay session events from JSONL.
- `awp demo` — in-process demo (no server).

Server socket: `$AWP_SOCKET` (default `~/.awp/runtime/awp.sock`).

## Build & verify

- `go build ./...`
- `go vet ./...`
- `go test ./test/...`
- `make build / make test / make lint` — see Makefile.
- `gofmt -w` before commit. CI should run `go vet ./...` and
  `go test -race ./test/...`.

## Architectural rules

1. `llm` owns the `Provider` / `Protocol` / `Registry` / `RetryCore`
   abstractions. Vendors depend on `llm`; agents depend on `llm`;
   `protocol` depends on nothing internal. No cycles.
2. Interfaces live with their consumer. `Provider` is consumed by
   `Core` so it lives in `internal/llm/core.go`. Do not move it. Keep
   the interface small — strong abstractions beat fat ones.
3. `internal/llm/protocol/` must not import `llm`. Transport errors
   cross the boundary as `*protocol.HTTPError`; `llm.Classify`
   upgrades them to `*llm.Error` with a `Kind` for retry decisions.
4. LLM layer = single-call. Multi-turn loop, tool execution, message
   history, and capability validation live in `internal/agent/`. Do
   not put loop logic in `llm/`.
5. `Core.StreamChat` is the only method on `Core`. Streaming events
   ride a single channel of `StreamEvent{Chunk, Err}`. Tool-call
   accumulation happens across SSE deltas — assembled `[]ToolCall`
   rides the LAST `StreamEvent{Chunk}` on `message_stop`.
6. `RetryCore` retries only `StreamChat` on transient errors
   (`RateLimit` / `Server` / `Network`). Streamed events are not
   retried — mid-stream errors propagate to the caller.
7. Default provider is registered under `llm.DefaultProviderName`
   (`"minimax"`). New vendors plug into `Registry` in `cmd/awp/main.go`.
8. `internal/server/` exposes JSON-RPC 2.0 over Unix domain socket.
   `internal/client-sdk/` is the Go client. They share
   `internal/protocol/` for message types — never duplicate.
9. `internal/session/` is JSONL append-only. Headers and events
   share one file. Compaction is left to future work.
10. `internal/tui/` uses bubbletea. It depends on `internal/client-sdk`
    and `internal/storage` — never on `internal/server/` or
    `internal/agent/` directly.

## Code style

- No comments in code. Inline `//` and `/* */` are stripped on sight.
  Behavioural explanation lives in test names and commit messages.
- One package per directory. Do not split a package across siblings.
- Errors: `fmt.Errorf("...: %w", err)`. Match with `errors.Is` /
  `errors.As`. Vendor / transport / network failures are wrapped in
  `*llm.Error` with a `Kind` (`Auth / RateLimit / Client / Server
  / Network / Vendor`).
- Context first: `func(ctx context.Context, ...)`. Propagate to
  `http.NewRequestWithContext`.

## Adding a vendor

1. New file `internal/llm/providers/<name>.go` implementing `Provider`
   (`Name / BaseURL / Path / Headers / ConvertRequest /
   ConvertResponse / Models`).
2. Register in `cmd/awp/main.go` `loadAgent()`:
   `registry.MustRegisterDefault(providers.New<Name>(...))`.
3. Vendor field-name quirks are handled inside the Provider. Do not
   push them up to `Core` or the agent.

## Adding a tool

- Build `llm.ToolDef` with a JSON Schema `Parameters`.
- Register via `ag.WithTool(agent.Tool{...})` where the tool's
  `Execute` takes `argsJSON string` and returns `(result string,
  err error)`.
- Add the tool to `loadAgent()` in `cmd/awp/main.go`.
- Parse args locally with `json.Unmarshal` into a typed struct.

## Testing

- Tests live in `test/`, mirror `internal/` structure.
- Each test package is `package <pkg>_test` (external).
- White-box unexported access is not used — export helpers instead.
