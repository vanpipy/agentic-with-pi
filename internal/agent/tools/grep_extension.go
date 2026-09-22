package tools

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vanpiyp/awp/internal/agent"
)

type grepArgs struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	IgnoreCase bool   `json:"ignoreCase,omitempty"`
	Literal    bool   `json:"literal,omitempty"`
	Context    int    `json:"context,omitempty"`
}

func Grep(cwd string, opts FileOptions) agent.Tool {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultGrepMaxBytes
	}
	return agent.ToolFunc{
		N: "grep",
		D: "Search file contents for a regex or literal pattern. Returns matching lines as `path:lineno:text`. REQUIRED: 'pattern' must be a non-empty string.",
		P: requireIntentSchema("grep", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":    map[string]any{"type": "string", "description": "REQUIRED. Regex or literal pattern to search for."},
				"path":       map[string]any{"type": "string", "description": "File or directory (default cwd)"},
				"glob":       map[string]any{"type": "string", "description": "Filter files by glob pattern"},
				"ignoreCase": map[string]any{"type": "boolean", "default": false},
				"literal":    map[string]any{"type": "boolean", "default": false, "description": "Treat pattern as literal string instead of regex"},
				"context":    map[string]any{"type": "integer", "description": "Lines before/after match (default 0)"},
			},
			"required": []string{"pattern"},
		}),
		Fn: func(_ context.Context, argsJSON string) (string, error) {
			return agent.RunTool(context.TODO(), argsJSON, func(_ context.Context, args grepArgs) (string, error) {
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
			})
		},
	}
}
