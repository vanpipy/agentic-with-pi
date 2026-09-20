package transport

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/vanpiyp/awp/internal/storage"
)

const socketPerm os.FileMode = 0o660

func Listen(path string) (Listener, error) {
	if path == "" {
		return nil, errors.New("transport: empty socket path")
	}

	if err := storage.EnsureDir(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("ensure parent dir: %w", err)
	}

	os.Remove(path)

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, fmt.Errorf("resolve addr: %w", err)
	}

	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	if err := os.Chmod(path, socketPerm); err != nil {
		listener.Close()
		os.Remove(path)
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	return &unixListener{UnixListener: listener, path: path}, nil
}

type unixListener struct {
	*net.UnixListener
	path string
}

func (u *unixListener) Path() string { return u.path }

func (u *unixListener) Close() error {
	err := u.UnixListener.Close()
	os.Remove(u.path)
	return err
}

func Dial(path string) (net.Conn, error) {
	if path == "" {
		return nil, errors.New("transport: empty socket path")
	}
	return net.Dial("unix", path)
}

func IsRunning(path string) bool {
	conn, err := Dial(path)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func Cleanup(path string) error {
	return os.Remove(path)
}
