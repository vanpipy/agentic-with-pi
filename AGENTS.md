# AGENTS.md

Go binary (`awp`) — LLM agent over JSON-RPC 2.0 (Unix socket) with a bubbletea TUI.

## 黄金规则 (Golden Rules)

Override everything below. When a lower-priority rule conflicts, the golden rule wins.

1. **不保持向前兼容 (no backward compatibility).** `internal/...` is not importable from outside this module by Go's design; treat that as a feature, not a constraint. Refactor / rename / change signatures / on-disk formats / tool schemas / wire types freely. No deprecation cycles, no compat shims, no version negotiation, no `// Deprecated:` markers, no migration helpers, no "keep the old function around for callers." Delete the old code in the same commit that introduces the new code. The only candidate for cross-release stability is `internal/agent-protocol` (JSON-RPC over UDS); mark it `// stable:` if you lock a surface.
2. **Sealed interfaces only.** Marker methods (`streamEvent()`, `contentBlock()`) are unexported so only this package can implement them. Consumers pattern-match with type switches; the compiler enforces exhaustiveness. Never export an interface satisfied by types in other packages.
3. **Context first, always.** `func(ctx context.Context, ...)`. No `ctx == nil` paths — use `context.Background()`. `http.NewRequestWithContext`, never `http.NewRequest`. `defer cancel()` on every `WithCancel` / `WithTimeout`.
4. **Errors are values, not control flow.** `fmt.Errorf("...: %w", err)` + `errors.Is` / `errors.As`. Sentinel errors (`ErrFoo`) for behavior; typed errors (`*llm.Error`) for data. Error strings lowercase, no terminal punctuation. Don't log and return.
5. **No goroutine leaks.** Every goroutine has an exit path keyed to `ctx.Done()` or channel closure. Sender closes, receiver only ranges. Buffer sizes are part of the contract. `go test -race` is mandatory for any change that touches concurrency.
6. **Tests are external.** `package <pkg>_test` mirrored under `test/`. White-box through unexported fields is forbidden — export helpers. Table-driven for ≥3 cases; `t.Helper()` / `t.Cleanup` / `t.TempDir()`; `-race -count=1` in CI.

## Layout

- `cmd/awp` — entrypoint, subcommands.
- `internal/agent-core` + `internal/agent-core/tools` — ReAct loop, built-in tools.
- `internal/llm` + `internal/llm/protocol` + `internal/llm/providers` — `Provider` / `Protocol` / `Core` / `RetryCore` / `Registry`.
- `internal/agent-server` — accept loop, JSON-RPC 2.0 dispatch, JSONL session store.
- `internal/agent-protocol` — JSON-RPC 2.0 wire types shared by server + client.
- `internal/agent-client` — Go client SDK.
- `internal/paths` · `internal/log` · `internal/ipc` — XDG paths, slog rotation, UDS + PID lifecycle.
- `internal/tui` — bubbletea frontend (default `awp` mode).
- `test/` — external tests, mirror `internal/`.

## Commands

- `awp` — TUI (default).
- `awp serve --socket <path>` — background server on per-instance socket (`~/.awp/runtime/awp.sock.<serve_pid>`).
- `awp connect --connect-pid <pid> <prompt>` — send prompt to existing instance.
- `awp resume --connect-pid <pid> <session_id> [new_prompt]` — replay or extend a session.
- Each TUI spawns its own server (`~/.awp/runtime/awp.tui.<tui_pid>.pid`); SIGTERM on shutdown.
- `awp demo` — in-process demo (no server).

## Build & verify

- `go build ./...` · `go vet ./...` · `go test -race ./test/...`
- `gofmt -w` before commit. `make build / make test / make lint`.
- AFT integration tests skip when `aft` is not on `PATH`; set `AWP_TEST_AFT=/path/to/aft` to exercise them.

## AFT integration

