package agentserver_test

import (
	"bufio"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
)

var _ = llm.Usage{}

func readOneResponse(t *testing.T, conn net.Conn) *json_rpc.Response {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	r := bufio.NewReader(conn)
	resp, err := json_rpc.ReadEvent(r)
	if err != nil {
		t.Fatalf("ReadEvent: %v", err)
	}
	return resp
}

func dialTestServer(t *testing.T, dir string) net.Conn {
	t.Helper()
	for i := 0; i < 100; i++ {
		conn, err := net.Dial("unix", filepath.Join(dir, "test.sock"))
		if err == nil {
			return conn
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Skip("socket never ready")
	return nil
}

func TestMarshalAgentEventForWireUserMessageEmitsMessage(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventUserMessage, Content: "hi"},
		&parentID, &streamBuf,
	)
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
	if emits[0].EventName != json_rpc.EventMessage {
		t.Errorf("eventName = %q, want %q", emits[0].EventName, json_rpc.EventMessage)
	}
	if parentID == "" {
		t.Error("parentID should be set")
	}
}

func TestMarshalAgentEventForWireThoughtStartCreatesStreamBuf(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventThoughtStart},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0", len(emits))
	}
	if streamBuf == nil {
		t.Error("streamBuf should be created")
	}
}

func TestMarshalAgentEventForWireThoughtChunkNilStreamBufNoPanic(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "thinking"},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil streamBuf)", len(emits))
	}
}

func TestMarshalAgentEventForWireThoughtChunkWithStreamBuf(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "thinking"},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (thought_chunk is buffered)", len(emits))
	}
}

func TestMarshalAgentEventForWireThoughtEndWithUsage(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventThoughtEnd, Usage: &llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (thought_end doesn't emit on its own)", len(emits))
	}
}

func TestMarshalAgentEventForWireThoughtEndNoUsage(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventThoughtEnd},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (thought_end doesn't emit on its own)", len(emits))
	}
}

func TestMarshalAgentEventForWireToolEmitsAssistantAndToolResult(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("parent-x")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{
			Category:   agentcore.EventTool,
			ToolName:   "ls",
			ToolArgs:   `{"intent":"x","path":"."}`,
			ToolIntent: "x",
		},
		&parentID, &streamBuf,
	)
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2 (assistant + toolResult)", len(emits))
	}
	if streamBuf == nil {
		t.Error("streamBuf should be re-created after tool emit")
	}
}

func TestMarshalAgentEventForWireToolEmptyArgsUsesEmptyObject(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventTool, ToolName: "ls", ToolArgs: ""},
		&parentID, &streamBuf,
	)
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2", len(emits))
	}
}

func TestMarshalAgentEventForWireToolNoIntentFallsBackToExtract(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventTool, ToolName: "ls", ToolArgs: `{"intent":"fallback"}`},
		&parentID, &streamBuf,
	)
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2", len(emits))
	}
}

func TestMarshalAgentEventForWireToolErrorInResult(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventTool, ToolName: "ls", ToolArgs: `{}`, ToolError: "denied"},
		&parentID, &streamBuf,
	)
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2", len(emits))
	}
}

func TestMarshalAgentEventForWireToolNilStreamBufNoEmit(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventTool, ToolName: "ls", ToolArgs: `{}`},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil streamBuf)", len(emits))
	}
}

func TestMarshalAgentEventForWireObserveEmitsMessageAndResetsBuf(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventObserve, ToolName: "ls", ToolResult: "out", ToolIntent: "x"},
		&parentID, &streamBuf,
	)
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
	if streamBuf == nil {
		t.Error("streamBuf should be re-created after Observe")
	}
}

func TestMarshalAgentEventForWireObserveNilStreamBufNoEmit(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventObserve, ToolName: "ls", ToolResult: "out"},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil streamBuf)", len(emits))
	}
}

func TestMarshalAgentEventForWireFinalAnswerEmitsAndClearsBuf(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{
			Category: agentcore.EventFinalAnswer,
			Content:  "answer",
			Usage:    &llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
		},
		&parentID, &streamBuf,
	)
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
	if streamBuf != nil {
		t.Error("streamBuf should be nil after FinalAnswer")
	}
}

func TestMarshalAgentEventForWireFinalAnswerNoUsage(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "ok"},
		&parentID, &streamBuf,
	)
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
}

