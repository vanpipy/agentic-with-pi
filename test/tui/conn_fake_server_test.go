package tui_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func startFakeRPCServer(t *testing.T, handler func(conn net.Conn, req *json_rpc.Request)) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "fake.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				reader := bufio.NewReader(c)
				_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
				req, rerr := json_rpc.ReadRequest(reader)
				if rerr != nil {
					return
				}
				handler(c, req)
			}(conn)
		}
	}()

	cleanup := func() {
		listener.Close()
		wg.Wait()
	}
	return socketPath, cleanup
}

func writeJSONRPC(t *testing.T, w net.Conn, req *json_rpc.Request, event string, data any) {
	t.Helper()
	if err := json_rpc.MarshalEvent(w, req.ID, event, data); err != nil {
		t.Logf("write response: %v", err)
	}
}

func dialFakeConn(t *testing.T, socketPath string) *agentclient.Client {
	t.Helper()
	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestStartResumeConnDrivenHappyPath(t *testing.T) {
	responded := make(chan struct{})
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if req.Method != json_rpc.MethodResume {
			t.Errorf("expected method %q, got %q", json_rpc.MethodResume, req.Method)
		}
		var params json_rpc.ResumeParams
		_ = json.Unmarshal(req.Params, &params)
		if params.SessionID != "sess-abc" {
			t.Errorf("expected sessionID 'sess-abc', got %q", params.SessionID)
		}
		writeJSONRPC(t, conn, req, "session_resumed", nil)
		close(responded)
	})
	defer cleanup()

	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	tui.SetSessionForTest(m, "")

	cmd := tui.StartResumeForTest(m, "sess-abc")
	if cmd == nil {
		t.Fatalf("startResume with conn should return non-nil cmd")
	}
	if got := m.StateForTest(); got != tui.StateStreaming {
		t.Fatalf("startResume should set StateStreaming, got %v", afterStateName(got))
	}
	select {
	case <-responded:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not receive resume request")
	}
}

func afterStateName(s tui.State) string {
	return fmt.Sprintf("State(%d)", int(s))
}

func TestStartResumeConnDrivenErrorPath(t *testing.T) {
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		_ = conn.Close()
	})
	defer cleanup()

	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)

	cmd := tui.StartResumeForTest(m, "missing")
	if cmd == nil {
		t.Fatal("startResume should return cmd (readNextEvent) even after server drops")
	}
}

func TestShowSessionPickerConnDrivenEmpty(t *testing.T) {
	handled := make(chan struct{})
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if req.Method != json_rpc.MethodListSessions {
			t.Errorf("expected method %q, got %q", json_rpc.MethodListSessions, req.Method)
		}
		body, _ := json.Marshal(json_rpc.ListSessionsResult{Sessions: nil})
		writeJSONRPC(t, conn, req, "list_sessions_result", body)
		close(handled)
	})
	defer cleanup()

	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)

	cmd := tui.ShowSessionPickerForTest(m)
	if cmd == nil {
		t.Fatal("showSessionPicker should return non-nil cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("cmd() returned nil")
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not handle list_sessions")
	}
}

func TestShowSessionPickerConnDrivenError(t *testing.T) {
	handled := make(chan struct{})
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		writeJSONRPC(t, conn, req, json_rpc.EventError, map[string]string{"error": "db unavailable"})
		close(handled)
	})
	defer cleanup()

	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)

	cmd := tui.ShowSessionPickerForTest(m)
	if cmd == nil {
		t.Fatal("showSessionPicker should return cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("cmd() returned nil")
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not handle list_sessions")
	}
}

func TestRunCompactConnDrivenHappyPath(t *testing.T) {
	handled := make(chan struct{})
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		if req.Method != json_rpc.MethodCompact {
			t.Errorf("expected method %q, got %q", json_rpc.MethodCompact, req.Method)
		}
		body := json_rpc.CompactResult{
			Triggered:    true,
			Strategy:     "truncate",
			TokensBefore: 12345,
			TokensAfter:  4321,
			DurationMS:   7,
		}
		writeJSONRPC(t, conn, req, "compact_result", body)
		close(handled)
	})
	defer cleanup()

	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	tui.SetSessionForTest(m, "sess-1")

	cmd := tui.RunCompactForTest(m, true)
	if cmd == nil {
		t.Fatal("runCompact should return cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("cmd() returned nil")
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not handle compact")
	}
}

func TestRunCompactConnDrivenErrorPath(t *testing.T) {
	handled := make(chan struct{})
	socketPath, cleanup := startFakeRPCServer(t, func(conn net.Conn, req *json_rpc.Request) {
		writeJSONRPC(t, conn, req, json_rpc.EventError, map[string]string{"error": "compact failed"})
		close(handled)
	})
	defer cleanup()

	c := dialFakeConn(t, socketPath)
	defer c.Close()

	m := tui.NewModelForTest()
	m.SetConnForTest(c)
	tui.SetSessionForTest(m, "sess-1")

	cmd := tui.RunCompactForTest(m, false)
	if cmd == nil {
		t.Fatal("runCompact should return cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("cmd() returned nil")
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not handle compact")
	}
}
