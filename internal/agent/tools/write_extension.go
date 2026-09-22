package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanpiyp/awp/internal/agent"
)

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteFile(cwd string) agent.Tool {
	return agent.ToolFunc{
		N: "write",
		D: "Write content to a file (overwrites any existing content). Creates parent directories as needed. Use this only when creating a brand-new file or replacing the entire contents; for partial edits use 'edit' instead — it is safer and faster. The 'path' must be absolute or relative to the project root.",
		P: requireIntentSchema("write", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "REQUIRED. Path to file to write."},
				"content": map[string]any{"type": "string", "description": "REQUIRED. Full file content to write."},
			},
			"required": []string{"path", "content"},
		}),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agent.RunTool(context.TODO(), argsJSON, func(_ context.Context, args writeArgs) (string, error) {
				fullPath := absPath(cwd, args.Path)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
					return "", err
				}
				if err := os.WriteFile(fullPath, []byte(args.Content), 0o644); err != nil {
					return "", err
				}
				return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), fullPath), nil
			})
		},
	}
}
