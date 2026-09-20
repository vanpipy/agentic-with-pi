package transport

import (
	"errors"
	"net"
)

type Listener interface {
	net.Listener
	Path() string
}

type unixListener struct {
	*net.UnixListener
	path string
}

func (u *unixListener) Path() string {
	return u.path
}

func NewUnixListener(path string) (Listener, error) {
	ln, err := Listen(path)
	if err != nil {
		return nil, err
	}
	ul, ok := ln.(*net.UnixListener)
	if !ok {
		ln.Close()
		return nil, errors.New("transport: not a UnixListener")
	}
	return &unixListener{UnixListener: ul, path: path}, nil
}
