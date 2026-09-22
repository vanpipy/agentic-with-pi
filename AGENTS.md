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
- `awp serve --socket <path>` — background server on a per-instance socket (default `~/.awp/runtime/awp.sock.<serve_pid>`).
- `awp connect --connect-pid <pid> <prompt>` — send prompt to an existing instance (TUI or `awp serve`) identified by PID; no own server.
- `awp resume --connect-pid <pid> <session_id> [new_prompt]` — replay or extend a session on an existing instance; no own server.
- Each `awp` TUI spawns its own server and tracks it via `~/.awp/runtime/awp.tui.<tui_pid>.pid`. TUI shutdown SIGTERMs the server.
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
