package paths_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/paths"
)

func TestHomeRespectsEnv(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := paths.Home(); got != "/custom/home" {
		t.Errorf("got %q, want /custom/home", got)
	}
}

func TestHomeDefault(t *testing.T) {
	t.Setenv("AWP_HOME", "")
	if got := paths.Home(); got == "" {
		t.Errorf("Home returned empty string in default")
	}
}

func TestRuntimeDirDefaultUnderHome(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "")
	t.Setenv("AWP_HOME", "/custom/home")
	want := filepath.Join("/custom/home", "runtime")
	if got := paths.RuntimeDir(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRuntimeDirIgnoresXDG(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "")
	t.Setenv("AWP_HOME", "/custom/home")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	want := filepath.Join("/custom/home", "runtime")
	if got := paths.RuntimeDir(); got != want {
		t.Errorf("got %q, want %q (XDG must not leak into RuntimeDir)", got, want)
	}
}

func TestRuntimeDirRespectsEnv(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "/custom/runtime")
	t.Setenv("AWP_HOME", "/should/be/ignored")
	if got := paths.RuntimeDir(); got != "/custom/runtime" {
		t.Errorf("got %q, want /custom/runtime", got)
	}
}

func TestRuntimeDirEnvOverridesHome(t *testing.T) {
	t.Setenv("AWP_RUNTIME_DIR", "/explicit/runtime")
	t.Setenv("AWP_HOME", "/custom/home")
	if got := paths.RuntimeDir(); got != "/explicit/runtime" {
		t.Errorf("got %q, want /explicit/runtime (AWP_RUNTIME_DIR must beat AWP_HOME)", got)
	}
}

func TestConfigDirUnderHome(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := paths.ConfigDir(); got != "/custom/home" {
		t.Errorf("got %q, want /custom/home", got)
	}
}

func TestConfigDirIgnoresXDG(t *testing.T) {
	t.Setenv("AWP_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/home/user/.config")
	t.Setenv("AWP_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".awp")
	if got := paths.ConfigDir(); got != want {
		t.Errorf("ConfigDir = %q, want %q (XDG must not leak into ConfigDir)", got, want)
	}
}

func TestConfigDirRespectsEnv(t *testing.T) {
	t.Setenv("AWP_CONFIG_HOME", "/custom/config")
	if got := paths.ConfigDir(); got != "/custom/config" {
		t.Errorf("got %q", got)
	}
}

func TestLogsDirUnderHome(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := paths.LogsDir(); got != "/custom/home/logs" {
		t.Errorf("got %q", got)
	}
}

func TestSessionsDirUnderLogs(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := paths.SessionsDir(); got != "/custom/home/logs/sessions" {
		t.Errorf("got %q", got)
	}
}

func TestClientSocketPathIncludesPID(t *testing.T) {
	got := paths.ClientSocketPath(1234)
	wantSuffix := "/awp.sock.1234"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("got %q, want suffix %q", got, wantSuffix)
	}
}

func TestServerPidPathIncludesPID(t *testing.T) {
	got := paths.ServerPidPath(1234)
	wantSuffix := "/awp.tui.1234.pid"
	if !strings.HasSuffix(got, wantSuffix) {
		t.Errorf("got %q, want suffix %q", got, wantSuffix)
	}
}

func TestConfigPathDefault(t *testing.T) {
	t.Setenv("AWP_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AWP_HOME", "")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".awp", "config.yaml")
	if got := paths.ConfigPath(); got != want {
		t.Errorf("ConfigPath = %q, want %q", got, want)
	}
}

func TestConfigPathRespectsHomeEnv(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := paths.ConfigPath(); got != "/custom/home/config.yaml" {
		t.Errorf("got %q, want /custom/home/config.yaml", got)
	}
}

func TestEnsureDir(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "a", "b", "c")
	if err := paths.EnsureDir(target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("dir not created: %v", err)
	}
}