AFT ([github.com/cortexkit/aft](https://github.com/cortexkit/aft)) is an optional Rust worker that AWP spawns as a long-lived subprocess. It implements file/bash/edit/symbol/grep tools in Rust with image/PDF inline, hashline byte verification, AST awareness via `oxc_engine`, callgraph, semantic search via ONNX embeddings, and a unified `tool_call` NDJSON protocol.

**Install** (any one): `npx @cortexkit/aft@latest setup` · `cargo install --path <aft-repo>/crates/aft --locked` · copy a pre-built binary to `~/.cargo/bin/aft` or anywhere on `PATH`.

**Verify**: `echo '{"id":"1","command":"echo","message":"hi"}' | aft` should print `[aft] started, pid N` then `{"id":"1","success":true,"message":"hi"}` then `[aft] stdin closed, shutting down`.

**Lifecycle**: one `aft` worker per AWP session. Started when the registry is built (first tool call), stopped by closing its stdin. If the worker exits unexpectedly mid-session, the wrapper falls back to Go for that call only and re-attempts the worker on the next call.

**Protocol**: NDJSON over stdin/stdout. Request: `{"id":"awp-N","command":"<name>","<param>":...}`. Response: `{"id":"awp-N","success":bool,"<data>":...}`. Lock-step per worker (Phase 1; mutating calls serialize on a per-worker mutex).

**Subprocess args**: `aft` is invoked with no CLI args (NDJSON standalone). Stderr captured to `$TMP/aft-<pid>.log` for crash triage.

**Persistent state**: `~/.local/share/cortexkit/aft/` (SQLite, callgraph, checkpoints, cache). Owned by AFT, not AWP — AWP never reads from or writes to this directory.

**Detection**: `exec.LookPath("aft")` at first tool registration. `AWP_NO_AFT=1` forces the Go path.

**Failure modes**:
- `aft` not on `PATH` → silent fallback to Go.
- `aft` crashes mid-session → next tool call returns `ErrAftUnavailable`; wrapper retries once, then falls back to Go.
- `aft` returns `success: false` with `code` + `message` → surfaces as Go error.

**Adding a new AFT-backed tool**: write `ReadXxxAft(cwd)` / `EditXxxAft(cwd)` etc. in `internal/agent-core/tools/aft_extension.go`, mirror the Go impl's `agentcore.Tool` interface exactly (`Name` / `Description` / `Parameters` / `Invoke`). Add to `tools.All(cwd)` registry branch when `useAft`. Keep the Go impl as the fallback.

**Why not cgo / FFI**: AFT is a Rust crate without a C-ABI binding. Subprocess + NDJSON is the documented transport and gives the same capability with ~225 lines of Go.

## Architectural rules

1. **No cycles.** `llm` is the leaf for vendors and the agent; `internal/llm/protocol` depends on nothing internal.
2. **Interfaces live with their consumer** — small surfaces, strong abstractions.
3. **Transport boundary**: `protocol` returns `*protocol.HTTPError`; `llm.Classify` upgrades to `*llm.Error` with a `Kind`.
4. **LLM layer is single-call.** `Core.StreamChat` is the only method; multi-turn, tools, history live in `internal/agent-core`.
5. **Streaming**: one channel of `StreamEvent{Chunk, Err}`. Tool calls ride the last chunk on `message_stop`. `RetryCore` retries only `StreamChat` on transient kinds (`RateLimit` / `Server` / `Network`).
6. **agent-server ↔ agent-client**: share `internal/agent-protocol/` only. agent-server owns the JSONL session store (`Store` / `Load` / `SessionMeta` / `NewID`).
7. **TUI**: depends on `internal/agent-client` + `internal/paths`. Never `internal/agent-server` or `internal/agent-core` directly.
8. **Adding a vendor**: implement `Provider` in `internal/llm/providers/<name>.go`, register in `cmd/awp/main.go loadAgent()`. Field-name quirks stay inside the provider.
9. **Adding a tool**: build `llm.ToolDef` with JSON Schema, register via `ag.WithTool(agentcore.Tool{...})`, parse `argsJSON` locally with `json.Unmarshal`. Wire up in `cmd/awp/main.go loadAgent()`.
10. **AFT backend**: see "AFT integration" section.

## TUI constraints

- **Use `charm.land/bubbles/v2` components exclusively.** `viewport` (chat, `SoftWrap = true`), `textarea`, `textinput` (single-line only), `spinner` (`spinner.MiniDot`, single per TUI), `help` + `key.Binding`, `progress`, `filepicker`, `list`, `table` / `tree` / `timer` / `paginator` as needed. If bubbles ships a component, use it.
- **Use each component's official default styles.** Call `DefaultStyles(isDark bool)` (or `DefaultStyles()` where no dark flag) and drop your own stylesheet. `spinner` / `viewport` / `progress` ship no `Styles` struct — leave at zero value.
- **Detect dark mode once at startup** via `lipgloss.HasDarkBackground(os.Stdin, os.Stdout)` and thread `lightDark := lipgloss.LightDark(hasDark)` into every `DefaultStyles(isDark)` call. Bubble Tea v2 handles downsampling; do not reach for `compat.AdaptiveColor`.
- **Custom styling only where bubbles ships no component** — currently the chat-pane role palette and the header strip. Express via `lightDark(lipgloss.Color("#dark"), lipgloss.Color("#light"))`.
- **Prefer `charm.land/lipgloss/v2` layout**: `Style.Width/Height/Border/Align/Padding/Margin(...)`, `lipgloss.JoinHorizontal/Vertical`, `lipgloss.Width(s)`.
- **No self-rolled layout primitives.** Do not reimplement wrap, indent, or width counters — lipgloss v2 is ANSI-aware and word-aware.
- **Header status composition**: split into brand + separator + status + track + session so outer `Style.Width(n).Render(...)` doesn't collapse inner ANSI escapes.
- **No inline ANSI escapes in render.** No `\x1b[...]` strings; render through lipgloss styles only.
- **No comments in TUI code.** Behavioural explanation lives in test names and commit messages.

## Go conventions

Cross-reference Go's [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) when in doubt. Golden Rules above cover context, errors, goroutine leaks, and sealed interfaces — this section adds only what they don't.

### Language & naming

- **MixedCaps / mixedCaps only.** No underscores, no `snake_case`, no `kebab-case`. Initialisms consistent case: `URL`, `ID`, `HTTP`, `SSE`, `JSON`, `NDJSON` — never `Url`, `Id`, `Http`.
- **No stutter.** `http.Server` not `http.HTTPServer`; `llm.Provider` not `llm.LLMProvider`. Don't import as an alias to "fix" stutter — rename the identifier.
- **Package names**: short, lowercase, singular (`bytes` not `bytesSet`, `tool` not `tools`).
- **Interfaces**: single-method → `-er` (`Reader`, `Writer`, `Closer`); two-method → dominant verb; three+ → concept name (`Tool`, `Provider`).
- **No `Get` prefix.** `name()` not `getName()`.
- **Constructors**: `NewType` or `NewTypeWithOptions`, never bare `New`.
- **Godoc on every exported identifier** starts with the name itself. Package godoc begins with `// Package foo provides ...`.
- **No commented-out code in commits.** Delete; git remembers.
- **No `TODO` / `FIXME` / `XXX` markers in committed code.** Open an issue or tracked follow-up instead.
- **Zero value useful.** `bytes.Buffer{}` is ready to use, `sync.Mutex{}` is unlocked. If your type can't do that, add an explicit `NewX`.

### Types & APIs

- **Accept interfaces, return concrete types.** Take the smallest interface you need; return struct or pointer.
- **Generics (Go 1.18+)**: use only when they make code safer or shorter, not for DRY. Minimal constraints — `any` over bespoke interfaces, `cmp.Ordered` / `slices.SortFunc` over hand-rolled.
- **No copy-pasted struct definitions** between packages. Cross-boundary shapes live in `internal/agent-protocol/`.
- **Optional config via functional options or config struct** — never constructor-argument explosion.

### Concurrency

- **`select { case ... case <-ctx.Done(): return }`** everywhere a blocking op can stall. Never bare `ch <- v` in a goroutine without an exit path.
- **Sender closes, receiver only ranges.** Never close a channel you didn't send on.
- **Buffer sizes are part of the contract.** `make(chan T, 1)` for single-shot signal, `make(chan T, 0)` for sync handoff, `make(chan T, N)` for expected burst.
- **sync.Mutex for state, channels for signals, atomic for counters.** Don't substitute one for the other.
- **`sync.Once` for one-time init.** `sync.Pool` for hot-path allocations (reset before `Put` — best-effort, may discard at GC). **`atomic.Value` / `atomic.Pointer[T]`** for lock-free reads of immutable snapshots.
- **Never embed a goroutine inside a struct literal** — `Foo{ run: someFunc }` is a leak waiting to happen. Take a function or `Start(ctx)` method.

### Resource management & I/O

- **`defer` for cleanup, in reverse order of acquisition.** Verify `Close` errors that matter via deferred func capturing named return.
- **`io.Closer` is the contract.** Multi-resource types expose a single `Close` that fans out.
- **HTTP client timeouts on every layer**: `Client.Timeout`, `Transport.DialContext.Timeout`, `Transport.TLSHandshakeTimeout`, `Transport.ResponseHeaderTimeout`.
- **Never `http.DefaultClient`** — no timeout. Construct per logical client.
- **Always `defer resp.Body.Close()`** before reading the body. `io.Copy(io.Discard, resp.Body)` before `Close` for connection reuse.
- **`io.LimitReader` on every untrusted body** — at the trust boundary, not the consumer.
- **`bufio.Scanner` with custom `Buffer`** when input line length is unbounded (`scanner.Buffer(make([]byte, 0, initial), max)` or `bufio.Reader.ReadBytes('\n')`).
- **`bytes.Buffer` / `strings.Builder` for concatenation** — never `s := ""; for ... { s += x }`.
- **`encoding/json` discipline**: struct tags, `json.RawMessage` for partial, `json.NewDecoder(r).Decode(&v)` for streams (not `io.ReadAll` + `Unmarshal`). `Decoder.DisallowUnknownFields()` at strict API boundaries.

### Slices, maps, strings

- **Pre-allocate with capacity** when knowable: `make([]T, 0, expectedLen)`.
- **Nil slice is valid empty**: `len(nil) == 0`, `for range nil`, `append(nil, x)` all work.
- **Never modify a map while ranging it** — copy keys first.
- **`slices` / `maps` (Go 1.21+)** for `Sort` / `Contains` / `Index` / `Keys` / `Values` / `Delete` / `Compact`.
- **`strconv` for primitive conversions** — `strconv.Itoa(n)` not `fmt.Sprintf("%d", n)`.
- **`strings` / `bytes`** over hand-rolled byte loops.

### Logging

- **`log/slog` only.** No `fmt.Println`, no `log.Printf`, no package-global `log`. Structured attributes via `slog.Info("...", "key", value)` — never `fmt.Sprintf` inside the message.
- **Levels**: Debug verbose; Info state changes; Warn recoverable oddities; Error failures the caller should see. Default Info.
- **Never log secrets.** Redact at source; if unavoidable, by key name (`"Authorization": "[REDACTED]"`).
- **Logger via parameter or context**, not package global (exception: `internal/log`).

### Module hygiene

- **Pin the toolchain in `go.mod`.** `go mod tidy` before commit. `go mod verify` in CI.
- **No `replace` directives for production deps.** `replace` is for forks / local-only debugging.
- **`go.work` only for local multi-module work.** Don't commit to a single-module repo.
- **Vendor only when reproducible builds are mandatory.**

### Performance & allocation

- **Measure first.** `pprof` and `go test -benchmem -bench` before "this is slow" rewrites.
- **Avoid allocations in hot paths.** Reuse `bytes.Buffer` via `Reset`, slices via `slices.Delete` (in place), scratch maps via `clear(m)`.
- **`sync.Pool` for repeated allocations** — `json.Encoder`, `bytes.Buffer`, `[]byte` scratch buffers.
- **Don't pre-optimize readability away.** A slow loop that runs once is fine. A clear allocation in a hot path is a bug.
