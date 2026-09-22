# AGENTS.md

Go binary (`awp`) — LLM agent over JSON-RPC 2.0 (Unix socket) with a bubbletea TUI.

## Layout

- `cmd/awp` — entrypoint, subcommands (`serve` / `connect` / `resume` / `demo`).
- `internal/agent` + `internal/agent/tools` — ReAct loop, built-in tools (file / shell / search). Each tool implementation lives in `{name}_extension.go`; helpers (`errors.go`, `intent.go`, `registry.go`, `shared.go`, `truncate.go`) keep effect-based names.
- `internal/llm` + `internal/llm/protocol` + `internal/llm/providers` — `Provider` / `Protocol` / `Core` / `RetryCore` / `Registry`. `minimax` is the default vendor (Anthropic-compat).
- `internal/server` — accept loop, JSON-RPC 2.0 dispatch, JSONL session store.
- `internal/protocol` — JSON-RPC 2.0 message types, shared by `server` and `client-sdk`.
- `internal/client-sdk` — Go client SDK (`Dial` / `Prompt` / `Resume` / `Ping`).
- `internal/storage` — XDG paths. `internal/log` — slog rotation. `internal/transport` — Unix domain socket.
- `internal/tui` — bubbletea frontend (default `awp` mode).
- `test/` — external tests, mirror `internal/` (`package <pkg>_test`).

## Commands

- `awp` — TUI (default, requires terminal).
- `awp serve --socket <path>` — background server on a per-instance socket (default `~/.awp/runtime/awp.sock.<serve_pid>`).
- `awp connect --connect-pid <pid> <prompt>` — send prompt to an existing instance (TUI or `awp serve`) identified by PID; no own server.
- `awp resume --connect-pid <pid> <session_id> [new_prompt]` — replay or extend a session on an existing instance; no own server.
- Each `awp` TUI spawns its own server and tracks it via `~/.awp/runtime/awp.tui.<tui_pid>.pid`. TUI shutdown SIGTERMs the server.
- `awp demo` — in-process demo (no server).

## Build & verify

- `go build ./...` · `go vet ./...` · `go test -race ./test/...`
- `gofmt -w` before commit. `make build / make test / make lint` (see Makefile).
- **AFT integration tests** (`internal/agent/tools/aft_*_test.go`): skip
  automatically when `aft` is not on `PATH`. Run `cargo install --path
  <aft-repo>/crates/aft --locked` or set `AWP_TEST_AFT=/path/to/aft` to
  exercise them.

## AFT integration

