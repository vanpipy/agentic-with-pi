package tools

import (
	"context"
	"encoding/json"

	"github.com/vanpiyp/awp/internal/agent"
)

func aftCallTool(backend *AftBackend, name, description string, schema map[string]any) agent.Tool {
	return agent.ToolFunc{
		N: name,
		D: description,
		P: requireIntentSchema(name, schema),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			params := map[string]any{}
			if argsJSON != "" {
				if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
					return "", err
				}
			}
			return backend.Call(name, params)
		},
	}
}

func ReadAftForTest(backend *AftBackend) agent.Tool {
	return aftCallTool(backend, "read",
		"Read a file. Backed by AFT (Rust). Supports text, images, PDFs, line ranges.",
		map[string]any{
			"type":     "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "REQUIRED. File path (relative or absolute)."},
				"offset":      map[string]any{"type": "integer", "description": "1-based line number to start reading from."},
				"limit":       map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
				"start_line":  map[string]any{"type": "integer", "description": "Alternative to offset (1-based)."},
				"end_line":    map[string]any{"type": "integer", "description": "Inclusive end line (1-based)."},
				"max_bytes":   map[string]any{"type": "integer", "description": "Cap the response payload size."},
				"hashline":    map[string]any{"type": "boolean", "description": "If true, return hashline-tagged content for use with hashline-aware edits."},
			},
			"required": []string{"path"},
		})
}

func WriteAftForTest(backend *AftBackend) agent.Tool {
	return aftCallTool(backend, "write",
		"Write a file. Backed by AFT (Rust). Creates parent directories as needed.",
		map[string]any{
			"type":     "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "REQUIRED. File path (relative or absolute)."},
				"content": map[string]any{"type": "string", "description": "REQUIRED. File contents to write."},
			},
			"required": []string{"path", "content"},
		})
}

func EditAftForTest(backend *AftBackend) agent.Tool {
	return aftCallTool(backend, "edit_match",
		"Replace text in a file by matching old_string and substituting new_string. Backed by AFT (Rust) with hashline byte verification.",
		map[string]any{
			"type":     "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "REQUIRED. File path."},
				"old_string":  map[string]any{"type": "string", "description": "REQUIRED. Exact substring to match."},
				"new_string":  map[string]any{"type": "string", "description": "REQUIRED. Replacement string."},
				"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence instead of just the first."},
			},
			"required": []string{"path", "old_string", "new_string"},
		})
}

func BashAftForTest(backend *AftBackend) agent.Tool {
	return aftCallTool(backend, "bash",
		"Execute a shell command. Backed by AFT (Rust) with sandbox + permissions + 30-min background task support.",
		map[string]any{
			"type":     "object",
			"properties": map[string]any{
				"command":     map[string]any{"type": "string", "description": "REQUIRED. Shell command to execute."},
				"timeout":     map[string]any{"type": "integer", "description": "Timeout in milliseconds (default 120000 = 2 min)."},
				"workdir":     map[string]any{"type": "string", "description": "Working directory (default: cwd)."},
				"background":  map[string]any{"type": "boolean", "description": "Run in background, return task id immediately."},
				"compressed":  map[string]any{"type": "boolean", "description": "Compress repetitive output."},
			},
			"required": []string{"command"},
		})
}
