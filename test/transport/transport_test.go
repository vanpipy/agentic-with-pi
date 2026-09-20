package transport_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/transport"
)

func TestListenAndDial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	conn, err := transport.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := []byte("hello")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len(msg))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, msg) {
		t.Errorf("got %q, want %q", buf, msg)
	}
}

func TestListenRemovesStaleSocket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()

	ln2, err := transport.Listen(path)
	if err != nil {
		t.Fatalf("second listen should succeed: %v", err)
	}
	ln2.Close()
}

func TestListenRemovesRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")
	if err := os.WriteFile(path, []byte("not a socket"), 0o644); err != nil {
		t.Fatal(err)
	}

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatalf("expected listen to remove regular file and succeed: %v", err)
	}
	ln.Close()
}

func TestDialFailsForMissing(t *testing.T) {
	if _, err := transport.Dial("/nonexistent/path.sock"); err == nil {
		t.Error("expected error dialing non-existent socket")
	}
}

func TestIsRunningFalse(t *testing.T) {
	if transport.IsRunning("/nonexistent/path.sock") {
		t.Error("should not be running")
	}
}

func TestIsRunningTrue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if !transport.IsRunning(path) {
		t.Error("should be running after listen")
	}
}

func TestListenEmptyPath(t *testing.T) {
	if _, err := transport.Listen(""); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestListenerCloseRemovesSocketFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("socket file missing right after Listen: %v", err)
	}

	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("socket file still exists after Close; stat err = %v", err)
	}
}

func TestDialEmptyPath(t *testing.T) {
	if _, err := transport.Dial(""); err == nil {
		t.Error("expected error for empty path")
	}
}

func TestListen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if ln.Path() != path {
		t.Errorf("Path() = %q, want %q", ln.Path(), path)
	}
}

func TestSocketPermIsNotWorldAccessible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := transport.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	info, _ := os.Stat(path)
	perm := info.Mode().Perm()
	if perm&0o007 != 0 {
		t.Errorf("socket should not be world accessible, got %v", perm)
	}
}
