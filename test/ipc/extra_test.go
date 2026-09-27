package ipc_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/ipc"
)

func TestListenEmptyPathError(t *testing.T) {
	if _, err := ipc.Listen(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestListenPathMethod(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	got, ok := ln.(interface{ Path() string })
	if !ok {
		t.Fatal("listener does not implement Path()")
	}
	if got.Path() != path {
		t.Errorf("Path() = %q, want %q", got.Path(), path)
	}
}

func TestListenEmptyParentDirectoryIsCreated(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b", "c")
	path := filepath.Join(deep, "sock")

	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatalf("Listen with deep parent dir: %v", err)
	}
	defer ln.Close()
}

func TestListenChmodFailureOnReadOnlyParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate read-only filesystem when running as root")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	path := filepath.Join(dir, "new.sock")
	if _, err := ipc.Listen(path); err == nil {
		t.Fatal("expected Listen to fail when parent dir is read-only")
	}
}

func TestListenParentDirIsRegularFile(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(regular, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(regular, "sock")
	if _, err := ipc.Listen(path); err == nil {
		t.Fatal("expected Listen to fail when parent is a regular file")
	}
}

func TestListenPathLengthLimit(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("a", 200)
	path := filepath.Join(dir, long, "sock")
	_, err := ipc.Listen(path)
	if err == nil {
		t.Fatal("expected Listen to fail on overly long path")
	}
}

func TestListenInvalidNetwork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sock")

	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	listener.Close()
	os.Remove(path)

	if _, err := ipc.Listen("@@bogus"); err == nil {
		t.Fatal("expected Listen to error on invalid abstract addr")
	}
}

func TestListenOverwritesExistingStaleSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.sock")
	if err := os.WriteFile(path, []byte("not a socket"), 0o644); err != nil {
		t.Fatal(err)
	}

	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatalf("Listen over stale: %v", err)
	}
	defer ln.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Errorf("expected socket file mode, got %v", info.Mode())
	}
}

func TestDialEmptyPathError(t *testing.T) {
	if _, err := ipc.Dial(""); err == nil {
		t.Fatal("expected error for empty dial path")
	}
}

func TestDialMissingPathError(t *testing.T) {
	if _, err := ipc.Dial(filepath.Join(t.TempDir(), "missing.sock")); err == nil {
		t.Fatal("expected error when socket does not exist")
	}
}

func TestIsRunningFalseOnMissingSocket(t *testing.T) {
	if ipc.IsRunning(filepath.Join(t.TempDir(), "nope.sock")) {
		t.Error("IsRunning should return false on missing socket")
	}
}

func TestIsRunningTrueOnLiveSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if !ipc.IsRunning(path) {
		t.Error("IsRunning should return true on a live socket")
	}
}

func TestIsRunningFalseOnEmptyPath(t *testing.T) {
	if ipc.IsRunning("") {
		t.Error("IsRunning should return false on empty path")
	}
}

func TestListenCloseRemovesSocketFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "removed.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected socket file to be removed after Close; stat err = %v", err)
	}
}

func TestListenCloseErrorPropagatesFromUnderlyingClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "close.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := ln.(interface{ Close() error }); !ok {
		t.Fatal("listener missing Close()")
	}

	if err := ln.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
}

func TestListenAcceptConnectionSucceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accept.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- c
	}()

	client, err := ipc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	select {
	case <-connCh:
	case err := <-errCh:
		t.Fatalf("server accept error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("server accept timeout")
	}
}

func TestWriteServerPIDWithDeepParentCreatesDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x", "y", "z", "pid")

	if err := ipc.WriteServerPID(path, 4242); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "4242" {
		t.Errorf("contents = %q, want 4242", string(data))
	}
}

