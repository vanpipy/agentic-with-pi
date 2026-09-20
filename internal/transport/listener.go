package transport

import "net"

type Listener interface {
	net.Listener
	Path() string
}
