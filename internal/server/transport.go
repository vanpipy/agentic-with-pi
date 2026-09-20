package server

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"

	"github.com/vanpiyp/awp/internal/protocol"
)

func (s *Server) SocketPath() string {
	if ul, ok := s.listener.(interface{ Path() string }); ok {
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

// dispatchRun runs a single dispatch call and signals completion.
// Used by dispatchAsync to keep the per-conn read loop responsive.
func (s *Server) dispatchRun(conn io.Writer, connCtx context.Context, req *protocol.Request) {
	switch req.Method {
	case protocol.MethodPing:
		s.handlePing(conn, req)
	case protocol.MethodPrompt:
		s.handlePrompt(conn, connCtx, req)
	case protocol.MethodResume:
		s.handleResume(conn, req)
	case protocol.MethodCancel:
		s.handleCancel(conn, req)
	default:
		if err := protocol.MarshalEvent(conn, req.ID, protocol.EventError, map[string]string{
			"error": "unknown method: " + req.Method,
		}); err != nil {
			slog.Debug("server: marshal event failed", "req_id", req.ID, "method", req.Method, "stage", "default_error", "err", err)
		}
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