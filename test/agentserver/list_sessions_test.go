package agentserver_test

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/paths"
)

func writeSessionJSONL(t *testing.T, dir, sessionID string, model string) {
	t.Helper()
	store := agentserver.NewStore(filepath.Join(dir, sessionID+".jsonl"))
	if err := store.WriteHeader(agentserver.SessionMeta{
		SessionID: sessionID,
		Model:     model,
		MaxTurns:  200,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteEvent("thought_chunk", map[string]string{"reasoning": "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestServerListSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()

	writeSessionJSONL(t, sessionsDir, "alpha001", "test-model-a")
	writeSessionJSONL(t, sessionsDir, "beta002", "test-model-b")
	writeSessionJSONL(t, sessionsDir, "gamma003", "test-model-c")

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	socketPath := filepath.Join(dir, "test.sock")
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()

	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("L", json_rpc.MethodListSessions, json_rpc.ListSessionsParams{})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	resp, err := json_rpc.ReadEvent(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Event != "sessions_list" {
		t.Fatalf("event = %q, want sessions_list", resp.Event)
	}
	var result json_rpc.ListSessionsResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 3 {
		t.Fatalf("want 3 sessions, got %d", len(result.Sessions))
	}
}

func TestServerListSessionsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	socketPath := filepath.Join(dir, "test.sock")
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()

	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("L", json_rpc.MethodListSessions, json_rpc.ListSessionsParams{})
	json_rpc.MarshalRequest(conn, req)
	resp, err := json_rpc.ReadEvent(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	var result json_rpc.ListSessionsResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 0 {
		t.Errorf("want 0 sessions in empty dir, got %d", len(result.Sessions))
	}
}

func TestServerSessionsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	got := paths.SessionsDir()
	logs := paths.LogsDir()
	if !filepath.IsAbs(got) {
		t.Errorf("SessionsDir() must return absolute path, got %s", got)
	}
	if filepath.Base(got) != "sessions" {
		t.Errorf("SessionsDir() base = %s, want sessions", filepath.Base(got))
	}
	if got != filepath.Join(logs, "sessions") {
		t.Errorf("SessionsDir() = %s, want %s", got, filepath.Join(logs, "sessions"))
	}
}
