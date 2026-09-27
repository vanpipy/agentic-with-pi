package log_test

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/log"
)

func TestSetupReturnsErrorWhenNoHandlers(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	cfg := log.Config{File: "", Stderr: false}
	if err := log.Setup(cfg); err == nil {
		t.Fatal("expected error when both File and Stderr are disabled")
	}
}

func TestSetupOpensExistingFile(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	dir := t.TempDir()
	logFile := filepath.Join(dir, "existing.log")
	if err := os.WriteFile(logFile, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := log.Config{
		Level:  slog.LevelInfo,
		File:   logFile,
		Stderr: false,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	slog.Info("after-setup")

	data, _ := os.ReadFile(logFile)
	if !strings.Contains(string(data), "old") {
		t.Error("Setup should preserve existing log content (append mode)")
	}
	if !strings.Contains(string(data), "after-setup") {
		t.Error("Setup should append new entries to existing file")
	}
}

func TestSetupOpenFileFailsForInvalidPath(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	dir := t.TempDir()
	parent := filepath.Join(dir, "not_a_dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := log.Config{
		Level:  slog.LevelInfo,
		File:   filepath.Join(parent, "child", "x.log"),
		Stderr: false,
	}
	if err := log.Setup(cfg); err == nil {
		t.Fatal("expected Setup to fail when parent path is a regular file")
	}
}

func TestLLMLogPathFromEnv(t *testing.T) {
	t.Setenv("AWP_LLM_LOG", "/custom/llm.log")
	if got := log.LLMLogPath(); got != "/custom/llm.log" {
		t.Errorf("LLMLogPath = %q, want /custom/llm.log", got)
	}
}

func TestLLMLogPathFromTMP(t *testing.T) {
	t.Setenv("AWP_LLM_LOG", "")
	tmp := t.TempDir()
	t.Setenv("TMP", tmp)

	got := log.LLMLogPath()
	want := filepath.Join(tmp, "awp-llm.log")
	if got != want {
		t.Errorf("LLMLogPath = %q, want %q", got, want)
	}
}

func TestLLMLogPathDefault(t *testing.T) {
	t.Setenv("AWP_LLM_LOG", "")
	t.Setenv("TMP", "")
	got := log.LLMLogPath()
	if !strings.HasSuffix(got, "awp-llm.log") {
		t.Errorf("LLMLogPath = %q, want suffix awp-llm.log", got)
	}
}

func TestNewFileLoggerWritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "x.log")
	logger, err := log.NewFileLogger(logFile, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hello", "k", "v")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Split(string(data), "\n")[0]
	if !strings.Contains(first, "hello") {
		t.Errorf("missing 'hello' in %q", first)
	}
	if !strings.Contains(first, "k=v") {
		t.Errorf("missing 'k=v' in %q", first)
	}
}

func TestNewFileLoggerEmptyPath(t *testing.T) {
	if _, err := log.NewFileLogger("", slog.LevelInfo); err == nil {
		t.Fatal("expected error for empty log path")
	}
}

func TestNewFileLoggerOpensAppend(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "x.log")
	if err := os.WriteFile(logFile, []byte("pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logger, err := log.NewFileLogger(logFile, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("after")

	data, _ := os.ReadFile(logFile)
	if !strings.Contains(string(data), "pre-existing") {
		t.Error("NewFileLogger should preserve prior content (append mode)")
	}
	if !strings.Contains(string(data), "after") {
		t.Error("NewFileLogger should write new entry")
	}
}

func TestNewFileLoggerRespectsLevel(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "x.log")
	logger, err := log.NewFileLogger(logFile, slog.LevelWarn)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("info-msg")
	logger.Warn("warn-msg")

	data, _ := os.ReadFile(logFile)
	content := string(data)
	if strings.Contains(content, "info-msg") {
		t.Error("info should be filtered at warn level")
	}
	if !strings.Contains(content, "warn-msg") {
		t.Error("warn should be written")
	}
}

func TestNewFileLoggerOpenFailsForInvalidPath(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "not_a_dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := log.NewFileLogger(filepath.Join(parent, "child", "x.log"), slog.LevelInfo); err == nil {
		t.Fatal("expected NewFileLogger to fail when parent path is a regular file")
	}
}

func TestSetupWithStderrOnly(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	cfg := log.Config{File: "", Stderr: true, Level: slog.LevelInfo}
	if err := log.Setup(cfg); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	slog.Info("from-stderr")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "from-stderr") {
		t.Errorf("stderr missing 'from-stderr'; got %q", out)
	}
	if !strings.Contains(out, "source=awp") {
		t.Errorf("stderr missing 'source=awp'; got %q", out)
	}
}

func TestSetupRotationTriggersRotate(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	dir := t.TempDir()
	logFile := filepath.Join(dir, "big.log")
	if err := os.WriteFile(logFile, make([]byte, 5000), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := log.Config{
		Level:   slog.LevelInfo,
		File:    logFile,
		Stderr:  false,
		MaxSize: 100,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "big-*.log.bak"))
	if len(matches) == 0 {
		t.Errorf("expected rotated backup; got %v", matches)
	}
}

func TestSetupWithCleanupOldDeletesOldBackups(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	dir := t.TempDir()
	oldBackup := filepath.Join(dir, "awp-2020-01-01.log.bak")
	if err := os.WriteFile(oldBackup, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldBackup, time.Now().AddDate(0, 0, -30), time.Now().AddDate(0, 0, -30)); err != nil {
		t.Fatal(err)
	}

	cfg := log.Config{
		Level:   slog.LevelInfo,
		File:    filepath.Join(dir, "fresh.log"),
		Stderr:  false,
		MaxAge:  1,
		MaxSize: 0,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
		t.Errorf("old .log.bak should be cleaned; stat err = %v", err)
	}
}