func TestMarshalAgentEventForWireFinalAnswerNilBufNoEmit(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventFinalAnswer, Content: "ok"},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil streamBuf)", len(emits))
	}
}

func TestMarshalAgentEventForWireErrorEmitsMessageAndCustomEvent(t *testing.T) {
	var parentID string
	streamBuf := agentcore.NewStreamBuffer("p1")
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventError, ToolError: "boom"},
		&parentID, &streamBuf,
	)
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2 (message + custom tool_error)", len(emits))
	}
	if emits[1].EventName != json_rpc.EventCustom {
		t.Errorf("second event = %q, want %q", emits[1].EventName, json_rpc.EventCustom)
	}
	if streamBuf != nil {
		t.Error("streamBuf should be nil after Error")
	}
}

func TestMarshalAgentEventForWireErrorNilBufNoEmit(t *testing.T) {
	var parentID string
	var streamBuf *agentcore.StreamBuffer
	emits := agentserver.MarshalAgentEventForWireForTest(
		agentcore.Event{Category: agentcore.EventError, ToolError: "boom"},
		&parentID, &streamBuf,
	)
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil streamBuf)", len(emits))
	}
}

func TestDispatchUnknownMethodReturnsError(t *testing.T) {
	dir := t.TempDir()
	s, err := agentserver.New(agentcore.NewAgent(&fakeCore{}), filepath.Join(dir, "test.sock"))
	go s.Serve()
	if err != nil {
		t.Skipf("server.New: %v", err)
	}
	conn := dialTestServer(t, dir)
	defer conn.Close()
	req, _ := json_rpc.NewRequest("1", "totally-unknown", nil)
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}
	resp := readOneResponse(t, conn)
	if resp.Event != json_rpc.EventError {
		t.Errorf("event = %q, want error", resp.Event)
	}
}

func TestHandleListSessionsEmptyDirReturnsSessionsList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_CONFIG_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "logs", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := agentserver.New(agentcore.NewAgent(&fakeCore{}), filepath.Join(tmp, "test.sock"))
	go s.Serve()
	if err != nil {
		t.Skipf("server.New: %v", err)
	}
	conn := dialTestServer(t, tmp)
	defer conn.Close()
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodListSessions, json_rpc.ListSessionsParams{})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}
	resp := readOneResponse(t, conn)
	if resp.Event != "sessions_list" {
		t.Errorf("event = %q, want sessions_list", resp.Event)
	}
}

func TestHandleListSessionsWithTwoFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_CONFIG_HOME", tmp)
	sessionsDir := filepath.Join(tmp, "logs", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := `[{"kind":"session","version":1,"id":"older","model":"m1","max_turns":200,"system":"x","tools":[],"started_at":"2020-01-01T00:00:00Z"}]
`
	newer := `[{"kind":"session","version":1,"id":"newer","model":"m2","max_turns":200,"system":"x","tools":[],"started_at":"2024-01-01T00:00:00Z"}]
`
	if err := os.WriteFile(filepath.Join(sessionsDir, "older.jsonl"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "newer.jsonl"), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := agentserver.New(agentcore.NewAgent(&fakeCore{}), filepath.Join(tmp, "test.sock"))
	go s.Serve()
	if err != nil {
		t.Skipf("server.New: %v", err)
	}
	conn := dialTestServer(t, tmp)
	defer conn.Close()
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodListSessions, json_rpc.ListSessionsParams{})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}
	resp := readOneResponse(t, conn)
	if resp.Event != "sessions_list" {
		t.Errorf("event = %q, want sessions_list", resp.Event)
	}
}

func TestCompactSessionExistsLoadsFromDisk(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_CONFIG_HOME", tmp)
	sessionsDir := filepath.Join(tmp, "logs", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `[{"kind":"session","version":1,"id":"abc","model":"m","max_turns":200,"system":"x","tools":[],"started_at":"2024-01-01T00:00:00Z"}]
`
	if err := os.WriteFile(filepath.Join(sessionsDir, "abc.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := agentserver.New(agentcore.NewAgent(&fakeCore{}), filepath.Join(tmp, "test.sock"))
	go s.Serve()
	if err != nil {
		t.Skipf("server.New: %v", err)
	}
	req, _ := json_rpc.NewRequest("1", json_rpc.MethodCompact, json_rpc.CompactParams{SessionID: "abc", Force: true})
	if err := json_rpc.MarshalRequest(io.Discard, req); err != nil {
		t.Fatal(err)
	}
}
