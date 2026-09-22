package tools

import (
	"log/slog"

	"github.com/vanpiyp/awp/internal/agent"
)

func All(cwd string) []agent.Tool {
	if backend, ok := AftBackendForTest(); ok {
		slog.Info("agent: toolset from AFT backend")
		return []agent.Tool{
			aftCallTool(backend, "read",
				"Read a file. Backed by AFT (Rust). Supports text, images, PDFs, line ranges, hashline output.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file":       map[string]any{"type": "string", "description": "REQUIRED. File path (relative or absolute)."},
						"start_line": map[string]any{"type": "integer", "description": "1-based start line."},
						"end_line":   map[string]any{"type": "integer", "description": "Inclusive end line."},
						"limit":      map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
						"offset":     map[string]any{"type": "integer", "description": "0-based byte offset."},
						"hashline":   map[string]any{"type": "boolean", "description": "If true, return hashline-tagged content for use with hashline-aware edits."},
					},
					"required": []string{"file"},
				}),
			aftCallTool(backend, "write",
				"Write a file. Backed by AFT (Rust). Creates parent directories as needed.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file":    map[string]any{"type": "string", "description": "REQUIRED. File path (relative or absolute)."},
						"content": map[string]any{"type": "string", "description": "REQUIRED. File contents to write."},
					},
					"required": []string{"file", "content"},
				}),
			aftCallTool(backend, "edit_match",
				"Replace text in a file. Backed by AFT (Rust) with hashline byte verification — safe even when the file moved lines since the agent last read it.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file":        map[string]any{"type": "string", "description": "REQUIRED. File path."},
						"old_string":  map[string]any{"type": "string", "description": "REQUIRED. Exact substring to match."},
						"new_string":  map[string]any{"type": "string", "description": "REQUIRED. Replacement string."},
						"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence instead of just the first."},
					},
					"required": []string{"file", "old_string", "new_string"},
				}),
			aftCallToolNested(backend, "bash",
				"Execute a shell command. Backed by AFT (Rust) with sandbox + permissions + 30-min background task support.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":    map[string]any{"type": "string", "description": "REQUIRED. Shell command to execute."},
						"timeout":    map[string]any{"type": "integer", "description": "Timeout in milliseconds (default 120000 = 2 min)."},
						"workdir":    map[string]any{"type": "string", "description": "Working directory (default: cwd)."},
						"background": map[string]any{"type": "boolean", "description": "Run in background, return task id immediately."},
						"wait":       map[string]any{"type": "boolean", "description": "If true (and not background), block until completion. Otherwise return immediately."},
						"compressed": map[string]any{"type": "boolean", "description": "Compress repetitive output."},
					},
					"required": []string{"command"},
				}),
			aftCallTool(backend, "agentgrep",
				"Search for a regex pattern across files. Backed by AFT (Rust) with structured output.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":      map[string]any{"type": "string", "description": "REQUIRED. Regex pattern to search for."},
						"path":       map[string]any{"type": "string", "description": "File or directory to search in."},
						"glob":       map[string]any{"type": "string", "description": "Glob filter (e.g. **/*.go)."},
						"context":    map[string]any{"type": "integer", "description": "Lines of context around matches."},
						"ignore_case":map[string]any{"type": "boolean", "description": "Case-insensitive match."},
					},
					"required": []string{"query"},
				}),
			aftCallTool(backend, "glob",
				"List files matching a glob pattern. Backed by AFT (Rust) with sorted, deduplicated output.",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "REQUIRED. Glob pattern (e.g. **/*.go)."},
						"path":    map[string]any{"type": "string", "description": "Root directory for the search."},
					},
					"required": []string{"pattern"},
				}),
			aftCallTool(backend, "ls",
				"List directory entries. Backed by AFT (Rust).",
				map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":  map[string]any{"type": "string", "description": "Directory to list."},
						"depth": map[string]any{"type": "integer", "description": "Maximum recursion depth (0 = non-recursive)."},
					},
				}),
			InvalidTool(),
		}
	}

	slog.Info("agent: toolset from built-in Go (aft not available)")
	return []agent.Tool{
		ReadFile(cwd, FileOptions{}),
		WriteFile(cwd),
		EditFile(cwd),
		Bash(cwd, BashOptions{}),
		Grep(cwd, FileOptions{}),
		Find(cwd, FileOptions{}),
		Ls(cwd, FileOptions{}),
		InvalidTool(),
	}
}
