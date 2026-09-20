package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func Home() string {
	if env := os.Getenv("AWP_HOME"); env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".awp")
}

func RuntimeDir() string {
	if env := os.Getenv("AWP_RUNTIME_DIR"); env != "" {
		return env
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "awp")
	}
	if tmpdir := os.Getenv("TMPDIR"); tmpdir != "" {
		return filepath.Join(tmpdir, "awp-"+userDiscriminator())
	}
	return filepath.Join("/tmp", "awp-"+userDiscriminator())
}

func ConfigDir() string {
	if env := os.Getenv("AWP_CONFIG_HOME"); env != "" {
		return env
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "awp")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "awp")
}

func LogsDir() string {
	return filepath.Join(Home(), "logs")
}

func SessionsDir() string {
	return filepath.Join(LogsDir(), "sessions")
}

func SocketPath() string {
	if env := os.Getenv("AWP_SOCKET"); env != "" {
		return env
	}
	return filepath.Join(RuntimeDir(), "awp.sock")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "awp.yaml")
}

func EnsureDirs() error {
	for _, dir := range []string{
		Home(),
		LogsDir(),
		SessionsDir(),
		ConfigDir(),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func userDiscriminator() string {
	if uid := os.Getuid(); uid >= 0 {
		return strconv.Itoa(uid)
	}
	return "default"
}