- AFT ([github.com/cortexkit/aft](https://github.com/cortexkit/aft)) is an
  optional Rust worker that AWP spawns as a long-lived subprocess. It
  implements file/bash/edit/symbol/grep tools in Rust with image/PDF inline,
  hashline byte verification, AST awareness via `oxc_engine`, callgraph,
  semantic search via ONNX embeddings, and a unified `tool_call` NDJSON
  protocol.
- **Detection**: `exec.LookPath("aft")` at first tool registration. If
  `AWP_NO_AFT=1`, force the Go path.
- **Lifecycle**: one `aft` worker per AWP session. Worker is started when
  the registry is built (first tool call) and stopped by closing its stdin,
  which triggers `[aft] stdin closed, shutting down`. If the worker exits
  unexpectedly mid-session, the wrapper falls back to Go for that call only
  and re-attempts the worker on the next call.
- **Protocol**: NDJSON over stdin/stdout. Request: `{"id":"awp-N","command":"<name>","<param>":...}`.
  Response: `{"id":"awp-N","success":bool,"<data>":...}`. Lock-step per
  worker (no concurrent in-flight calls in Phase 1; mutating calls
  serialize on a per-worker mutex).
- **Subprocess args**: `aft` is invoked with no CLI args (NDJSON standalone
  mode). Stderr is captured to `$TMP/aft-<pid>.log` for crash triage.
- **Failure modes**:
  - `aft` not on `PATH` → silent fallback to Go.
  - `aft` crashes mid-session → the next tool call returns
    `ErrAftUnavailable`, the wrapper tries once to restart the worker,
    then falls back to Go for that call.
  - `aft` returns `success: false` with `code` + `message` → wrapper
    surfaces as Go error (so the LLM sees the same `Tool X failed: ...`
    shape it already handles).
- **Adding a new AFT-backed tool**: write `ReadXxxAft(cwd)` /
  `EditXxxAft(cwd)` etc. in `internal/agent/tools/aft_extension.go`,
  mirror the Go impl's `agent.Tool` interface exactly (same `Name`,
  `Description`, `Parameters`, `Invoke` signature). Add to
  `tools.All(cwd)` registry branch when `useAft`. Keep the Go impl as the
  fallback for users without `aft` installed.
- **Why not cgo / FFI**: AFT is published as a Rust crate but has no
  C-ABI binding. A cgo bridge would require upstream changes to AFT and
  per-platform build complexity; subprocess + NDJSON is the documented
  transport and gives us the same capability with ~225 lines of Go.

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
10. **AFT backend (optional)**: when `aft` is on `PATH`, AWP spawns a long-lived `aft` subprocess at agent start and routes file/bash/edit tools through it via NDJSON over stdin/stdout. The `aft` worker lives for the duration of the TUI/serve session; closing stdin triggers `[aft] stdin closed, shutting down`. When `aft` is missing, AWP silently falls back to the Go implementations in `internal/agent/tools/`. Set `AWP_NO_AFT=1` to disable AFT entirely. Implementation: `internal/agent/tools/aft_backend.go` (subprocess + NDJSON), `internal/agent/tools/aft_extension.go` (per-tool wrappers). Both Go and AFT implementations share the same `agent.Tool` interface, so the registry swap is transparent.

## TUI constraints

- **Prefer `charm.land/bubbles/v2` components** over self-rolled implementations: `viewport` (chat scroll, `SoftWrap = true` for auto-wrap), `textarea` (multi-line input), `textinput` only when single-line is fixed, `spinner` (`spinner.MiniDot`), `help` + `key.Binding` (`ShortHelpView` / `FullHelpView`), `progress` (token progress), `filepicker` (file pickers), `list` (autocomplete), `table` / `tree` / `timer` / `paginator` as needed. If bubbles ships a component, use it.
- **Prefer `charm.land/lipgloss/v2` layout**: `Style.Width(n).Render(text)` for chat-pane line wrap, `Style.Height(n)`, `Style.Border(b)`, `Style.Align(...)`, `Style.Padding(...)`, `Style.Margin(...)`, `lipgloss.JoinHorizontal` / `lipgloss.JoinVertical` for composite layouts. `lipgloss.Width(s)` for ANSI-aware width measurement.
- **No self-rolled layout primitives**: do not reimplement line wrapping, alignment indent, or width counters by hand — lipgloss v2 is ANSI-aware and word-aware; hand-rolled byte/rune counters risk subtle ANSI/Unicode bugs.
- **No self-rolled ANSI spinner frames**: `bubbles/spinner` is the only spinner. The TUI Model owns one `spinner.Model`; tick via `m.spinner.Tick()` and `m.spinner.Update(spinner.TickMsg)`.
- **Header status composition**: split the header into brand + separator + status + track + session so status colors are not overwritten by an outer `Style.Width(n).Render(...)` wrapper. The wrapper collapses any inner ANSI escapes.
- **No inline ANSI escapes in render**: do not write `\x1b[...]` strings by hand; render through lipgloss styles only.
- **TUI palette** lives in `internal/tui/styles.go` and is the only place colors are defined; other TUI files reference styles, never raw hex.
- **No comments in TUI code**; behavioural explanation lives in test names and commit messages (project rule).

## Code style

- **No comments in code.** `//` and `/* */` are stripped on sight. Behavioural explanation lives in test names and commit messages.
- **One package per directory.** Never split across siblings.
- **Errors**: `fmt.Errorf("...: %w", err)` + `errors.Is` / `errors.As`. Wrap vendor / transport / network failures in `*llm.Error` with a `Kind` (`Auth / RateLimit / Client / Server / Network / Vendor`).
- **Context first**: `func(ctx context.Context, ...)`. Propagate to `http.NewRequestWithContext`.
- **Tests** are external (`package <pkg>_test`); mirror `internal/`. White-box unexported access is not used — export helpers instead.
