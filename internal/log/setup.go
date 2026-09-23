package log

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/vanpiyp/awp/internal/paths"
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
		if err := prepareFile(cfg.File, cfg.MaxSize, cfg.MaxAge); err != nil {
			return err
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

func prepareFile(path string, maxSize, maxAge int) error {
	if err := paths.EnsureDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("mkdir logs dir: %w", err)
	}
	if maxAge > 0 {
		CleanupOld(filepath.Dir(path), maxAge)
	}
	if maxSize > 0 {
		if err := Rotate(path, int64(maxSize)); err != nil {
			return fmt.Errorf("rotate: %w", err)
		}
	}
	return nil
}

func openAppend(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

func DefaultLogFile() string {
	if env := os.Getenv("AWP_LOG_FILE"); env != "" {
		return env
	}
	return filepath.Join(paths.Home(), "logs", "awp.log")
}

func LLMLogPath() string {
	if env := os.Getenv("AWP_LLM_LOG"); env != "" {
		return env
	}
	if tmp := os.Getenv("TMP"); tmp != "" {
		return filepath.Join(tmp, "awp-llm.log")
	}
	return filepath.Join(os.TempDir(), "awp-llm.log")
}

func NewFileLogger(path string, level slog.Level) (*slog.Logger, error) {
	if path == "" {
		return nil, fmt.Errorf("log path is empty")
	}
	if err := prepareFile(path, 0, 0); err != nil {
		return nil, err
	}
	f, err := openAppend(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})), nil
}
