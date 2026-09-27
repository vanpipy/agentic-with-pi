package ipc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/ipc"
)

func TestReadServerPIDInvalidContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pid")

	if err := os.WriteFile(path, []byte("not a number"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ipc.ReadServerPID(path)
	if err == nil {
		t.Fatal("expected error for non-numeric pid file")
	}
	if !strings.Contains(err.Error(), "invalid pid") {
		t.Errorf("error = %q, want 'invalid pid' message", err.Error())
	}
}

func TestReadServerPIDEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pid")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ipc.ReadServerPID(path)
	if err == nil {
		t.Fatal("expected error for empty pid file")
	}
}

func TestReadServerPIDTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pid")

	if err := os.WriteFile(path, []byte("  12345  \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ipc.ReadServerPID(path)
	if err != nil {
		t.Fatalf("ReadServerPID: %v", err)
	}
	if got != 12345 {
		t.Errorf("got %d, want 12345", got)
	}
}

func TestRemoveServerPIDRemovesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pid")

	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ipc.RemoveServerPID(path); err != nil {
		t.Errorf("RemoveServerPID: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file should be gone, stat err = %v", err)
	}
}

func TestWriteServerPIDFailsWhenParentIsFile(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "not_a_dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "child", "pid")

	if err := ipc.WriteServerPID(path, 42); err == nil {
		t.Fatal("expected error when parent path is a regular file")
	}
}

func TestIsAliveCurrentProcess(t *testing.T) {
	if !ipc.IsAlive(os.Getpid()) {
		t.Errorf("current pid %d should be alive", os.Getpid())
	}
}

func TestIsAliveZero(t *testing.T) {
	if ipc.IsAlive(0) {
		t.Error("pid 0 should not be alive")
	}
}

func TestIsAliveNegative(t *testing.T) {
	if ipc.IsAlive(-1) {
		t.Error("negative pid should not be alive")
	}
}

func TestIsAliveDeadProcess(t *testing.T) {
	if ipc.IsAlive(2_000_000_000) {
		t.Error("out-of-range pid should not be alive")
	}
}
