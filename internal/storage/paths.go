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
	return Home()
}

func LogsDir() string {
	return filepath.Join(Home(), "logs")
}

func SessionsDir() string {
	return filepath.Join(LogsDir(), "sessions")
}

func ServerLogDir() string {
	return filepath.Join(LogsDir(), "server")
}

func ClientSocketPath(clientPID int) string {
	return filepath.Join(RuntimeDir(), fmt.Sprintf("awp.sock.%d", clientPID))
}

func ServerPidPath(clientPID int) string {
	return filepath.Join(RuntimeDir(), fmt.Sprintf("awp.tui.%d.pid", clientPID))
}

func ServerLogPath(clientPID int) string {
	return filepath.Join(ServerLogDir(), fmt.Sprintf("awp.server.%d.log", clientPID))
}

func ConfigPath() string {
	return filepath.Join(Home(), "config.yaml")
}

func userDiscriminator() string {
	if uid := os.Getuid(); uid >= 0 {
		return strconv.Itoa(uid)
	}
	return "default"
}
