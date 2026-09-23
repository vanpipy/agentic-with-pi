package agentserver_test

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/paths"
)

func TestHandleResumePlaysBackLegacyWithLegacyWireNames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "legacy-resume-001"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")

	const legacyJSONL = `{"kind":"session","version":1,"id":"legacy-resume-001","model":"MiniMax-M3","max_turns":200,"system":"you are helpful","tools":["read"],"started_at":"2026-09-22T16:26:47.803329214Z"}
{"kind":"event","seq":1,"at":"2026-09-22T16:26:47.803436334Z","category":"thought_start"}
{"kind":"event","seq":2,"at":"2026-09-22T16:26:48.675664287Z","category":"thought_chunk","reasoning":"the user"}
{"kind":"event","seq":3,"at":"2026-09-22T16:26:48.67567672Z","category":"thought_end","reasoning":"the user","content":"hi there"}
{"kind":"event","seq":4,"at":"2026-09-22T16:26:48.71773254Z","category":"tool","tool_name":"bash","tool_args":"{\"command\":\"date\"}"}
{"kind":"event","seq":5,"at":"2026-09-22T16:26:48.923685841Z","category":"observe","tool_name":"bash","tool_result":"ok","tool_error":""}
{"kind":"event","seq":6,"at":"2026-09-22T16:26:49.114038355Z","category":"final_answer","content":"done"}
{"kind":"event","seq":7,"at":"2026-09-22T16:26:58.013837338Z","category":"error","tool_error":"aborting"}
`
	if err := os.WriteFile(sessionPath, []byte(legacyJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, err := json_rpc.NewRequest("R1", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	gotEvents := readAllWireEvents(t, conn)

	wantOrder := []string{
		json_rpc.EventThoughtStart,
		json_rpc.EventThoughtChunk,
		json_rpc.EventThoughtEnd,
		json_rpc.EventTool,
		json_rpc.EventObserve,
		json_rpc.EventFinalAnswer,
		json_rpc.EventError,
		"session_resumed",
	}
	if len(gotEvents) != len(wantOrder) {
		t.Fatalf("wire events len = %d, want %d (got=%v)", len(gotEvents), len(wantOrder), gotEvents)
	}
	for i, want := range wantOrder {
		if gotEvents[i] != want {
			t.Errorf("wire event[%d] = %q, want %q", i, gotEvents[i], want)
		}
	}
}

func TestHandleResumePlaysBackNewSchemaWithNewWireNames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "newresume002"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")

	userMsg := json_rpc.MessageEvent{
		ID:        "0190a3b7-0001-7c8a-9000-000000000001",
		ParentID:  "",
		Timestamp: "2026-09-23T13:00:00.789Z",
		Message: json_rpc.Message{
			Role:    "user",
			Content: []json_rpc.MessageContentPart{{Type: "text", Text: "Hi"}},
		},
	}
	assistantMsg := json_rpc.MessageEvent{
		ID:        "0190a3b7-0002-7c8a-9000-000000000002",
		ParentID:  "0190a3b7-0001-7c8a-9000-000000000001",
		Timestamp: "2026-09-23T13:00:01.234Z",
		Message: json_rpc.Message{
			Role: "assistant",
			Content: []json_rpc.MessageContentPart{
				{Type: "thinking", Thinking: "greeting"},
				{Type: "text", Text: "Hi there!"},
			},
		},
		StopReason: "end_turn",
		Usage: &json_rpc.UsageStats{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	customEvent := json_rpc.CustomEvent{
		ID:         "0190a3b7-0003-7c8a-9000-000000000003",
		ParentID:   "0190a3b7-0002-7c8a-9000-000000000002",
		Timestamp:  "2026-09-23T13:00:02.000Z",
		CustomType: "abort",
		Data:       json.RawMessage(`{"reason":"user cancelled"}`),
	}

	lines := []string{
		`{"kind":"session","version":2,"id":"newresume002","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-23T13:00:00.000Z"}`,
	}
	for _, m := range []json_rpc.MessageEvent{userMsg, assistantMsg} {
		raw, _ := json.Marshal(m)
		wrapped, _ := json.Marshal(struct {
			Kind    string          `json:"kind"`
			Version int             `json:"version"`
			Entry   json.RawMessage `json:"entry"`
		}{Kind: "message", Version: 2, Entry: raw})
		lines = append(lines, string(wrapped))
	}
	{
		raw, _ := json.Marshal(customEvent)
		wrapped, _ := json.Marshal(struct {
			Kind    string          `json:"kind"`
			Version int             `json:"version"`
			Entry   json.RawMessage `json:"entry"`
		}{Kind: "custom", Version: 2, Entry: raw})
		lines = append(lines, string(wrapped))
	}

	if err := os.WriteFile(sessionPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, err := json_rpc.NewRequest("R1", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	gotEvents := readAllWireEvents(t, conn)

	wantOrder := []string{
		json_rpc.EventMessage,
		json_rpc.EventMessage,
		json_rpc.EventCustom,
		"session_resumed",
	}
	if len(gotEvents) != len(wantOrder) {
		t.Fatalf("wire events len = %d, want %d (got=%v)", len(gotEvents), len(wantOrder), gotEvents)
	}
	for i, want := range wantOrder {
		if gotEvents[i] != want {
			t.Errorf("wire event[%d] = %q, want %q", i, gotEvents[i], want)
		}
	}

	if strings.Contains(strings.Join(gotEvents, ","), "thought_start") {
		t.Errorf("new-schema resume must NOT emit legacy event names; got=%v", gotEvents)
	}
	if strings.Contains(strings.Join(gotEvents, ","), "thought_chunk") {
		t.Errorf("new-schema resume must NOT emit legacy event names; got=%v", gotEvents)
	}
}

func TestHandleResumeMissingSessionReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, err := json_rpc.NewRequest("R1", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: "missing-session-zzz"})
	if err != nil {
		t.Fatal(err)
	}
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Event != json_rpc.EventError {
		t.Fatalf("event = %q, want %q", resp.Event, json_rpc.EventError)
	}
	var d struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(resp.Data, &d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Error, "session not found") {
		t.Errorf("error = %q, expected to mention 'session not found'", d.Error)
	}
}

func startTestServerWithAgent(t *testing.T) (*agentserver.Server, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()

	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			return s, socketPath
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
	return nil, ""
}

func readAllWireEvents(t *testing.T, conn net.Conn) []string {
	t.Helper()
	reader := bufio.NewReader(conn)
	var events []string
	for {
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			return events
		}
		events = append(events, resp.Event)
		if resp.Event == "session_resumed" {
			return events
		}
	}
}
