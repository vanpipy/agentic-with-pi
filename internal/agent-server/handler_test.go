package agentserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
)

type fakeCore struct{}

func (f *fakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	close(ch)
	return ch, nil
}

func TestHandleCompactNoSession(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "c.sock")

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	defer func() { _ = s.Shutdown(t.Context()) }()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, err := json_rpc.NewRequest("C1", json_rpc.MethodCompact, json_rpc.CompactParams{
		SessionID: "missing",
		Force:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.Event != json_rpc.EventError {
		t.Fatalf("event = %q, want %q", resp.Event, json_rpc.EventError)
	}
	var d struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Data, &d); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if !strings.Contains(d.Error, "session not found") {
		t.Errorf("error message = %q, want it to mention 'session not found'", d.Error)
	}
}

func TestHandleCompactEmptySessionID(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "d.sock")

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	defer func() { _ = s.Shutdown(t.Context()) }()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, err := json_rpc.NewRequest("C1", json_rpc.MethodCompact, json_rpc.CompactParams{
		Force: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.Event != json_rpc.EventError {
		t.Fatalf("event = %q, want %q (rejected empty session_id)", resp.Event, json_rpc.EventError)
	}
}
