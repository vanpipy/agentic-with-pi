package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

func Find(cwd string, opts FileOptions) agent.Tool {
	limit := opts.MaxLines
	if limit <= 0 {
		limit = defaultFindLimit
	}
	return agent.Tool{
		Name:        "find",
		Description: "Find files matching a glob pattern. Default cwd. Limited results. REQUIRED: 'pattern' must be a non-empty glob (e.g. '*.go', 'cmd/**/*.ts').",
		Parameters: requireIntentSchema("find", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "REQUIRED. Glob pattern, e.g. '*.go' or 'cmd/**/*.ts'."},
				"path":    map[string]any{"type": "string", "description": "Directory to search (default cwd)"},
				"limit":   map[string]any{"type": "integer", "description": "Max results (default 1000)"},
			},
			"required": []string{"pattern"},
		}),
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			if err := requireIntentOrError(argsJSON); err != nil {
				return "", err
			}
			var args struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path,omitempty"`
				Limit   int    `json:"limit,omitempty"`
				Intent  string `json:"intent"`
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
		},
	}
}