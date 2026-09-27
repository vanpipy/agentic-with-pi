package log_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/log"
)

func TestCleanupOldZeroMaxAgeIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, 0)

	if _, err := os.Stat(filepath.Join(dir, "old.log")); err != nil {
		t.Errorf("expected file to remain when maxAge=0; stat err = %v", err)
	}
}

func TestCleanupOldNegativeMaxAgeIsNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.log"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, -10)

	if _, err := os.Stat(filepath.Join(dir, "old.log")); err != nil {
		t.Errorf("expected file to remain when maxAge<0; stat err = %v", err)
	}
}

func TestCleanupOldMissingDirIsNoop(t *testing.T) {
	log.CleanupOld(filepath.Join(t.TempDir(), "nope"), 7)
}

func TestCleanupOldSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir.log")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, 7)

	if _, err := os.Stat(sub); err != nil {
		t.Errorf("subdir should be preserved; stat err = %v", err)
	}
}

func TestCleanupOldSkipsNonLogFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"readme.txt", "data.json", "noext"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	log.CleanupOld(dir, 7)

	for _, name := range []string{"readme.txt", "data.json", "noext"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should be preserved; stat err = %v", name, err)
		}
	}
}

func TestCleanupOldRemovesOldLogFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.log")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, 1)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old.log removed; stat err = %v", err)
	}
}

func TestCleanupOldRemovesOldLogBakFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.log.bak")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, 1)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old.log.bak removed; stat err = %v", err)
	}
}

func TestCleanupOldKeepsRecentLogFiles(t *testing.T) {
	dir := t.TempDir()
	recent := filepath.Join(dir, "recent.log")
	if err := os.WriteFile(recent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, 365)

	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent.log should be preserved; stat err = %v", err)
	}
}

func TestCleanupOldContinuesOnInfoError(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "broken")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(sub, "weird.log")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	log.CleanupOld(dir, 1)

	if _, err := os.Stat(bad); err != nil {
		t.Errorf("nested log file should be preserved (subdir not recursed); stat err = %v", err)
	}
}

func TestRotateMissingFileNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.log")
	if err := log.Rotate(path, 100); err != nil {
		t.Errorf("Rotate on missing file should be nil; got %v", err)
	}
}

func TestRotateSmallFileNoop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	if err := os.WriteFile(path, []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := log.Rotate(path, 1000); err != nil {
		t.Errorf("Rotate on small file should be nil; got %v", err)
	}
}

func TestRotateSuccessRenamesToBak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "active.log")
	if err := os.WriteFile(path, []byte("big content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := log.Rotate(path, 5); err != nil {
		t.Fatal(err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "active-*.log.bak"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Errorf("expected backup file matching active-*.log.bak; got none")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("active.log should have been renamed away; stat err = %v", err)
	}
}

func TestDefaultConfigValues(t *testing.T) {
	c := log.DefaultConfig()
	if c.Level != slog.LevelInfo {
		t.Errorf("Level = %v, want slog.LevelInfo", c.Level)
	}
	if !c.Stderr {
		t.Error("Stderr = false, want true")
	}
	if c.MaxSize <= 0 {
		t.Errorf("MaxSize = %d, want > 0", c.MaxSize)
	}
	if c.MaxAge <= 0 {
		t.Errorf("MaxAge = %d, want > 0", c.MaxAge)
	}
	if c.File == "" {
		t.Error("File = \"\", want non-empty")
	}
}

func TestSetupWithEmptyFileNoHandlersReturnsError(t *testing.T) {
	err := log.Setup(log.Config{
		File:   "",
		Stderr: false,
	})
	if err == nil {
		t.Fatal("Setup with no file and no stderr should fail")
	}
}

func TestSetupSuccessfulCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := log.Config{
		Level:  slog.LevelDebug,
		File:   filepath.Join(dir, "x", "log.txt"),
		Stderr: false,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.File); err != nil {
		t.Errorf("expected log file at %s; stat err = %v", cfg.File, err)
	}
}

func TestPrepareFileWithRotateOnFreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.log")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := log.Config{
		File:    path,
		MaxSize: 1,
		Stderr:  false,
	}

	dir2 := filepath.Join(dir, "rotate-stale")
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}
	path2 := filepath.Join(dir2, "stale.log")
	if err := os.WriteFile(path2, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg2 := log.Config{
		File:    path2,
		MaxSize: 5,
		Stderr:  false,
	}

	_ = cfg

	if err := log.Setup(cfg2); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareFileMkdirFailurePropagates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate read-only filesystem when running as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	path := filepath.Join(parent, "x", "log.txt")
	err := log.Setup(log.Config{
		File:   path,
		Stderr: false,
	})
	if err == nil {
		t.Fatal("expected Setup to fail when parent dir is read-only")
	}
}

func TestSetupOpenAppendFailureWhenPathIsDir(t *testing.T) {
	dir := t.TempDir()
	dirPath := filepath.Join(dir, "log.txt")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	err := log.Setup(log.Config{
		File:   dirPath,
		Stderr: false,
	})
	if err == nil {
		t.Fatal("expected Setup to fail when log path is a directory")
	}
}

func TestNewFileLoggerOpenAppendFailureWhenPathIsDir(t *testing.T) {
	dir := t.TempDir()
	dirPath := filepath.Join(dir, "log.txt")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := log.NewFileLogger(dirPath, slog.LevelInfo); err == nil {
		t.Fatal("expected NewFileLogger to fail when log path is a directory")
	}
}

func TestPrepareFileRotateFailurePropagates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate read-only filesystem when running as root")
	}
	parent := t.TempDir()
	logPath := filepath.Join(parent, "log.txt")
	if err := os.WriteFile(logPath, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	err := log.Setup(log.Config{
		File:    logPath,
		MaxSize: 5,
		Stderr:  false,
	})
	if err == nil {
		t.Fatal("expected Setup to fail when rename fails")
	}
}

func TestNewFileLoggerInvalidParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate read-only filesystem when running as root")
	}

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	path := filepath.Join(parent, "x", "log.txt")
	if _, err := log.NewFileLogger(path, slog.LevelInfo); err == nil {
		t.Fatal("expected NewFileLogger to fail on read-only parent")
	}
}

func TestDefaultLogFileFromEnv(t *testing.T) {
	t.Setenv("AWP_LOG_FILE", "/tmp/explicit.log")
	if got := log.DefaultLogFile(); got != "/tmp/explicit.log" {
		t.Errorf("DefaultLogFile = %q, want /tmp/explicit.log", got)
	}
}

func TestSetupRotatesStaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := log.Config{
		File:    path,
		MaxSize: 5,
		Stderr:  false,
	}
	if err := log.Setup(cfg); err != nil {
		t.Fatal(err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "rotate-*.log.bak"))
	if len(matches) == 0 {
		t.Error("expected rotation to create a backup file")
	}
}
