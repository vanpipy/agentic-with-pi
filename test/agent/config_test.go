package agent_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vanpiyp/awp/internal/agent"
)

func TestLoadConfigReadsUserFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "model: claude-3\nlog_path: /tmp/awp.log\n")
	t.Setenv("AWP_HOME", dir)

	cfg, err := agent.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "claude-3" {
		t.Errorf("Model = %q, want claude-3", cfg.Model)
	}
	if cfg.LogPath != "/tmp/awp.log" {
		t.Errorf("LogPath = %q", cfg.LogPath)
	}
}

func TestLoadConfigEmptyFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "")
	t.Setenv("AWP_HOME", dir)

	cfg, err := agent.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "MiniMax-M3" {
		t.Errorf("Model = %q, want default MiniMax-M3", cfg.Model)
	}
	if cfg.LogPath != "" {
		t.Errorf("LogPath = %q, want empty default", cfg.LogPath)
	}
}

func TestLoadConfigMissingFileFallsBack(t *testing.T) {
	t.Setenv("AWP_HOME", t.TempDir())

	cfg, err := agent.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "MiniMax-M3" {
		t.Errorf("Model = %q, want default", cfg.Model)
	}
}

func TestLoadConfigSucceedsWithoutAPIKey(t *testing.T) {
	t.Setenv("AWP_HOME", t.TempDir())
	t.Setenv("MINIMAX_API_KEY", "")

	if _, err := agent.LoadConfig(); err != nil {
		t.Errorf("LoadConfig should not require MINIMAX_API_KEY (env-only): %v", err)
	}
}

func TestLoadConfigDoesNotReadAPIKeyFromYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", "api_key: should-not-load\nmodel: claude-3\n")
	t.Setenv("AWP_HOME", dir)

	cfg, err := agent.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "claude-3" {
		t.Errorf("Model = %q, want claude-3", cfg.Model)
	}
}

func TestLoadConfigIgnoresAgentInternalFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml", `
model: claude-3
api_key: should-not-load
max_turns: 99
compaction:
  enabled: false
  reserve_tokens: 1
`)
	t.Setenv("AWP_HOME", dir)

	cfg, err := agent.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "claude-3" {
		t.Errorf("Model = %q, want claude-3", cfg.Model)
	}
	if !structOnlyHasFields(*cfg, "Model", "LogPath", "Tools") {
		t.Errorf("Config struct should only expose user-facing fields; got %+v", *cfg)
	}
}

func structOnlyHasFields(v any, allowed ...string) bool {
	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}
	rv := reflect.ValueOf(v)
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		if !allowedSet[rt.Field(i).Name] {
			return false
		}
	}
	return true
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
