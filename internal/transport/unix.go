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

func Listen(path string) (net.Listener, error) {
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
		return nil, fmt.Errorf("chmod socket: %w", err)
	}

	return listener, nil
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
