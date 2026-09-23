package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
)

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func Bash(cwd string, opts BashOptions) agentcore.Tool {
	shell := opts.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	timeoutSec := opts.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	return agentcore.ToolFunc{
		N: "bash",
		D: "Execute a shell command and return stdout+stderr combined. Working directory is the project root by default; pass 'workdir' to change. Do not put large temp files under /tmp — prefer a project-local scratch directory. Use the 'read' tool to look at file contents instead of `cat` when you only need a portion. Use 'edit' for changes, not `sed -i`. Avoid commands that block indefinitely (e.g. `tail -f`, `watch`) — give them a timeout.",
		P: requireIntentSchema("bash", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "REQUIRED. The shell command to run. Must be non-empty. Avoid commands that block indefinitely (use timeout instead)."},
				"timeout": map[string]any{"type": "integer", "description": "Timeout in SECONDS (default 60). Use 5–10 for quick probes; 120+ for builds/tests. If the command runs longer it is killed."},
				"workdir": map[string]any{"type": "string", "description": "Working directory for the command. Defaults to the project root. Pass an absolute path if you need to escape."},
			},
			"required": []string{"command"},
		}),
		Fn: func(parentCtx context.Context, argsJSON string) (string, error) {
			return agentcore.RunTool(parentCtx, argsJSON, func(parentCtx context.Context, args bashArgs) (string, error) {
				if trimSpace(args.Command) == "" {
					return "", fmt.Errorf("command is required")
				}
				timeout := args.Timeout
				if timeout <= 0 {
					timeout = timeoutSec
				}
				ctx, cancel := context.WithTimeout(parentCtx, time.Duration(timeout)*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, shell, "-c", opts.CommandPrefix+args.Command)
				cmd.Dir = cwd
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				err := cmd.Run()
				out := stdout.String()
				if stderr.Len() > 0 {
					out += "\n[stderr]\n" + stderr.String()
				}
				if ctx.Err() == context.DeadlineExceeded {
					return out, fmt.Errorf("command timed out after %ds", timeout)
				}
				if err != nil {
					return out, fmt.Errorf("command failed: %w", err)
				}
				return out, nil
			})
		},
	}
}
