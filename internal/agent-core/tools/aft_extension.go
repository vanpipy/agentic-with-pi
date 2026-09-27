package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
)

type GoFallbackFn func(ctx context.Context, argsJSON string) (string, error)

func isToolLevelFailure(name string, err error) bool {
	if err == nil {
		return false
	}
	prefix := "aft " + name + " failed: "
	return strings.HasPrefix(err.Error(), prefix)
}

func invokeAFT(ctx context.Context, backend *AftBackend, name string, argsJSON string, nested bool) (string, error) {
	params := map[string]any{}
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
			return "", err
		}
	}
	if nested {
		return backend.CallNested(name, params)
	}
	return backend.Call(name, params)
}

func aftCallTool(backend *AftBackend, name, description string, schema map[string]any, fallback GoFallbackFn) agentcore.Tool {
	return agentcore.ToolFunc{
		N: name,
		D: description,
		P: requireIntentSchema(name, schema),
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
			out, err := invokeAFT(ctx, backend, name, argsJSON, false)
			if err != nil && fallback != nil && isToolLevelFailure(name, err) {
				slog.Info("tool fell back from aft to go", "tool", name, "aft_err", err.Error())
				return fallback(ctx, argsJSON)
			}
			return out, err
		},
	}
}

func aftCallToolNested(backend *AftBackend, name, description string, schema map[string]any, fallback GoFallbackFn) agentcore.Tool {
	return agentcore.ToolFunc{
		N: name,
		D: description,
		P: requireIntentSchema(name, schema),
		Fn: func(ctx context.Context, argsJSON string) (string, error) {
			out, err := invokeAFT(ctx, backend, name, argsJSON, true)
			if err != nil && fallback != nil && isToolLevelFailure(name, err) {
				slog.Info("tool fell back from aft to go", "tool", name, "aft_err", err.Error())
				return fallback(ctx, argsJSON)
			}
			return out, err
		},
	}
}

func ReadAftForTest(backend *AftBackend) agentcore.Tool {
	return aftCallTool(backend, "read",
		"Read a file. Backed by AFT (Rust). Supports text, images, PDFs, line ranges.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "REQUIRED. File path (relative or absolute)."},
				"offset":     map[string]any{"type": "integer", "description": "1-based line number to start reading from."},
				"limit":      map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
				"start_line": map[string]any{"type": "integer", "description": "Alternative to offset (1-based)."},
				"end_line":   map[string]any{"type": "integer", "description": "Inclusive end line (1-based)."},
				"max_bytes":  map[string]any{"type": "integer", "description": "Cap the response payload size."},
				"hashline":   map[string]any{"type": "boolean", "description": "If true, return hashline-tagged content for use with hashline-aware edits."},
			},
			"required": []string{"path"},
		}, nil)
}

func WriteAftForTest(backend *AftBackend) agentcore.Tool {
	return aftCallTool(backend, "write",
		"Write a file. Backed by AFT (Rust). Creates parent directories as needed.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "REQUIRED. File path (relative or absolute)."},
				"content": map[string]any{"type": "string", "description": "REQUIRED. File contents to write."},
			},
			"required": []string{"path", "content"},
		}, nil)
}

func EditAftForTest(backend *AftBackend) agentcore.Tool {
	return aftCallTool(backend, "edit_match",
		"Replace text in a file by matching old_string and substituting new_string. Backed by AFT (Rust) with hashline byte verification.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":        map[string]any{"type": "string", "description": "REQUIRED. File path."},
				"old_string":  map[string]any{"type": "string", "description": "REQUIRED. Exact substring to match."},
				"new_string":  map[string]any{"type": "string", "description": "REQUIRED. Replacement string."},
				"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence instead of just the first."},
			},
			"required": []string{"path", "old_string", "new_string"},
		}, nil)
}

