package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/tools"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/ipc"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/providers"
	"github.com/vanpiyp/awp/internal/log"
	"github.com/vanpiyp/awp/internal/paths"
	"github.com/vanpiyp/awp/internal/tui"
)

const usage = `awp — agent with pi

Usage:
  awp                                              launch TUI (recommended, requires terminal)
  awp serve --socket <path>                        run as background server on a per-client socket
  awp connect --connect-pid <pid> <prompt>         send prompt to a running TUI/server instance
  awp resume --connect-pid <pid> <session_id> [new_prompt]
                                                  resume session on a running instance
  awp help                                         show this message

Process model: each awp instance owns its own socket and (for TUI) the
server it spawned. Use --connect-pid to attach awp connect/awp resume
to an existing instance. The PID is printed by 'awp' on startup.
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "serve":
			runServe(os.Args[2:])
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

func loadAgent() *agentcore.Agent {
	cfg, err := agentcore.LoadConfig()
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
	ag := agentcore.NewAgent(core).
		WithModel(llm.Model{
			ID:                cfg.Model,
			SupportsTool:      true,
			SupportsStreaming: true,
			SupportsReasoning: true,
		})
	for _, t := range tools.All(cwd) {
		ag.WithTool(t)
	}
	ag.SetSystemPrompts(`You are a coding assistant that operates a local repository through file and shell tools.

Tool usage rules:
- Every tool call has a strongly recommended 'intent' string. State in one short sentence why you are making the call. The intent is shown in the UI alongside the call so future-you can scan it to find calls you actually meant to make. If you omit it, the call still runs; the UI falls back to '<tool_name> <args>'.
- Every tool call also has a REQUIRED set of domain-specific fields (e.g. 'path' for read, 'command' for bash, 'old_text' for edit). A tool that omits its required field returns an error like "path is required" or "command is required" and the call counts as a failed attempt.
- If a tool returns an error, read the error and adjust the next call. Do not retry the same empty/malformed call — repeated identical errors will cause the agent to abort.
- If you realize a tool call you made was malformed (wrong tool, wrong argument type, semantic error), call the 'invalid' tool with the tool name and a short reason. This records the mistake without aborting the loop.
- Do not re-read files you already have the contents of.
- For 'read', pass an explicit file path. Use 'ls' or 'find' to discover files first.
- For 'bash', pass a non-empty command string.
- For 'edit', old_text must match exactly once unless replace_all=true.

Planning rules:
- Think briefly before acting. State the plan, then execute.
- Prefer minimal, focused tool calls over broad exploration.
- Avoid running the same command twice — its output is already in your history.
- Stop and report to the user when the task is done, rather than continuing to explore.`)

	return ag
}

func runServe(args []string) {
	socket := parseSocketFlag(args)
	if socket == "" {
		socket = paths.ClientSocketPath(os.Getpid())
	}
	setupLog()
	ag := loadAgent()

	srv, err := agentserver.New(ag, socket)
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

	fmt.Printf("awp-server pid=%d listening on %s\n", os.Getpid(), socket)
	if err := srv.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}

func parseSocketFlag(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--socket" {
			return args[i+1]
		}
	}
	return ""
}

func parseConnectPIDFlag(args []string) (int, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--connect-pid" {
			pid, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --connect-pid %q: %v\n", args[i+1], err)
				os.Exit(1)
			}
			return pid, true
		}
	}
	return 0, false
}

func runConnect(args []string) {
	connectPID, ok := parseConnectPIDFlag(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: awp connect --connect-pid <pid> <prompt>")
		os.Exit(1)
	}
	promptIdx := indexAfterFlag(args, "--connect-pid")
	if promptIdx >= len(args) {
		fmt.Fprintln(os.Stderr, "usage: awp connect --connect-pid <pid> <prompt>")
		os.Exit(1)
	}
	prompt := strings.Join(args[promptIdx:], " ")

	setupLog()

	socket := paths.ClientSocketPath(connectPID)
	if !ipc.IsRunning(socket) {
		fmt.Fprintf(os.Stderr, "no server on %s (connect-pid=%d)\n", socket, connectPID)
		os.Exit(1)
	}

	fmt.Printf("=== awp connect ===\nsocket: %s\n\n", socket)

	events, err := agentclient.SendPrompt(context.Background(), socket, "", prompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prompt:", err)
		os.Exit(1)
	}

	for ev := range events {
		printServerEvent(ev.Kind, ev.Data)
	}
}

func indexAfterFlag(args []string, flag string) int {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return i + 1
		}
	}
	return len(args)
}

func runResume(args []string) {
	connectPID, ok := parseConnectPIDFlag(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "usage: awp resume --connect-pid <pid> <session_id> [new_prompt]")
		os.Exit(1)
	}
	sessionIDIdx := indexAfterFlag(args, "--connect-pid")
	if sessionIDIdx >= len(args) {
		fmt.Fprintln(os.Stderr, "usage: awp resume --connect-pid <pid> <session_id> [new_prompt]")
		os.Exit(1)
	}
	sessionID := args[sessionIDIdx]
	newPrompt := ""
	if sessionIDIdx+1 < len(args) {
		newPrompt = strings.Join(args[sessionIDIdx+1:], " ")
	}

	setupLog()

	socket := paths.ClientSocketPath(connectPID)
	if !ipc.IsRunning(socket) {
		fmt.Fprintf(os.Stderr, "no server on %s (connect-pid=%d)\n", socket, connectPID)
		os.Exit(1)
	}

	c, err := agentclient.Dial(socket)
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

	events, err := agentclient.SendPrompt(context.Background(), socket, sessionID, newPrompt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prompt:", err)
		os.Exit(1)
	}
	for ev := range events {
		printServerEvent(ev.Kind, ev.Data)
	}
	_ = c
}

func spawnServer(socket string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(exe, "serve", "--socket", socket)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func waitForServer(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ipc.IsRunning(socketPath) {
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

func printAgentEvent(ev agentcore.Event) {
	switch ev.Category {
	case agentcore.EventThoughtStart:
		fmt.Println("--- turn ---")
	case agentcore.EventThoughtChunk:
		if ev.Reasoning != "" {
			fmt.Printf("[thinking] %s", ev.Reasoning)
		}
		if ev.Content != "" {
			fmt.Printf("\n[text] %s", ev.Content)
		}
	case agentcore.EventThoughtEnd:
		fmt.Println()
	case agentcore.EventTool:
		fmt.Printf("\n[tool] %s(%s)\n", ev.ToolName, ev.ToolArgs)
	case agentcore.EventObserve:
		if ev.ToolError != "" {
			fmt.Printf("[observe] error: %s\n", ev.ToolError)
		} else {
			fmt.Printf("[observe] %s\n", ev.ToolResult)
		}
	case agentcore.EventFinalAnswer:
		fmt.Println("\n=== final answer ===")
		fmt.Println(ev.Content)
	case agentcore.EventError:
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
