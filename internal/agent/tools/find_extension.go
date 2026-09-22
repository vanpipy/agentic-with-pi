package tools

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

type findArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

func Find(cwd string, opts FileOptions) agent.Tool {
	limit := opts.MaxLines
	if limit <= 0 {
		limit = defaultFindLimit
	}
	return agent.ToolFunc{
		N: "find",
		D: "Find files matching a glob pattern. Default cwd. Limited results. REQUIRED: 'pattern' must be a non-empty glob (e.g. '*.go', 'cmd/**/*.ts').",
		P: requireIntentSchema("find", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "REQUIRED. Glob pattern, e.g. '*.go' or 'cmd/**/*.ts'."},
				"path":    map[string]any{"type": "string", "description": "Directory to search (default cwd)"},
				"limit":   map[string]any{"type": "integer", "description": "Max results (default 1000)"},
			},
			"required": []string{"pattern"},
		}),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agent.RunTool(context.TODO(), argsJSON, func(_ context.Context, args findArgs) (string, error) {
				if args.Path == "" {
					args.Path = cwd
				} else {
					args.Path = absPath(cwd, args.Path)
				}
				if args.Limit <= 0 {
					args.Limit = limit
				}

				var matches []string
				err := filepath.WalkDir(args.Path, func(path string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if d.IsDir() {
						return nil
					}
					ok, err := filepath.Match(args.Pattern, filepath.Base(path))
					if err != nil {
						return err
					}
					if ok {
						matches = append(matches, path)
					}
					return nil
				})
				if err != nil {
					return "", err
				}
				if len(matches) == 0 {
					return "(no matches)", nil
				}
				if len(matches) > args.Limit {
					matches = matches[:args.Limit]
				}
				return strings.Join(matches, "\n"), nil
			})
		},
	}
}
