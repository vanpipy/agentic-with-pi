package server

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/protocol"
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

func (s *Server) SocketPath() string {
	if ul, ok := s.listener.(transport.Listener); ok {
		return ul.Path()
	}
	return ""
}

func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return nil
			}
			slog.Debug("server: accept error", "err", err)
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	s.listener.Close()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	connCtx, connCancel := context.WithCancel(s.ctx)
	defer connCancel()

	reader := bufio.NewReader(conn)

	for {
		req, err := protocol.ReadRequest(reader)
		if err != nil {
			return
		}

		if req.JSONRPC != "2.0" {
			return
		}

		s.dispatch(conn, connCtx, req)
	}
}

func (s *Server) registerConnCancel(reqID string, cancel context.CancelFunc) {
	s.connCancelsMu.Lock()
	defer s.connCancelsMu.Unlock()
	if prev, ok := s.connCancels[reqID]; ok {
		prev()
	}
	s.connCancels[reqID] = cancel
}

func (s *Server) popConnCancel(reqID string) context.CancelFunc {
	s.connCancelsMu.Lock()
	defer s.connCancelsMu.Unlock()
	cancel, ok := s.connCancels[reqID]
	if ok {
		delete(s.connCancels, reqID)
	}
	return cancel
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