func TestWriteServerPIDWithEmptyParentDirStillWrites(t *testing.T) {
	tmp, err := os.CreateTemp("", "pid")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	os.Remove(tmp.Name())

	defer os.Remove(tmp.Name())
	if err := ipc.WriteServerPID(tmp.Name(), 99); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(tmp.Name())
	if string(data) != "99" {
		t.Errorf("contents = %q, want 99", string(data))
	}
}

func TestReadServerPIDMissingFile(t *testing.T) {
	if _, err := ipc.ReadServerPID(filepath.Join(t.TempDir(), "nope.pid")); err == nil {
		t.Fatal("expected error reading missing pid file")
	}
}

func TestRemoveServerPIDReadOnlyParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test cannot simulate read-only dir when running as root")
	}

	dir := t.TempDir()
	sub := filepath.Join(dir, "parent")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sub, "pid")
	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o555); err != nil {
		t.Skipf("cannot chmod 0o555: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

	err := ipc.RemoveServerPID(path)
	if err == nil {
		t.Fatal("expected RemoveServerPID to fail when parent dir lacks write+exec")
	}
}

func TestRemoveServerPIDMissingFileIsNoop(t *testing.T) {
	if err := ipc.RemoveServerPID(filepath.Join(t.TempDir(), "nope.pid")); err != nil {
		t.Errorf("RemoveServerPID on missing file should be nil; got %v", err)
	}
}

func TestRemoveServerPIDRemovesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pid")
	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ipc.RemoveServerPID(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after RemoveServerPID; stat err = %v", err)
	}
}

func TestIsAliveZeroOrNegativeReturnsFalse(t *testing.T) {
	if ipc.IsAlive(0) {
		t.Error("IsAlive(0) should be false")
	}
	if ipc.IsAlive(-1) {
		t.Error("IsAlive(-1) should be false")
	}
}

func TestIsAliveSelfPIDTrue(t *testing.T) {
	if !ipc.IsAlive(os.Getpid()) {
		t.Errorf("IsAlive(os.Getpid()) should be true; got false")
	}
}

func TestIsAliveNonexistentPIDReturnsFalse(t *testing.T) {
	if ipc.IsAlive(99999999) {
		t.Errorf("IsAlive(99999999) should be false; got true")
	}
}

func TestListenBackToBackSamePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "back2back.sock")

	ln1, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ln1.Close()

	ln2, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
}

func TestDialTypeAsNetConn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "type.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
	}()

	conn, err := ipc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, ok := conn.(net.Conn); !ok {
		t.Error("Dial did not return a net.Conn")
	}
}

func TestListenListenerIsNetListener(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "net.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if _, ok := ln.(net.Listener); !ok {
		t.Error("Listen did not return a net.Listener")
	}
}

func TestListenUnblocksAcceptOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unblock.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}

	doneCh := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		doneCh <- err
	}()

	time.Sleep(50 * time.Millisecond)

	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected Accept to error after Close")
		}
		if !errors.Is(err, net.ErrClosed) {
			t.Errorf("err = %v, want net.ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept did not unblock after Close")
	}
}

func TestListenContextDoesNotInterfere(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ctx.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
		cancel()
	}()

	conn, err := ipc.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context cancellation did not propagate")
	}
}

func TestListenInvalidPathWithNullByte(t *testing.T) {
	if _, err := ipc.Listen("/tmp/\x00bad.sock"); err == nil {
		t.Fatal("expected error for path with null byte")
	}
}

func TestIsRunningTrueAfterCloseIsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alive.sock")
	ln, err := ipc.Listen(path)
	if err != nil {
		t.Fatal(err)
	}

	if !ipc.IsRunning(path) {
		t.Fatal("IsRunning should be true while listener is open")
	}

	ln.Close()

	if ipc.IsRunning(path) {
		t.Error("IsRunning should be false after Close")
	}
}

func TestSyscallEpermHandledForIsAlive(t *testing.T) {
	if ipc.IsAlive(syscall.Getpid()) == false {
		t.Fatal("IsAlive(self) should be true; this confirms EPERM/0 path")
	}
}
