package log_test

import (
	"github.com/vanpiyp/awp/internal/log"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetGlobalLogger(t *testing.T) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))
}

func TestSetupDefault(t *testing.T) {
	resetGlobalLogger(t)
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)

	cfg := log.DefaultConfig()
	cfg.File = filepath.Join(tmp, "logs", "awp.log")

	if err := log.Setup(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.File); err != nil {
		t.Errorf("log file not created: %v", err)
	}
}

func TestSetupWritesToFile(t *testing.T) {
	resetGlobalLogger(t)
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "awp.log")

	cfg := log.Config{
		Level:  slog.LevelInfo,
		File:   logFile,
		Stderr: false,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatal(err)
	}

	slog.Info("test message", "key", "value")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}

	firstLine := strings.Split(string(data), "\n")[0]
	var entry map[string]any
	if err := json.Unmarshal([]byte(firstLine), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\nline: %s", err, firstLine)
	}
	if entry["msg"] != "test message" {
		t.Errorf("msg = %v, want test message", entry["msg"])
	}
	if entry["source"] != "awp" {
		t.Errorf("source = %v, want awp", entry["source"])
	}
}

func TestSetupRespectsLevel(t *testing.T) {
	resetGlobalLogger(t)
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "awp.log")

	cfg := log.Config{
		Level:  slog.LevelWarn,
		File:   logFile,
		Stderr: false,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatal(err)
	}

	slog.Debug("debug msg")
	slog.Info("info msg")
	slog.Warn("warn msg")

	data, _ := os.ReadFile(logFile)
	content := string(data)

	if strings.Contains(content, "debug msg") {
		t.Errorf("debug should be filtered out at warn level")
	}
	if strings.Contains(content, "info msg") {
		t.Errorf("info should be filtered out at warn level")
	}
	if !strings.Contains(content, "warn msg") {
		t.Errorf("warn should be written")
	}
}

func TestRotateIfNeeded(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "awp.log")
	os.WriteFile(path, make([]byte, 200), 0o644)

	if err := log.Rotate(path, 100); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original file should be gone")
	}
	matches, _ := filepath.Glob(filepath.Join(tmp, "awp-*.log.bak"))
	if len(matches) != 1 {
		t.Errorf("expected 1 backup file, got %d", len(matches))
	}
}

func TestRotateIfNeededSkipsSmallFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "awp.log")
	os.WriteFile(path, []byte("small"), 0o644)

	if err := log.Rotate(path, 1000); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("small file should remain: %v", err)
	}
}

func TestCleanupOld(t *testing.T) {
	tmp := t.TempDir()
	old := filepath.Join(tmp, "awp-2020-01-01.log")
	recent := filepath.Join(tmp, "awp-2026-09-18.log")
	unrelated := filepath.Join(tmp, "random.txt")

	os.WriteFile(old, []byte("old"), 0o644)
	os.WriteFile(recent, []byte("recent"), 0o644)
	os.WriteFile(unrelated, []byte("keep"), 0o644)

	oldTime := time.Now().AddDate(0, 0, -10)
	os.Chtimes(old, oldTime, oldTime)

	log.CleanupOld(tmp, 7)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old .log should be deleted")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent .log should remain")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated file should not be touched")
	}
}

func TestCleanupOldZeroAge(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "awp-2020-01-01.log")
	os.WriteFile(path, []byte("old"), 0o644)
	os.Chtimes(path, time.Now().AddDate(0, 0, -100), time.Now().AddDate(0, 0, -100))

	log.CleanupOld(tmp, 0)

	if _, err := os.Stat(path); err != nil {
		t.Errorf("maxAge=0 should disable cleanup, but file deleted")
	}
}

func TestDefaultConfigEnvOverride(t *testing.T) {
	t.Setenv("AWP_LOG_FILE", "/custom/log/path.log")

	cfg := log.DefaultConfig()
	if cfg.File != "/custom/log/path.log" {
		t.Errorf("expected AWP_LOG_FILE override, got %q", cfg.File)
	}
}

func TestDefaultHomeRespectsEnv(t *testing.T) {
	t.Setenv("AWP_HOME", "/custom/home")
	if got := log.DefaultHome(); got != "/custom/home" {
		t.Errorf("defaultHome = %q, want /custom/home", got)
	}
}

func TestMultiHandlerDispatchesToBoth(t *testing.T) {
	resetGlobalLogger(t)
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()

	logFile1 := filepath.Join(tmp1, "a.log")
	logFile2 := filepath.Join(tmp2, "b.log")

	cfg := log.Config{
		Level:  slog.LevelInfo,
		File:   logFile1,
		Stderr: false,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatal(err)
	}

	f, _ := os.Create(logFile2)
	defer f.Close()
	slog.SetDefault(slog.New(slog.NewMultiHandler(
		slog.Default().Handler(),
		slog.NewJSONHandler(f, nil),
	)).With("source", "awp"))

	slog.Info("both handlers fire")

	data1, _ := os.ReadFile(logFile1)
	data2, _ := os.ReadFile(logFile2)

	if !strings.Contains(string(data1), "both handlers fire") {
		t.Errorf("file 1 missing message")
	}
	if !strings.Contains(string(data2), "both handlers fire") {
		t.Errorf("file 2 missing message")
	}
}
