package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/storage"
	"github.com/vanpiyp/awp/internal/transport"
)

type Server struct {
	listener    net.Listener
	stores      map[string]*Store
	storesMu    sync.RWMutex
	agent       *agent.Agent
	sessionsDir string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	connCancels   map[string]context.CancelFunc
	connCancelsMu sync.Mutex
}

func New(ag *agent.Agent, socketPath string) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ln, err := transport.NewUnixListener(socketPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("listen: %w", err)
	}

	sessionsDir := storage.SessionsDir()
	if err := storage.EnsureDir(sessionsDir); err != nil {
		cancel()
		return nil, fmt.Errorf("mkdir sessions: %w", err)
	}

	return &Server{
		listener:    ln,
		stores:      make(map[string]*Store),
		agent:       ag,
		sessionsDir: sessionsDir,
		ctx:         ctx,
		cancel:      cancel,
		connCancels: make(map[string]context.CancelFunc),
	}, nil
}

func (s *Server) getOrCreateStore(sessionID string) *Store {
	s.storesMu.Lock()
	defer s.storesMu.Unlock()
	if st, ok := s.stores[sessionID]; ok {
		return st
	}
	path := DefaultPath(s.sessionsDir, sessionID)
	st := NewStore(path)
	s.stores[sessionID] = st
	return st
}

func (s *Server) getStore(sessionID string) (*Store, error) {
	s.storesMu.RLock()
	if st, ok := s.stores[sessionID]; ok {
		s.storesMu.RUnlock()
		return st, nil
	}
	s.storesMu.RUnlock()

	path := DefaultPath(s.sessionsDir, sessionID)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	st := NewStore(path)
	s.storesMu.Lock()
	s.stores[sessionID] = st
	s.storesMu.Unlock()
	return st, nil
}