func BashAftForTest(backend *AftBackend) agentcore.Tool {
	return aftCallToolNested(backend, "bash",
		"Execute a shell command. Backed by AFT (Rust) with sandbox + permissions + 30-min background task support.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":    map[string]any{"type": "string", "description": "REQUIRED. Shell command to execute."},
				"timeout":    map[string]any{"type": "integer", "description": "Timeout in milliseconds (default 120000 = 2 min)."},
				"workdir":    map[string]any{"type": "string", "description": "Working directory (default: cwd)."},
				"background": map[string]any{"type": "boolean", "description": "Run in background, return task id immediately."},
				"compressed": map[string]any{"type": "boolean", "description": "Compress repetitive output."},
			},
			"required": []string{"command"},
		}, nil)
}

func NewAftToolWithFallbackForTest(backend *AftBackend, name, description string, schema map[string]any, fallback GoFallbackFn) agentcore.Tool {
	return aftCallTool(backend, name, description, schema, fallback)
}

func NewAftNestedToolWithFallbackForTest(backend *AftBackend, name, description string, schema map[string]any, fallback GoFallbackFn) agentcore.Tool {
	return aftCallToolNested(backend, name, description, schema, fallback)
}

type argOp func(map[string]any)

func applyArgTransform(argsJSON string, ops ...argOp) string {
	if argsJSON == "" {
		return argsJSON
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return argsJSON
	}
	for _, op := range ops {
		op(m)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return argsJSON
	}
	return string(b)
}

func renameField(oldKey, newKey string) argOp {
	return func(m map[string]any) {
		if v, ok := m[oldKey]; ok {
			if _, has := m[newKey]; !has {
				m[newKey] = v
			}
			delete(m, oldKey)
		}
	}
}

func dropFields(keys ...string) argOp {
	return func(m map[string]any) {
		for _, k := range keys {
			delete(m, k)
		}
	}
}

func transformReadOrWriteArgs(argsJSON string) string {
	return applyArgTransform(argsJSON,
		renameField("file", "path"),
		dropFields("start_line", "end_line", "max_bytes", "hashline"),
	)
}

func transformEditMatchArgs(argsJSON string) (string, error) {
	if argsJSON == "" {
		return "", fmt.Errorf("edit_match: empty args")
	}
	var src struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &src); err != nil {
		return "", fmt.Errorf("edit_match fallback: %w", err)
	}
	type op struct {
		OldText    string `json:"old_text"`
		NewText    string `json:"new_text"`
		ReplaceAll bool   `json:"replace_all"`
	}
	b, err := json.Marshal(struct {
		Path  string `json:"path"`
		Edits []op   `json:"edits"`
	}{
		Path: src.Path,
		Edits: []op{{
			OldText:    src.OldString,
			NewText:    src.NewString,
			ReplaceAll: src.ReplaceAll,
		}},
	})
	if err != nil {
		return "", fmt.Errorf("edit_match fallback marshal: %w", err)
	}
	return string(b), nil
}

func transformAgentGrepArgs(argsJSON string) string {
	return applyArgTransform(argsJSON,
		renameField("query", "pattern"),
		renameField("ignore_case", "ignoreCase"),
	)
}

func transformBashArgs(argsJSON string) string {
	return applyArgTransform(argsJSON,
		dropFields("background", "wait", "compressed"),
		func(m map[string]any) {
			if v, ok := m["timeout"]; ok {
				if n, ok := v.(float64); ok && n > 0 {
					m["timeout"] = int(n) / 1000
					if int(n)%1000 != 0 {
						m["timeout"] = int(n)/1000 + 1
					}
				}
			}
		},
	)
}

func transformLsArgs(argsJSON string) string {
	return applyArgTransform(argsJSON,
		func(m map[string]any) {
			if v, ok := m["depth"]; ok {
				delete(m, "depth")
				if n, ok := v.(float64); ok && n <= 0 {
					if _, has := m["limit"]; !has {
						m["limit"] = 500
					}
				}
			}
		},
	)
}
