package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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
  awp                              launch TUI (recommended, requires terminal)
  awp serve                        run as background server (Unix socket)
  awp connect <prompt>             spawn server + send prompt + print events
  awp resume <session_id>          resume session + replay events
  awp help                         show this message

Server socket: $AWP_SOCKET (default ~/.awp/runtime/awp.sock)
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			runServe()
			return
		case "connect":
			runConnect(os.Args[2:])
			return
		case "resume":
			runResume(os.Args[2:])
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
	core := llm.NewRetryCore(llm.NewCore(provider, protocol.NewHTTPRest()), llm.DefaultRetryConfig())

	cwd, _ := os.Getwd()
	ag := agent.NewAgent(core).
		WithModel(llm.Model{
			ID:                cfg.Model,
			SupportsTool:      true,
			SupportsStreaming: true,
			SupportsReasoning: true,
		})
	ag.SetSystemPrompts("You are a coding assistant. Use file tools to read/write/edit code and bash to run commands. Think step by step before acting.")

	ag.WithTool(tools.ReadFile(cwd, tools.FileOptions{}))
	ag.WithTool(tools.WriteFile(cwd))
	ag.WithTool(tools.EditFile(cwd))
	ag.WithTool(tools.Bash(cwd, tools.BashOptions{}))
	ag.WithTool(tools.Grep(cwd, tools.FileOptions{}))
	ag.WithTool(tools.Find(cwd, tools.FileOptions{}))
	ag.WithTool(tools.Ls(cwd, tools.FileOptions{}))

	return ag
}

func runServe() {
	setupLog()
	ag := loadAgent()

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

func runConnect(args []string) {
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

func runResume(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: awp resume <session_id>")
		os.Exit(1)
	}
	sessionID := args[0]

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

	events, err := c.Resume(context.Background(), sessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resume:", err)
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
