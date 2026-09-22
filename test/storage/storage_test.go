package storage_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/storage"
)

func TestHomeRespectsEnv(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := storage.Home(); got != "/custom/home" {
		t.Errorf("got %q, want /custom/home", got)
	}
}

func TestHomeDefault(t *testing.T) {
	t.Setenv("AWP_HOME", "")
	if got := storage.Home(); got == "" {
		t.Errorf("Home returned empty string in default")
	}
}

func TestRuntimeDirPrefersXDG(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := storage.RuntimeDir(); got != "/run/user/1000/awp" {
		t.Errorf("got %q, want /run/user/1000/awp", got)
	}
}

func TestRuntimeDirRespectsEnv(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "/custom/runtime")
	if got := storage.RuntimeDir(); got != "/custom/runtime" {
		t.Errorf("got %q, want /custom/runtime", got)
	}
}

func TestRuntimeDirHasFallback(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := storage.RuntimeDir(); got == "" {
		t.Errorf("RuntimeDir returned empty in fallback path")
	}
}

func TestConfigDirUnderHome(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := storage.ConfigDir(); got != "/custom/home" {
		t.Errorf("got %q, want /custom/home", got)
	}
}

func TestConfigDirIgnoresXDG(t *testing.T) {
	t.Setenv("AWP_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/home/user/.config")
	t.Setenv("AWP_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".awp")
	if got := storage.ConfigDir(); got != want {
		t.Errorf("ConfigDir = %q, want %q (XDG must not leak into ConfigDir)", got, want)
	}
}

func TestConfigDirRespectsEnv(t *testing.T) {
	t.Setenv("AWP_CONFIG_HOME", "/custom/config")
	if got := storage.ConfigDir(); got != "/custom/config" {
		t.Errorf("got %q", got)
	}
}

func TestLogsDirUnderHome(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := storage.LogsDir(); got != "/custom/home/logs" {
		t.Errorf("got %q", got)
	}
}

func TestSessionsDirUnderLogs(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := storage.SessionsDir(); got != "/custom/home/logs/sessions" {
		t.Errorf("got %q", got)
	}
}

func TestClientSocketPathIncludesPID(t *testing.T) {
	got := storage.ClientSocketPath(1234)
	wantSuffix := "/awp.sock.1234"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("got %q, want suffix %q", got, wantSuffix)
	}
}

func TestServerPidPathIncludesPID(t *testing.T) {
	got := storage.ServerPidPath(1234)
	wantSuffix := "/awp.tui.1234.pid"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("got %q, want suffix %q", got, wantSuffix)
	}
}

func TestServerLogPathIncludesPID(t *testing.T) {
	got := storage.ServerLogPath(1234)
	wantSuffix := "/awp.server.1234.log"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("got %q, want suffix %q", got, wantSuffix)
	}
	if !strings.Contains(got, "/server/") {
		t.Errorf("got %q, want path under server/ subdir", got)
	}
}

func TestConfigPathDefault(t *testing.T) {
	t.Setenv("AWP_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AWP_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".awp", "config.yaml")
	if got := storage.ConfigPath(); got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}

func TestConfigPathRespectsHomeEnv(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := storage.ConfigPath(); got != "/custom/home/config.yaml" {
		t.Errorf("got %q, want /custom/home/config.yaml", got)
	}
}

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "a", "b", "c")
	if err := storage.EnsureDir(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}

