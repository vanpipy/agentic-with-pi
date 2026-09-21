package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vanpiyp/awp/internal/agent"
)

func WriteFile(cwd string) agent.Tool {
	return agent.Tool{
		Name:        "write",
		Description: "Write content to a file (overwrites existing content). Creates parent directories as needed. REQUIRED: both 'path' and 'content' must be provided.",
		Parameters: requireIntentSchema("write", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "REQUIRED. Path to file to write."},
				"content": map[string]any{"type": "string", "description": "REQUIRED. Full file content to write."},
			},
			"required": []string{"path", "content"},
		}),
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			if err := requireIntentOrError(argsJSON); err != nil {
				return "", err
			}
			var args struct {
				Path    string `json:"path"`
				Content string `json:"content"`
				Intent  string `json:"intent"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			fullPath := absPath(cwd, args.Path)
			if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(fullPath, []byte(args.Content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), fullPath), nil
		},
	}
}