package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
)

type EditOp struct {
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

type editArgs struct {
	Path  string   `json:"path"`
	Edits []EditOp `json:"edits"`
}

func EditFile(cwd string) agentcore.Tool {
	return agentcore.ToolFunc{
		N: "edit",
		D: "Apply edits to a file. Each edit replaces 'old_text' with 'new_text' in order. By default 'old_text' must match exactly once in the file; pass 'replace_all=true' to replace every occurrence. Read the file first and copy the 'old_text' exactly (including whitespace) — whitespace mismatch is the #1 cause of edit failures. If 'old_text' does not match, re-read the file before retrying; the file may have changed since your last read.",
		P: requireIntentSchema("edit", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "REQUIRED. Path to file to edit."},
				"edits": map[string]any{
					"type":        "array",
					"description": "REQUIRED. Non-empty array of {old_text, new_text} pairs.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"old_text":    map[string]any{"type": "string", "description": "Text to find (must be unique unless replace_all=true)"},
							"new_text":    map[string]any{"type": "string", "description": "Replacement text"},
							"replace_all": map[string]any{"type": "boolean", "default": false, "description": "Replace every occurrence (otherwise old_text must match exactly once)"},
						},
						"required": []string{"old_text", "new_text"},
					},
				},
			},
			"required": []string{"path", "edits"},
		}),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agentcore.RunTool(context.TODO(), argsJSON, func(_ context.Context, args editArgs) (string, error) {
				if len(args.Edits) == 0 {
					return "", errors.New("edits array must not be empty")
				}
				fullPath := absPath(cwd, args.Path)
				data, err := os.ReadFile(fullPath)
				if err != nil {
					return "", err
				}
				content := string(data)
				for i, op := range args.Edits {
					if !op.ReplaceAll && strings.Count(content, op.OldText) != 1 {
						return "", fmt.Errorf("edit %d: old_text must match exactly once (or set replace_all=true)", i)
					}
					if op.ReplaceAll {
						content = strings.ReplaceAll(content, op.OldText, op.NewText)
					} else {
						content = strings.Replace(content, op.OldText, op.NewText, 1)
					}
				}
				if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
					return "", err
				}
				return fmt.Sprintf("applied %d edits to %s", len(args.Edits), fullPath), nil
			})
		},
	}
}
