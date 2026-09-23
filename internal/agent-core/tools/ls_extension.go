package tools

import (
	"context"
	"os"
	"strings"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
)

type lsArgs struct {
	Path  string `json:"path,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

func Ls(cwd string, opts FileOptions) agentcore.Tool {
	limit := opts.MaxLines
	if limit <= 0 {
		limit = defaultLsLimit
	}
	return agentcore.ToolFunc{
		N: "ls",
		D: "List directory entries. Default cwd. Directories have / suffix. The 'path' argument is optional; if omitted, lists the current working directory.",
		P: requireIntentSchema("ls", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "Optional. Directory to list (default cwd). Pass an explicit path to explore a specific directory; omitting it lists cwd."},
				"limit": map[string]any{"type": "integer", "description": "Max entries (default 500)"},
			},
		}),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agentcore.RunTool(context.TODO(), argsJSON, func(_ context.Context, args lsArgs) (string, error) {
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
			})
		},
	}
}
