package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/agent/tools"
	"github.com/vanpiyp/awp/internal/client-sdk"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
	"github.com/vanpiyp/awp/internal/log"
	"github.com/vanpiyp/awp/internal/server"
	"github.com/vanpiyp/awp/internal/storage"
	"github.com/vanpiyp/awp/internal/transport"
	"github.com/vanpiyp/awp/internal/tui"
)

const usage = `awp — agent with pi

Usage:
  awp [--max-turns N]               launch TUI (recommended, requires terminal)
  awp serve [--max-turns N]         run as background server (Unix socket)
  awp connect <prompt> [--max-turns N]   spawn server + send prompt + print events
  awp resume <session_id> [new_prompt]   resume session; with new_prompt,
                                          picks up the last compaction summary.
                                          MaxTurns is a fresh per-call budget,
                                          not a sliding window across calls.
                                          Use --max-turns to raise it.
  awp help                         show this message

Server socket: $AWP_SOCKET (default ~/.awp/runtime/awp.sock)
`

func main() {
	maxTurns := extractMaxTurnsFlag(os.Args)
	args := stripMaxTurnsFlag(os.Args)
	if len(args) > 1 {
		switch args[1] {
		case "serve":
			runServe(maxTurns)
			return
		case "connect":
			runConnect(args[2:], maxTurns)
			return
		case "resume":
			runResume(args[2:], maxTurns)
			return
		case "demo":
			runDemo()
			return
		case "help", "-h", "--help":
			fmt.Print(usage)
			return
		}
	}
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}
}

func extractMaxTurnsFlag(argv []string) int {
	for i := 1; i < len(argv); i++ {
		if argv[i] == "--max-turns" {
			if i+1 < len(argv) {
				if n, err := strconv.Atoi(argv[i+1]); err == nil && n > 0 {
					return n
				}
			}
		}
		if v, ok := strings.CutPrefix(argv[i], "--max-turns="); ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

func stripMaxTurnsFlag(argv []string) []string {
	out := argv[:1]
	for i := 1; i < len(argv); {
		if argv[i] == "--max-turns" && i+1 < len(argv) {
			i += 2
			continue
		}
		if v, ok := strings.CutPrefix(argv[i], "--max-turns="); ok {
			_ = v
			i++
			continue
		}
		out = append(out, argv[i])
		i++
	}
	return out
}

func setupLog() {
	cfg := log.DefaultConfig()
	cfg.Level = slog.LevelDebug
	if err := log.Setup(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "log setup:", err)
	}
}

func loadAgent() *agent.Agent {
	cfg, err := agent.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		fmt.Fprintln(os.Stderr, "Check ~/.awp/config.yaml.")
		os.Exit(1)
	}

	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "MINIMAX_API_KEY is required.")
		os.Exit(1)
	}

	registry := llm.NewRegistry()
	registry.MustRegisterDefault(providers.NewMiniMaxProvider(apiKey))
	provider, _ := registry.Default()
	base := llm.NewCore(provider, protocol.NewHTTPRest())
	throttled := llm.NewRateLimitedCore(base, llm.RateLimitConfig{RatePerSec: 5, Burst: 2})
	core := llm.NewRetryCore(throttled, llm.DefaultRetryConfig())

	cwd, _ := os.Getwd()
	ag := agent.NewAgent(core).
		WithModel(llm.Model{
			ID:                cfg.Model,
			SupportsTool:      true,
			SupportsStreaming: true,
			SupportsReasoning: true,
		})
	ag.SetSystemPrompts(`You are a coding assistant that operates a local repository through file and shell tools.

Tool usage rules:
- Every tool call MUST include the required parameters. If a tool returns an error like "path is required" or "command is required", DO NOT retry the same empty call — read the error, fix the argument, then retry once. Repeated identical errors will cause the agent to abort.
- If a tool call fails, examine the error message and adjust your next call. Do not loop on the same mistake.
- For 'read', pass an explicit file path. Use 'ls' or 'find' to discover files first.
- For 'bash', pass a non-empty command string.
- For 'edit', old_text must match exactly once unless replace_all=true.
- Do not re-read files you already have the contents of.

Planning rules:
- Think briefly before acting. State the plan, then execute.
- Prefer minimal, focused tool calls over broad exploration.
- Avoid running the same command twice — its output is already in your history.
- Stop and report to the user when the task is done, rather than continuing to explore.`)

	ag.WithTool(tools.ReadFile(cwd, tools.FileOptions{}))
	ag.WithTool(tools.WriteFile(cwd))
	ag.WithTool(tools.EditFile(cwd))
	ag.WithTool(tools.Bash(cwd, tools.BashOptions{}))
	ag.WithTool(tools.Grep(cwd, tools.FileOptions{}))
	ag.WithTool(tools.Find(cwd, tools.FileOptions{}))
	ag.WithTool(tools.Ls(cwd, tools.FileOptions{}))

	return ag
}

func runServe(maxTurns int) {
	setupLog()
	ag := loadAgent()
	if maxTurns > 0 {
		ag.WithMaxTurns(maxTurns)
	}

	socket := storage.SocketPath()
	srv, err := server.New(ag, socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "server start:", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5_000_000_000)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	fmt.Printf("awp-server listening on %s\n", socket)
	if err := srv.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}

