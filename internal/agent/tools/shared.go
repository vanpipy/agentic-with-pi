package tools

import (
	"path/filepath"
	"strings"
)

const (
	defaultReadMaxBytes = 256 * 1024
	defaultReadMaxLines = 2000
	defaultGrepMaxBytes = 256 * 1024
	defaultFindLimit    = 1000
	defaultLsLimit      = 500
)

type FileOptions struct {
	MaxBytes int
	MaxLines int
}

type BashOptions struct {
	Shell         string
	CommandPrefix string
	TimeoutSec    int
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

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}