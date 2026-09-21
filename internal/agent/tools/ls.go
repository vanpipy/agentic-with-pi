package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

func Ls(cwd string, opts FileOptions) agent.Tool {
	limit := opts.MaxLines
	if limit <= 0 {
		limit = defaultLsLimit
	}
	return agent.Tool{
		Name:        "ls",
		Description: "List directory entries. Default cwd. Directories have / suffix. The 'path' argument is optional; if omitted, lists the current working directory.",
		Parameters: requireIntentSchema("ls", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "Optional. Directory to list (default cwd). Pass an explicit path to explore a specific directory; omitting it lists cwd."},
				"limit": map[string]any{"type": "integer", "description": "Max entries (default 500)"},
			},
		}),
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			if err := requireIntentOrError(argsJSON); err != nil {
				return "", err
			}
			var args struct {
				Path   string `json:"path,omitempty"`
				Limit  int    `json:"limit,omitempty"`
				Intent string `json:"intent"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			if args.Path == "" {
				args.Path = cwd
			} else {
				args.Path = absPath(cwd, args.Path)
			}
			if args.Limit <= 0 {
				args.Limit = limit
			}

			entries, err := os.ReadDir(args.Path)
			if err != nil {
				return "", err
			}
			if len(entries) > args.Limit {
				entries = entries[:args.Limit]
			}
			lines := make([]string, 0, len(entries))
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				lines = append(lines, name)
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}