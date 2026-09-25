package agentclient_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func TestClient_Compact_SendsCompactMethodAndParsesResult(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "compact.sock")

	var (
		mu        sync.Mutex
		gotMethod string
		gotParams json.RawMessage
	)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		req, rerr := json_rpc.ReadRequest(reader)
		if rerr != nil {
			t.Errorf("server: read request: %v", rerr)
			return
		}
		mu.Lock()
		gotMethod = req.Method
		gotParams = append(json.RawMessage(nil), req.Params...)
		mu.Unlock()

		if req.Method != json_rpc.MethodCompact {
			t.Errorf("server: got method %q, want %q", req.Method, json_rpc.MethodCompact)
		}
		var p json_rpc.CompactParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Errorf("server: unmarshal params: %v", err)
			return
		}
		if p.SessionID != "sess-9" {
			t.Errorf("server: session_id = %q, want sess-9", p.SessionID)
		}
		if !p.Force {
			t.Errorf("server: force = false, want true")
		}

		result := json_rpc.CompactResult{
			Triggered:    true,
			Strategy:     "forced",
			TokensBefore: 9000,
			TokensAfter:  3500,
			DurationMS:   1234,
		}
		if err := json_rpc.MarshalEvent(conn, req.ID, "compact_result", result); err != nil {
			t.Errorf("server: marshal result: %v", err)
		}
	}()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := c.Compact(context.Background(), "sess-9", true)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != json_rpc.MethodCompact {
		t.Errorf("client sent method %q, want %q", gotMethod, json_rpc.MethodCompact)
	}
	if !strings.Contains(string(gotParams), `"session_id":"sess-9"`) {
		t.Errorf("client params missing session_id: %q", gotParams)
	}
	if !strings.Contains(string(gotParams), `"force":true`) {
		t.Errorf("client params missing force=true: %q", gotParams)
	}
	if !res.Triggered || res.Strategy != "forced" || res.TokensBefore != 9000 || res.TokensAfter != 3500 || res.DurationMS != 1234 {
		t.Errorf("Compact result not parsed: %+v", res)
	}
}

func TestClient_Compact_ServerErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "compact-err.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		req, rerr := json_rpc.ReadRequest(reader)
		if rerr != nil {
			t.Errorf("server: read request: %v", rerr)
			return
		}
		if err := json_rpc.MarshalEvent(conn, req.ID, json_rpc.EventError, map[string]string{
			"error": "session not found",
		}); err != nil {
			t.Errorf("server: marshal error: %v", err)
		}
	}()

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_, err = c.Compact(context.Background(), "missing", false)
	<-done
	if err == nil {
		t.Fatal("expected error from Compact, got nil")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Errorf("error = %q, want it to mention 'session not found'", err)
	}
}
