package agentserver

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/ipc"
)

type Server struct {
	listener     net.Listener
	shuttingDown atomic.Bool
	agent        *agentcore.Agent
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup

	connCancels   map[string]context.CancelFunc
	connCancelsMu sync.Mutex

	sessionStates *sessionStateStore
}

func New(ag *agentcore.Agent, socketPath string) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ln, err := ipc.Listen(socketPath)
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
