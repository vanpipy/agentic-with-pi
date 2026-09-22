package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/vanpiyp/awp/internal/agent"
)

type bashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func Bash(cwd string, opts BashOptions) agent.Tool {
	shell := opts.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	timeoutSec := opts.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	return agent.ToolFunc{
		N: "bash",
		D: "Execute a shell command and return stdout+stderr. Working directory is cwd. REQUIRED: the 'command' argument must always be a non-empty string.",
		P: requireIntentSchema("bash", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string", "description": "REQUIRED. The shell command to run. Must be non-empty."},
				"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 60)"},
			},
			"required": []string{"command"},
		}),
		Fn: func(parentCtx context.Context, argsJSON string) (string, error) {
			return agent.RunTool(parentCtx, argsJSON, func(parentCtx context.Context, args bashArgs) (string, error) {
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
