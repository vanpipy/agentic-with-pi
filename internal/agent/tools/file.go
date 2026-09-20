package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

const (
	defaultReadMaxBytes = 256 * 1024
	defaultReadMaxLines = 2000
)

type FileOptions struct {
	MaxBytes int
	MaxLines  int
}

func absPath(cwd, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(cwd, p))
}

func truncate(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false
	}
	return s[:maxBytes] + "\n... [truncated]", true
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
	return agent.Tool{
		Name:        "read",
		Description: fmt.Sprintf("Read file contents. Output is truncated to %d lines or %dKB (whichever hits first). Use offset/limit for large files. REQUIRED: the 'path' argument must always be provided.", maxLines, maxBytes/1024),
		Parameters: requireIntentSchema("read", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "REQUIRED. Path to file (relative or absolute). Omit only if you intentionally want to discover the working directory."},
				"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-indexed)"},
				"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read"},
			},
			"required": []string{"path"},
		}),
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			if err := requireIntentOrError(argsJSON); err != nil {
				return "", err
			}
			var args struct {
				Path   string `json:"path"`
				Offset int    `json:"offset,omitempty"`
				Limit  int    `json:"limit,omitempty"`
				Intent string `json:"intent"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			if strings.TrimSpace(args.Path) == "" {
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
		},
	}
}

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

type EditOp struct {
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func EditFile(cwd string) agent.Tool {
	return agent.Tool{
		Name:        "edit",
		Description: "Apply one or more edits to a file. Each edit replaces old_text with new_text in order. By default old_text must match exactly once. REQUIRED: 'path' must be a file path; 'edits' must be a non-empty array.",
		Parameters: requireIntentSchema("edit", map[string]any{
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
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			if err := requireIntentOrError(argsJSON); err != nil {
				return "", err
			}
			var args struct {
				Path   string   `json:"path"`
				Edits  []EditOp `json:"edits"`
				Intent string   `json:"intent"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
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
		},
	}
}
