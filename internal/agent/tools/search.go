package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

const (
	defaultGrepMaxBytes = 256 * 1024
	defaultFindLimit    = 1000
	defaultLsLimit      = 500
)

func Grep(cwd string, opts FileOptions) agent.Tool {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultGrepMaxBytes
	}
	return agent.Tool{
		Name:        "grep",
		Description: "Search file contents for a regex or literal pattern. Returns matching lines as `path:lineno:text`.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":    map[string]any{"type": "string"},
				"path":       map[string]any{"type": "string", "description": "File or directory (default cwd)"},
				"glob":       map[string]any{"type": "string", "description": "Filter files by glob pattern"},
				"ignoreCase": map[string]any{"type": "boolean", "default": false},
				"literal":    map[string]any{"type": "boolean", "default": false, "description": "Treat pattern as literal string instead of regex"},
				"context":    map[string]any{"type": "integer", "description": "Lines before/after match (default 0)"},
			},
			"required": []string{"pattern"},
		},
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			var args struct {
				Pattern    string `json:"pattern"`
				Path       string `json:"path,omitempty"`
				Glob       string `json:"glob,omitempty"`
				IgnoreCase bool   `json:"ignoreCase,omitempty"`
				Literal    bool   `json:"literal,omitempty"`
				Context    int    `json:"context,omitempty"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("invalid args: %w", err)
			}
			if args.Path == "" {
				args.Path = cwd
			} else {
				args.Path = absPath(cwd, args.Path)
			}

			var pattern *regexp.Regexp
			patternStr := args.Pattern
			if !args.Literal {
				regexSrc := args.Pattern
				if args.IgnoreCase {
					regexSrc = "(?i)" + args.Pattern
				}
				p, err := regexp.Compile(regexSrc)
				if err != nil {
					return "", fmt.Errorf("invalid regex: %w", err)
				}
				pattern = p
			} else if args.IgnoreCase {
				patternStr = strings.ToLower(args.Pattern)
			}

			var matches []string
			err := filepath.WalkDir(args.Path, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if args.Glob != "" {
					ok, err := filepath.Match(args.Glob, filepath.Base(path))
					if err != nil {
						return err
					}
					if !ok {
						return nil
					}
				}
				f, err := os.Open(path)
				if err != nil {
					return nil
				}
				defer f.Close()
				scanner := bufio.NewScanner(f)
				scanner.Buffer(make([]byte, 0, 64*1024), maxBytes)
				lineNo := 0
				for scanner.Scan() {
					lineNo++
					line := scanner.Text()
					checkLine := line
					if args.IgnoreCase {
						checkLine = strings.ToLower(line)
					}
					var hit bool
					if args.Literal || pattern == nil {
						hit = strings.Contains(checkLine, patternStr)
					} else {
						hit = pattern.MatchString(line)
					}
					if hit {
						matches = append(matches, fmt.Sprintf("%s:%d:%s", path, lineNo, line))
					}
				}
				return nil
			})
			if err != nil {
				return "", err
			}
			if len(matches) == 0 {
				return "(no matches)", nil
			}
			out := strings.Join(matches, "\n")
			out, _ = truncate(out, maxBytes)
			return out, nil
		},
	}
}

func Find(cwd string, opts FileOptions) agent.Tool {
	limit := opts.MaxLines
	if limit <= 0 {
		limit = defaultFindLimit
	}
	return agent.Tool{
		Name:        "find",
		Description: "Find files matching a glob pattern. Default cwd. Limited results.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
				"path":    map[string]any{"type": "string", "description": "Directory to search (default cwd)"},
				"limit":   map[string]any{"type": "integer", "description": "Max results (default 1000)"},
			},
			"required": []string{"pattern"},
		},
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path,omitempty"`
				Limit   int    `json:"limit,omitempty"`
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

func Ls(cwd string, opts FileOptions) agent.Tool {
	limit := opts.MaxLines
	if limit <= 0 {
		limit = defaultLsLimit
	}
	return agent.Tool{
		Name:        "ls",
		Description: "List directory entries. Default cwd. Directories have / suffix.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "Directory (default cwd)"},
				"limit": map[string]any{"type": "integer", "description": "Max entries (default 500)"},
			},
		},
		Execute: func(_ context.Context, argsJSON string) (string, error) {
			var args struct {
				Path  string `json:"path,omitempty"`
				Limit int    `json:"limit,omitempty"`
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

			entries, err := os.ReadDir(args.Path)
			if err != nil {
				return "", err
			}
			if len(entries) > args.Limit {
				entries = entries[:args.Limit]
			}
			lines := make([]string, 0, len(entries))
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				lines = append(lines, name)
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}
