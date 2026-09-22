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

func TestRuntimeDirDefaultUnderHome(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "")
	t.Setenv("AWP_HOME", "/custom/home")
	want := filepath.Join("/custom/home", "runtime")
	if got := storage.RuntimeDir(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRuntimeDirIgnoresXDG(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "")
	t.Setenv("AWP_HOME", "/custom/home")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	want := filepath.Join("/custom/home", "runtime")
	if got := storage.RuntimeDir(); got != want {
		t.Errorf("got %q, want %q (XDG must not leak into RuntimeDir)", got, want)
	}
}

func TestRuntimeDirRespectsEnv(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "/custom/runtime")
	t.Setenv("AWP_HOME", "/should/be/ignored")
	if got := storage.RuntimeDir(); got != "/custom/runtime" {
		t.Errorf("got %q, want /custom/runtime", got)
	}
}

func TestRuntimeDirEnvOverridesHome(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "/explicit/runtime")
	t.Setenv("AWP_HOME", "/custom/home")
	if got := storage.RuntimeDir(); got != "/explicit/runtime" {
		t.Errorf("got %q, want /explicit/runtime (AWP_RUNTIME_DIR must beat AWP_HOME)", got)
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
	t.Setenv("AWP_SERVER_LOG_DIR", "")
	got := storage.ServerLogPath(1234)
	wantSuffix := "/awp.server.1234.log"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("got %q, want suffix %q", got, wantSuffix)
	}
	if !strings.HasPrefix(got, filepath.Join(os.TempDir(), "awp-server-logs")) {
		t.Errorf("got %q, want path under $TMP/awp-server-logs", got)
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


func TestServerLogDirUsesTemp(t *testing.T) {
	t.Setenv("AWP_SERVER_LOG_DIR", "")
	got := storage.ServerLogDir()
	wantPrefix := filepath.Join(os.TempDir(), "awp-server-logs")
	if got != wantPrefix {
		t.Errorf("ServerLogDir() = %q, want %q (must live in $TMP, not ~/.awp/logs/)", got, wantPrefix)
	}
	if strings.Contains(got, ".awp") {
		t.Errorf("ServerLogDir() must not be under ~/.awp; got %q", got)
	}
}

func TestServerLogDirRespectsEnv(t *testing.T) {
	t.Setenv("AWP_SERVER_LOG_DIR", "/custom/awp-server-logs")
	if got := storage.ServerLogDir(); got != "/custom/awp-server-logs" {
		t.Errorf("got %q, want /custom/awp-server-logs", got)
	}
}

func TestServerLogPathUnderTemp(t *testing.T) {
	t.Setenv("AWP_SERVER_LOG_DIR", "")
	got := storage.ServerLogPath(12345)
	if strings.Contains(got, ".awp/logs/server") {
		t.Errorf("ServerLogPath must not be under ~/.awp/logs/server; got %q", got)
	}
	if !strings.HasPrefix(got, os.TempDir()) {
		t.Errorf("ServerLogPath = %q, want prefix %q", got, os.TempDir())
	}
}