func runConnect(args []string, maxTurns int) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: awp connect <prompt>")
		os.Exit(1)
	}
	prompt := args[0]

	setupLog()

	socket := storage.SocketPath()
	if !transport.IsRunning(socket) {
		fmt.Fprintln(os.Stderr, "server not running, spawning...")
		if err := spawnServer(); err != nil {
			fmt.Fprintln(os.Stderr, "spawn failed:", err)
			os.Exit(1)
		}
		if err := waitForServer(socket, 5*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, "wait failed:", err)
			os.Exit(1)
		}
	}

	c, err := client_sdk.Dial(socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		os.Exit(1)
	}

	fmt.Printf("=== awp connect ===\nsocket: %s\n\n", socket)

	events, err := c.Prompt(context.Background(), prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prompt:", err)
		os.Exit(1)
	}

	for ev := range events {
		printServerEvent(ev.Kind, ev.Data)
	}
}

func runResume(args []string, maxTurns int) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: awp resume <session_id> [new_prompt]")
		os.Exit(1)
	}
	sessionID := args[0]
	newPrompt := ""
	if len(args) > 1 {
		newPrompt = strings.Join(args[1:], " ")
	}

	setupLog()

	socket := storage.SocketPath()
	if !transport.IsRunning(socket) {
		fmt.Fprintln(os.Stderr, "server not running, spawning...")
		if err := spawnServer(); err != nil {
			fmt.Fprintln(os.Stderr, "spawn failed:", err)
			os.Exit(1)
		}
		if err := waitForServer(socket, 5*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, "wait failed:", err)
			os.Exit(1)
		}
	}

	c, err := client_sdk.Dial(socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer c.Close()

	fmt.Printf("=== awp resume ===\nsocket: %s\nsession: %s\n\n", socket, sessionID)

	if newPrompt == "" {
		events, err := c.Resume(context.Background(), sessionID)
		if err != nil {
			fmt.Fprintln(os.Stderr, "resume:", err)
			os.Exit(1)
		}
		for ev := range events {
			printServerEvent(ev.Kind, ev.Data)
		}
		return
	}

	events, err := c.PromptWithSessionID(context.Background(), newPrompt, sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prompt:", err)
		os.Exit(1)
	}
	for ev := range events {
		printServerEvent(ev.Kind, ev.Data)
	}
}

func spawnServer() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	return cmd.Start()
}

func waitForServer(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if transport.IsRunning(socketPath) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not start within %v", timeout)
}

func runDemo() {
	fmt.Print(usage)
	if os.Getenv("AWP_DEMO") != "1" {
		return
	}

	setupLog()
	ag := loadAgent()

	fmt.Println("=== awp demo ===")
	fmt.Printf("model: %s\n", ag.Model.ID)

	prompts := []string{
		"List the files in the current directory.",
		"Read the first 30 lines of internal/llm/retry.go and summarize it briefly.",
	}

	for _, p := range prompts {
		fmt.Printf("\n=== prompt: %s ===\n", p)
		for ev := range ag.RunStream(context.Background(), p) {
			printAgentEvent(ev)
		}
	}
}

func printAgentEvent(ev agent.Event) {
	switch ev.Category {
	case agent.EventThoughtStart:
		fmt.Println("--- turn ---")
	case agent.EventThoughtChunk:
		if ev.Reasoning != "" {
			fmt.Printf("[thinking] %s", ev.Reasoning)
		}
		if ev.Content != "" {
			fmt.Printf("\n[text] %s", ev.Content)
		}
	case agent.EventThoughtEnd:
		fmt.Println()
	case agent.EventTool:
		fmt.Printf("\n[tool] %s(%s)\n", ev.ToolName, ev.ToolArgs)
	case agent.EventObserve:
		if ev.ToolError != "" {
			fmt.Printf("[observe] error: %s\n", ev.ToolError)
		} else {
			fmt.Printf("[observe] %s\n", ev.ToolResult)
		}
	case agent.EventFinalAnswer:
		fmt.Println("\n=== final answer ===")
		fmt.Println(ev.Content)
	case agent.EventError:
		fmt.Fprintf(os.Stderr, "\n[error] %s\n", ev.ToolError)
	}
}

func printServerEvent(eventKind string, data []byte) {
	switch eventKind {
	case "session_started":
		var d struct {
			SessionID string `json:"session_id"`
		}
		json.Unmarshal(data, &d)
		fmt.Printf("[session] %s\n", d.SessionID)
	case "thought_start":
		fmt.Println("--- turn ---")
	case "thought_chunk":
		var d struct {
			Reasoning string `json:"reasoning"`
			Content   string `json:"content"`
		}
		json.Unmarshal(data, &d)
		if d.Reasoning != "" {
			fmt.Printf("[thinking] %s", d.Reasoning)
		}
		if d.Content != "" {
			fmt.Printf("\n[text] %s", d.Content)
		}
	case "thought_end":
		fmt.Println()
	case "tool":
		var d struct {
			Name string `json:"name"`
			Args string `json:"args"`
		}
		json.Unmarshal(data, &d)
		fmt.Printf("\n[tool] %s(%s)\n", d.Name, d.Args)
	case "observe":
		var d struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}
		json.Unmarshal(data, &d)
		if d.Error != "" {
			fmt.Printf("[observe] error: %s\n", d.Error)
		} else {
			fmt.Printf("[observe] %s\n", d.Result)
		}
	case "final_answer":
		var d struct {
			Content string `json:"content"`
		}
		json.Unmarshal(data, &d)
		fmt.Println("\n=== final answer ===")
		fmt.Println(d.Content)
	case "error":
		var d struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &d)
		fmt.Fprintf(os.Stderr, "\n[error] %s\n", d.Error)
	case "session_resumed":
		var d struct {
			EventCount int `json:"event_count"`
		}
		json.Unmarshal(data, &d)
		fmt.Printf("[session] resumed (%d events)\n", d.EventCount)
	case "pong":
		fmt.Println("[pong]")
	default:
		fmt.Printf("[%s] %s\n", eventKind, string(data))
	}
}
