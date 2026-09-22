# AWP — AgenticWithPI

LLM agent over JSON-RPC 2.0 (Unix socket) with a bubbletea TUI.

## Scope

This project covers four layers of an AI coding system:

- **ai-core** — LLM provider abstraction, streaming protocol, retry. Foundational plumbing used by everything below.
- **ai-agent** — *current focus*. ReAct loop, tool registry, session JSONL, compaction, AFT integration. The agent in this repo.
- **ai-memory** — session replay, cross-session resume, compaction summaries, future vector recall.
- **ai-orchestration** — subagent dispatch, batch tool calls, swarm coordination. Not started yet.

## Features

- **Single Go binary** (`awp`) — agent + server + TUI + JSON-RPC dispatch.
- **Session JSONL** — every LLM turn, tool call, observation, and compaction persisted to `~/.awp/logs/sessions/<session_id>.jsonl`.
- **AFT-backed tools** (optional) — file/bash/edit can be offloaded to the [aft](https://github.com/cortexkit/aft) Rust worker for image/PDF inline, hashline-verified edits, AST-aware operations, callgraph and semantic search. Falls back to built-in Go implementations when `aft` is not on `PATH`.
- **LLM interface log** — every request/response logged to `$TMP/awp-llm.log` for debugging.

## Install

```bash
go build -o awp ./cmd/awp        # AWP
npx @cortexkit/aft@latest setup  # AFT (optional, enables the AFT-backed tools)
```

See [`AGENTS.md`](./AGENTS.md) for the full install matrix, AFT integration
details, architectural rules, and tool-extension conventions.
