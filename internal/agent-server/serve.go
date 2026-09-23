package agentserver

import (
	"bufio"
	"context"
	"log/slog"
	"net"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func (s *Server) SocketPath() string {
	if ul, ok := s.listener.(interface{ Path() string }); ok {
		return ul.Path()
	}
	return ""
}

func (s *Server) Serve() error {
	s.wg.Add(1)
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.shuttingDown.Load() {
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
	s.shuttingDown.Store(true)
	s.listener.Close()
	s.cancel()

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
		req, err := json_rpc.ReadRequest(reader)
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
