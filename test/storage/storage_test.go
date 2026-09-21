package storage_test

import (
	"github.com/vanpiyp/awp/internal/storage"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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

func TestSocketPathRespectsEnv(t *testing.T) {
	t.Setenv("AWP_SOCKET", "/custom/awp.sock")
	if got := storage.SocketPath(); got != "/custom/awp.sock" {
		t.Errorf("got %q", got)
	}
}

func TestSocketPathDefault(t *testing.T) {
	t.Setenv("AWP_SOCKET", "")
	if got := storage.SocketPath(); got == "" {
		t.Errorf("SocketPath returned empty")
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

func TestAtomicWriteFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "subdir", "file.txt")
	data := []byte("hello world")

	if err := storage.AtomicWriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "hello world" {
		t.Errorf("got %q, want hello world", got)
	}
}

func TestAtomicWriteFilePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions not applicable on windows")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "secret.txt")

	if err := storage.AtomicWriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %v, want 0600", info.Mode().Perm())
	}
}

func TestEnsureDirsCreatesAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", filepath.Join(tmp, "home"))
	t.Setenv("AWP_CONFIG_HOME", filepath.Join(tmp, "config"))

	if err := storage.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{storage.Home(), storage.LogsDir(), storage.SessionsDir(), storage.ConfigDir()} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("dir not created: %s: %v", dir, err)
		}
	}
}

func TestAtomicWriteCleansUpTemp(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")

	if err := storage.AtomicWriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		if e.Name() != "file.txt" {
			t.Errorf("leftover file: %s", e.Name())
		}
	}
}
