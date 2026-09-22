package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func ReadFile(cwd string, opts FileOptions) agent.Tool {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultReadMaxBytes
	}
	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = defaultReadMaxLines
	}
	return agent.ToolFunc{
		N: "read",
		D: fmt.Sprintf("Read a file's contents. Output is truncated to %d lines or %dKB (whichever hits first); for larger files use offset+limit to read a specific range. Prefer this over `cat`/`head`/`sed` so you don't blow the context window. The 'path' is required and must be an absolute path or relative to the project root. Do not re-read files you already have in context — the second read returns the same bytes.", maxLines, maxBytes/1024),
		P: requireIntentSchema("read", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "REQUIRED. Path to file (relative or absolute). Omit only if you intentionally want to discover the working directory."},
				"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-indexed)"},
				"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read"},
			},
			"required": []string{"path"},
		}),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agent.RunTool(context.TODO(), argsJSON, func(_ context.Context, args readArgs) (string, error) {
				if trimSpace(args.Path) == "" {
					return "", fmt.Errorf("path is required (use the ls tool to discover files in a directory)")
				}
				if args.Limit <= 0 {
					args.Limit = maxLines
				}
				fullPath := absPath(cwd, args.Path)
				info, err := os.Stat(fullPath)
				if err != nil {
					return "", err
				}
				if info.IsDir() {
					return "", fmt.Errorf("%s is a directory (use the ls tool to list its contents, not read)", fullPath)
				}
				data, err := os.ReadFile(fullPath)
				if err != nil {
					return "", err
				}
				content := string(data)
				if args.Offset > 0 || len(content) > maxBytes {
					lines := strings.Split(content, "\n")
					start := args.Offset
					if start < 1 {
						start = 1
					}
					if start > len(lines) {
						return fmt.Sprintf("(file has only %d lines)", len(lines)), nil
					}
					end := start + args.Limit - 1
					if end > len(lines) {
						end = len(lines)
					}
					content = strings.Join(lines[start-1:end], "\n")
				}
				out, _ := truncate(content, maxBytes)
				return out, nil
			})
		},
	}
}
