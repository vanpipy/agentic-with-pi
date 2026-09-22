package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/transport"
)

type Server struct {
	listener     net.Listener
	shuttingDown atomic.Bool
	agent        *agent.Agent
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup

	connCancels   map[string]context.CancelFunc
	connCancelsMu sync.Mutex

	sessionStates *sessionStateStore
}

func New(ag *agent.Agent, socketPath string) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ln, err := transport.Listen(socketPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen: %w", err)
	}

	return &Server{
		listener:      ln,
		agent:         ag,
		ctx:           ctx,
		cancel:        cancel,
		connCancels:   make(map[string]context.CancelFunc),
		sessionStates: newSessionStateStore(),
	}, nil
}