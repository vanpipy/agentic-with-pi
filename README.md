# AWP — AgenticWithPI

LLM agent over JSON-RPC 2.0 (Unix socket) with a bubbletea TUI.

## Scope

This project covers four layers of an AI coding system:

- **ai-core** — LLM provider abstraction, streaming protocol, retry. Foundational plumbing used by everything below.
- **ai-agent** — *current focus*. ReAct loop, tool registry, session JSONL, compaction, AFT integration. The agent in this repo.
- **ai-memory** — session replay, cross-session resume, compaction summaries, future vector recall.
- **ai-orchestration** — subagent dispatch, batch tool calls, swarm coordination. Not started yet.

## Features

- **Single Go binary** (`awp`) — agent + server + TUI + JSON-RPC dispatch, all in one.
- **Session JSONL** — every LLM turn, tool call, observation, and compaction is
  persisted to `~/.awp/logs/sessions/<session_id>.jsonl` for replay/resume.
- **AFT-backed tools** — file/bash/edit tools can be offloaded to
  [`aft`](https://github.com/cortexkit/aft) (a separate Rust worker process)
  for image/PDF inline, hashline-verified edits, AST-aware operations, and
  callgraph/semantic search. **Optional** — falls back to built-in Go
  implementations when `aft` is not on `PATH`.
- **LLM interface log** — every request/response is logged to
  `$TMP/awp-llm.log` for debugging. Override with `AWP_LLM_LOG`.

## Install

### AWP binary

```bash
go build -o awp ./cmd/awp
# or
make build
```

### AFT backend (optional)

When `aft` is on `PATH`, AWP uses it for file/bash/edit tools. Without it,
AWP silently falls back to the built-in Go implementations.

Pick one of:

```bash
# Option A — official installer (downloads a release binary into PATH,
# also configures any detected harnesses). Recommended for end users.
npx @cortexkit/aft@latest setup

# Option B — build from source (release build). Use this if you have the
# aft repo checked out and want the latest source-tree changes.
cargo install --path /path/to/aft/crates/aft --locked

# Option C — copy a pre-built binary you downloaded elsewhere
cp /path/to/aft ~/.cargo/bin/aft
```

Verify it works:

```bash
which aft                          # should print a path
echo '{"id":"1","command":"echo","message":"hi"}' | aft
# expected: [aft] started, pid N
#           {"id":"1","success":true,"message":"hi"}
#           [aft] stdin closed, shutting down
```

To **disable** AFT integration entirely (always use Go impls), set
`AWP_NO_AFT=1` in the environment.

## Usage

```bash
# Interactive TUI (default mode)
./awp

# Headless: send one prompt to a running TUI/serve
./awp connect --connect-pid <pid> "<prompt>"

# Headless: serve on a per-instance socket
./awp serve --socket /tmp/awp.sock

# Resume / extend a session
./awp resume --connect-pid <pid> <session_id> "<optional new prompt>"
```

## Architecture

- `cmd/awp` — entrypoint.
- `internal/agent` + `internal/agent/tools/` — ReAct loop, tool registry.
- `internal/llm` + `internal/llm/protocol` + `internal/llm/providers/` —
  provider abstraction (`Provider` / `Protocol` / `Core` / `RetryCore` /
  `Registry`). Default vendor is `minimax` (Anthropic-compatible).
- `internal/server` — accept loop, JSON-RPC 2.0 dispatch, JSONL session
  store.
- `internal/protocol` — JSON-RPC 2.0 message types, shared by server and
  client SDK.
- `internal/client-sdk` — Go client SDK.
- `internal/storage` — XDG paths. `internal/log` — slog rotation.
  `internal/transport` — Unix domain socket.
- `internal/tui` — bubbletea frontend.

## See also

- [`AGENTS.md`](./AGENTS.md) — architectural rules, layout, code style, TUI
  constraints, tool-extension conventions.
