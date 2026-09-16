# AGENTS.md

A small Go binary (`awp`) that talks to MiniMax's Anthropic-compatible
API. Two layers: `internal/llm` does single-call message processing;
`internal/agent` does multi-turn tool orchestration.

## Layout
- `main.go` — entrypoint, demo smoke.
- `internal/llm/` — message types (`Message`, `FinishReason`, `Model`,
  tool defs), `Provider` / `Protocol` interfaces, `Core`,
  `RetryCore`, error taxonomy, `Registry`.
- `internal/llm/protocol/` — transport (`HTTPRest`: HTTP + SSE). Leaf pkg.
- `internal/llm/providers/` — vendor adapters (`minimax` is Anthropic-compat).
- `internal/agent/` — multi-turn loop, tool execution, message history.

## Build & verify
- `go build ./...`
- `go vet ./...`
- `go test ./test/...`
- `go run .` — smoke (requires `MINIMAX_API_KEY`).
- `make build / make test / make lint` — see Makefile.
- `gofmt -w` before commit. CI should run `go vet ./...` and
  `go test -race ./test/...`.

## Architectural rules
1. `llm` owns the `Provider` / `Protocol` / `Registry` / `RetryCore`
   abstractions. Vendors depend on `llm`; agents depend on `llm`;
   `protocol` depends on nothing internal. No cycles.
2. Interfaces live with their consumer. `Provider` is consumed by `Core`
   so it lives in `internal/llm/core.go`, not in `providers/`. Do not
   move it. Keep the interface small (8 methods) — strong abstractions
   beat fat ones.
3. `internal/llm/protocol/` must not import `llm`. Transport errors
   cross the boundary as `*protocol.HTTPError`; `llm.Classify` upgrades
   them to `*llm.Error` with a `Kind` for retry decisions.
4. LLM layer = one call. Multi-turn loop, tool execution, message
   history, and capability validation live in `internal/agent/`. Do not
   put loop logic or model-capability checks in `llm/`.
5. Default provider is registered under `llm.DefaultProviderName`
   (`"minimax"`). New vendors plug into the `Registry` in `main.go`.
6. `RetryCore` retries `Chat` on transient errors (`RateLimit` /
   `Server` / `Network`). `StreamChat` is never retried — mid-stream
   errors propagate to the caller.
7. Tool-call accumulation across SSE deltas happens in
   `core.StreamChat`. The assembled `[]ToolCall` rides the LAST
   `StreamEvent{Chunk}` on `message_stop`. Mid-stream chunks carry
   only text / reasoning deltas.
8. Streaming errors cross via `protocol.StreamItem{Err}`; the
   `Core.StreamChat` goroutine upgrades them to `StreamEvent{Err}`.

## Code style
- No comments in code. Inline `//` and `/* */` are stripped on sight.
  Doc comments on exported boundaries are tolerated but kept terse.
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
   ConvertResponse / ConvertStreamChunk / Models`).
2. Register in `main.go`:
   `registry.MustRegister("<name>", providers.New<Name>(...))`.
3. Vendor field-name quirks are handled inside the Provider. Do not
   push them up to `Core` or the agent.

## Adding a tool
- Build `llm.ToolDef` with a JSON Schema `Parameters`.
- Register with `ag.RegisterTool(agent.Tool{Def, Func})` where
  `Func` takes `argsJSON string` and returns `(result string,
  err error)`.
- Parse args locally with `json.Unmarshal` into a typed struct.

## Open follow-ups
- Move capability validation from `Core.Chat` to `Agent.Run` so
  the LLM layer stays a pure transport.
- Second vendor (OpenAI / Anthropic-native) to exercise the
  Registry abstraction.
