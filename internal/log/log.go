package log

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type Config struct {
	Level   slog.Level
	File    string
	Stderr  bool
	MaxSize int
	MaxAge  int
}

func DefaultConfig() Config {
	return Config{
		Level:   slog.LevelInfo,
		File:    DefaultLogFile(),
		Stderr:  true,
		MaxSize: 10 * 1024 * 1024,
		MaxAge:  7,
	}
}

func Setup(cfg Config) error {
	if cfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil {
			return fmt.Errorf("mkdir logs dir: %w", err)
		}
		if cfg.MaxAge > 0 {
			CleanupOld(filepath.Dir(cfg.File), cfg.MaxAge)
		}
		if cfg.MaxSize > 0 {
			if err := Rotate(cfg.File, int64(cfg.MaxSize)); err != nil {
				return fmt.Errorf("rotate: %w", err)
			}
		}
	}

	var handlers []slog.Handler
	if cfg.File != "" {
		f, err := openAppend(cfg.File)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		handlers = append(handlers, slog.NewJSONHandler(f, &slog.HandlerOptions{
			Level: cfg.Level,
		}))
	}
	if cfg.Stderr {
		handlers = append(handlers, slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: cfg.Level,
		}))
	}

	if len(handlers) == 0 {
		return fmt.Errorf("no handlers configured")
	}

	slog.SetDefault(slog.New(slog.NewMultiHandler(handlers...)).With("source", "awp"))
	return nil
}

func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

func DefaultLogFile() string {
	if env := os.Getenv("AWP_LOG_FILE"); env != "" {
		return env
	}
	return filepath.Join(DefaultHome(), "logs", "awp.log")
}

func DefaultHome() string {
	if env := os.Getenv("AWP_HOME"); env != "" {
		return env
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".awp")
}
