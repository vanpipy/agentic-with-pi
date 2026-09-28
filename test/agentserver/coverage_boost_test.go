package agentserver_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/paths"
)

func writeCovSessionJSONL(t *testing.T, lines []string) string {
	t.Helper()
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID := "cov-" + filepath.Base(t.Name()) + "-" + time.Now().Format("150405.000000000")
	path := filepath.Join(sessionsDir, sessionID+".jsonl")
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return sessionID
}

func waitForSocketReady(t *testing.T, socketPath string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
}

func writeRawRequest(t *testing.T, conn net.Conn, raw string) {
	t.Helper()
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}

func dialAndWrite(t *testing.T, socketPath, rawReq string) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte(rawReq)); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn
}

func TestHandlePingLogsDebugOnWriteFailure(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json_rpc.NewRequest("ping-1", json_rpc.MethodPing, nil)
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "method=ping") && strings.Contains(logs, "stage=ping_pong") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug 'stage=ping_pong' when client closes mid-ping; got log:\n%s", logBuf.String())
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestHandleCancelSendsAckOnActivePrompt(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&blockingCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	defer func() { _ = s.Shutdown(t.Context()) }()
	waitForSocketReady(t, socketPath)

	connA, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer connA.Close()

	promptReq, _ := json_rpc.NewRequest("ACTIVE", json_rpc.MethodPrompt, json_rpc.PromptParams{Prompt: "long"})
	if err := json_rpc.MarshalRequest(connA, promptReq); err != nil {
		t.Fatal(err)
	}

	drainConnA := make(chan string, 16)
	go func() {
		readerA := bufio.NewReader(connA)
		for {
			connA.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			resp, err := json_rpc.ReadEvent(readerA)
			if err != nil {
				close(drainConnA)
				return
			}
			drainConnA <- resp.Event
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	sawSessionStarted := false
	for !sawSessionStarted && time.Now().Before(deadline) {
		select {
		case ev := <-drainConnA:
			if ev == "session_started" {
				sawSessionStarted = true
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !sawSessionStarted {
		t.Fatal("did not receive session_started before timeout; prompt handler did not reach registerConnCancel")
	}

	connB, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer connB.Close()

	cancelReq, _ := json_rpc.NewRequest("ACTIVE", json_rpc.MethodCancel, json_rpc.CancelParams{})
	if err := json_rpc.MarshalRequest(connB, cancelReq); err != nil {
		t.Fatal(err)
	}

	readerB := bufio.NewReader(connB)
	connB.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := json_rpc.ReadEvent(readerB)
	if err != nil {
		t.Fatalf("read cancel response: %v", err)
	}
	if resp.Event != json_rpc.EventCancelAck {
		t.Fatalf("event = %q, want %q", resp.Event, json_rpc.EventCancelAck)
	}
}

func TestHandleCancelSendsErrorOnUnknownID(t *testing.T) {
	s, socketPath := setupTest(t)
	defer s.Shutdown(context.Background())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("never-active", json_rpc.MethodCancel, json_rpc.CancelParams{})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
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
	if !strings.Contains(d.Error, "no active prompt") {
		t.Errorf("error = %q, expected to mention 'no active prompt'", d.Error)
	}
}

func TestHandleResumePlaysCustomMessageEvents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "resume-custom-msg"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")

	customMsg := json_rpc.CustomMessageEvent{
		ID:         "0190c0de-0001-7c8a-9000-000000000001",
		ParentID:   "",
		Timestamp:  "2026-09-23T14:00:00.000Z",
		CustomType: "tool_result_summary",
		Content:    "all good",
		Display:    true,
	}
	msgRaw, _ := json.Marshal(customMsg)
	wrapped, _ := json.Marshal(struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "custom_message", Version: 2, Entry: msgRaw})

	headerLine := `{"kind":"session","version":2,"id":"` + sessionID + `","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-23T13:00:00.000Z"}`
	lines := []string{headerLine, string(wrapped)}
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

	req, _ := json_rpc.NewRequest("R-CUSTOM", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	gotEvents := readAllWireEvents(t, conn)

	want := []string{json_rpc.EventCustomMessage, "session_resumed"}
	if len(gotEvents) != len(want) {
		t.Fatalf("events len = %d, want %d (got=%v)", len(gotEvents), len(want), gotEvents)
	}
	for i, w := range want {
		if gotEvents[i] != w {
			t.Errorf("event[%d] = %q, want %q", i, gotEvents[i], w)
		}
	}
}

func TestHandleResumeInvalidParamsReturnsError(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	rawReq := `{"jsonrpc":"2.0","id":"R-INVALID","method":"resume","params":"not-an-object"}` + "\n"
	writeRawRequest(t, conn, rawReq)

	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
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
	if !strings.Contains(d.Error, "invalid params") {
		t.Errorf("error = %q, expected to mention 'invalid params'", d.Error)
	}
}

func TestHandleCompactReactiveStrategyEmitsResult(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "compact-reactive"
	store := agentserver.NewStore(filepath.Join(sessionsDir, sessionID+".jsonl"))
	if err := store.WriteHeader(agentserver.SessionMeta{SessionID: sessionID, Model: "test"}); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("CR1", json_rpc.MethodCompact, json_rpc.CompactParams{
		SessionID: sessionID,
		Force:     false,
	})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	gotEvents := readCompactEvents(t, conn)

	var sawRequested, sawResult bool
	var strategy string
	var payloadRaw []byte
	for _, ev := range gotEvents {
		switch ev.event {
		case json_rpc.EventCustom:
			sawRequested = true
			var ce json_rpc.CustomEvent
			if err := json.Unmarshal(ev.data, &ce); err != nil {
				t.Fatalf("custom payload unmarshal: %v", err)
			}
			if ce.CustomType != "compaction_requested" {
				t.Errorf("custom_type = %q, want %q", ce.CustomType, "compaction_requested")
			}
			var data struct {
				Strategy string `json:"strategy"`
				Force    bool   `json:"force"`
			}
			if err := json.Unmarshal(ce.Data, &data); err != nil {
				t.Fatalf("compaction_requested data: %v", err)
			}
			strategy = data.Strategy
			if data.Force {
				t.Errorf("force should be false")
			}
		case "compact_result":
			sawResult = true
			payloadRaw = ev.data
		}
	}
	if !sawRequested {
		t.Errorf("expected compaction_requested custom event")
	}
	if !sawResult {
		t.Errorf("expected compact_result event")
	}
	if strategy != "reactive" {
		t.Errorf("strategy = %q, want %q", strategy, "reactive")
	}
	var result json_rpc.CompactResult
	if err := json.Unmarshal(payloadRaw, &result); err != nil {
		t.Fatalf("compact_result unmarshal: %v", err)
	}
	if result.Strategy != "reactive" {
		t.Errorf("CompactResult.Strategy = %q, want %q", result.Strategy, "reactive")
	}
	if result.Triggered {
		t.Errorf("Triggered should be false (server stub returns triggered=false)")
	}
}

func TestHandleCompactForcedStrategyEmitsResult(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "compact-forced"
	store := agentserver.NewStore(filepath.Join(sessionsDir, sessionID+".jsonl"))
	if err := store.WriteHeader(agentserver.SessionMeta{SessionID: sessionID, Model: "test"}); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("CF1", json_rpc.MethodCompact, json_rpc.CompactParams{
		SessionID: sessionID,
		Force:     true,
	})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	gotEvents := readCompactEvents(t, conn)

	var strategy string
	for _, ev := range gotEvents {
		if ev.event != json_rpc.EventCustom {
			continue
		}
		var ce json_rpc.CustomEvent
		if err := json.Unmarshal(ev.data, &ce); err != nil {
			continue
		}
		if ce.CustomType != "compaction_requested" {
			continue
		}
		var data struct {
			Strategy string `json:"strategy"`
			Force    bool   `json:"force"`
		}
		_ = json.Unmarshal(ce.Data, &data)
		strategy = data.Strategy
		if !data.Force {
			t.Errorf("force should be true when params.Force=true")
		}
	}
	if strategy != "forced" {
		t.Errorf("strategy = %q, want %q (params.Force=true)", strategy, "forced")
	}
}

func TestHandleCompactInvalidParamsReturnsError(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	rawReq := `{"jsonrpc":"2.0","id":"C-INVALID","method":"compact","params":42}` + "\n"
	writeRawRequest(t, conn, rawReq)

	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
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
	if !strings.Contains(d.Error, "invalid params") {
		t.Errorf("error = %q, expected to mention 'invalid params'", d.Error)
	}
}

type compactWireEvent struct {
	event string
	data  json.RawMessage
}

func readCompactEvents(t *testing.T, conn net.Conn) []compactWireEvent {
	t.Helper()
	reader := bufio.NewReader(conn)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var out []compactWireEvent
	for {
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			return out
		}
		out = append(out, compactWireEvent{event: resp.Event, data: resp.Data})
		if resp.Event == "compact_result" {
			return out
		}
	}
}

func TestNewReturnsErrorWhenListenFails(t *testing.T) {
	ag := agentcore.NewAgent(&handlerFakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, "")
	if err == nil {
		if s != nil {
			_ = s.Shutdown(context.Background())
		}
		t.Fatal("expected New(ag, \"\") to fail with empty path error")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("error = %q, expected to mention 'listen'", err.Error())
	}
	if s != nil {
		t.Errorf("server should be nil on listen failure; got %v", s)
	}
}

func TestNewReturnsErrorWhenParentDirIsAFile(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parent, []byte("file-as-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(parent, "test.sock")

	ag := agentcore.NewAgent(&handlerFakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err == nil {
		if s != nil {
			_ = s.Shutdown(context.Background())
		}
		t.Fatal("expected New to fail when parent dir is a regular file")
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("error = %q, expected to mention 'listen'", err.Error())
	}
}

type concurrentBlockingCore struct {
	mu      sync.Mutex
	started int
}

func (c *concurrentBlockingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.LegacyStreamEvent, error) {
	c.mu.Lock()
	c.started++
	c.mu.Unlock()
	ch := make(chan llm.LegacyStreamEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func TestHandleCancelMultipleCancelsForSameIDOnlyOneWins(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&concurrentBlockingCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	defer func() { _ = s.Shutdown(t.Context()) }()
	waitForSocketReady(t, socketPath)

	connPrompt, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer connPrompt.Close()
	promptReq, _ := json_rpc.NewRequest("DUP-ID", json_rpc.MethodPrompt, json_rpc.PromptParams{Prompt: "long"})
	if err := json_rpc.MarshalRequest(connPrompt, promptReq); err != nil {
		t.Fatal(err)
	}

	drainPrompt := make(chan string, 16)
	go func() {
		reader := bufio.NewReader(connPrompt)
		for {
			connPrompt.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			resp, err := json_rpc.ReadEvent(reader)
			if err != nil {
				close(drainPrompt)
				return
			}
			drainPrompt <- resp.Event
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	saw := false
	for !saw && time.Now().Before(deadline) {
		select {
		case ev := <-drainPrompt:
			if ev == "session_started" {
				saw = true
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !saw {
		t.Fatal("did not receive session_started; prompt handler did not register cancel")
	}

	connCancel1, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer connCancel1.Close()
	cancelReq1, _ := json_rpc.NewRequest("DUP-ID", json_rpc.MethodCancel, json_rpc.CancelParams{})
	if err := json_rpc.MarshalRequest(connCancel1, cancelReq1); err != nil {
		t.Fatal(err)
	}
	reader1 := bufio.NewReader(connCancel1)
	connCancel1.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp1, err := json_rpc.ReadEvent(reader1)
	if err != nil {
		t.Fatalf("first cancel read: %v", err)
	}
	if resp1.Event != json_rpc.EventCancelAck {
		t.Errorf("first cancel event = %q, want %q", resp1.Event, json_rpc.EventCancelAck)
	}

	connCancel2, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer connCancel2.Close()
	cancelReq2, _ := json_rpc.NewRequest("DUP-ID", json_rpc.MethodCancel, json_rpc.CancelParams{})
	if err := json_rpc.MarshalRequest(connCancel2, cancelReq2); err != nil {
		t.Fatal(err)
	}
	reader2 := bufio.NewReader(connCancel2)
	connCancel2.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp2, err := json_rpc.ReadEvent(reader2)
	if err != nil {
		t.Fatalf("second cancel read: %v", err)
	}
	if resp2.Event != json_rpc.EventError {
		t.Errorf("second cancel event = %q, want %q (already popped)", resp2.Event, json_rpc.EventError)
	}
}

func TestHandleCancelErrorPathLogsDebugWhenWriteFails(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawCancel := `{"jsonrpc":"2.0","id":"NOACTIVE","method":"cancel","params":{}}` + "\n"
	conn := dialAndWrite(t, socketPath, rawCancel)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=cancel_no_active") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=cancel_no_active on write failure; got:\n%s", logBuf.String())
}

func TestHandleCancelSuccessPathLogsDebugWhenWriteFails(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&blockingCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()
	defer func() { _ = s.Shutdown(t.Context()) }()
	waitForSocketReady(t, socketPath)

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	connPrompt, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer connPrompt.Close()
	promptReq, _ := json_rpc.NewRequest("CWFAIL", json_rpc.MethodPrompt, json_rpc.PromptParams{Prompt: "long"})
	if err := json_rpc.MarshalRequest(connPrompt, promptReq); err != nil {
		t.Fatal(err)
	}

	drainPrompt := make(chan string, 16)
	go func() {
		reader := bufio.NewReader(connPrompt)
		for {
			connPrompt.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			resp, err := json_rpc.ReadEvent(reader)
			if err != nil {
				close(drainPrompt)
				return
			}
			drainPrompt <- resp.Event
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	saw := false
	for !saw && time.Now().Before(deadline) {
		select {
		case ev := <-drainPrompt:
			if ev == "session_started" {
				saw = true
			}
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !saw {
		t.Fatal("did not receive session_started; prompt handler did not register cancel")
	}

	rawCancel := `{"jsonrpc":"2.0","id":"CWFAIL","method":"cancel","params":{}}` + "\n"
	conn := dialAndWrite(t, socketPath, rawCancel)
	_ = conn.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=cancel_ack") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=cancel_ack on write failure; got:\n%s", logBuf.String())
}

func TestHandleCompactInvalidParamsWriteFailureLogsDebug(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawReq := `{"jsonrpc":"2.0","id":"C-INVALID-WF","method":"compact","params":42}` + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=compact_invalid_params") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=compact_invalid_params on write failure; got:\n%s", logBuf.String())
}

func TestHandleCompactMissingSessionWriteFailureLogsDebug(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawReq := `{"jsonrpc":"2.0","id":"C-MISSING-WF","method":"compact","params":{"session_id":"","force":true}}` + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=compact_missing_session") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=compact_missing_session on write failure; got:\n%s", logBuf.String())
}

func TestHandleCompactNotFoundWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawReq := `{"jsonrpc":"2.0","id":"C-NOTFOUND-WF","method":"compact","params":{"session_id":"missing-sess-xx","force":true}}` + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=compact_not_found") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=compact_not_found on write failure; got:\n%s", logBuf.String())
}

func TestHandleCompactResultWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "compact-result-wf"
	store := agentserver.NewStore(filepath.Join(sessionsDir, sessionID+".jsonl"))
	if err := store.WriteHeader(agentserver.SessionMeta{SessionID: sessionID, Model: "test"}); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawReq := `{"jsonrpc":"2.0","id":"C-RESULT-WF","method":"compact","params":{"session_id":"` + sessionID + `","force":true}}` + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=compact_result") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=compact_result on write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeInvalidParamsWriteFailureLogsDebug(t *testing.T) {
	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawReq := `{"jsonrpc":"2.0","id":"R-INVALID-WF","method":"resume","params":"bad"}` + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_invalid_params") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_invalid_params on write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeNotFoundWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	rawReq := `{"jsonrpc":"2.0","id":"R-NOTFOUND-WF","method":"resume","params":{"session_id":"missing-zzz-12345"}}` + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_not_found") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_not_found on write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeLegacyUnknownCategoryFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "legacy-unknown-category"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	const legacyJSONL = `{"kind":"session","version":1,"id":"legacy-unknown-category","model":"MiniMax-M3","max_turns":200,"system":"you are helpful","tools":["read"],"started_at":"2026-09-22T16:26:47.803329214Z"}
{"kind":"event","seq":1,"at":"2026-09-22T16:26:47.803436334Z","category":"some_unknown_category"}
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

	req, _ := json_rpc.NewRequest("R-LEGACY-FB", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	gotEvents := readAllWireEvents(t, conn)
	if len(gotEvents) < 2 {
		t.Fatalf("expected at least 2 wire events, got %v", gotEvents)
	}
	if gotEvents[0] != "some_unknown_category" {
		t.Errorf("wire event[0] = %q, want fallback to category %q", gotEvents[0], "some_unknown_category")
	}
	last := gotEvents[len(gotEvents)-1]
	if last != "session_resumed" {
		t.Errorf("last wire event = %q, want %q", last, "session_resumed")
	}
}

func TestHandleResumeLegacyEventWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "legacy-wf"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	const legacyJSONL = `{"kind":"session","version":1,"id":"legacy-wf","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-22T16:26:47.803329214Z"}
{"kind":"event","seq":1,"at":"2026-09-22T16:26:47.803436334Z","category":"final_answer","content":"done"}
`
	if err := os.WriteFile(sessionPath, []byte(legacyJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	req, _ := json_rpc.NewRequest("R-LEGACY-WF", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	raw, _ := json.Marshal(req)
	rawReq := string(raw) + "\n"
	conn := dialAndWrite(t, socketPath, rawReq)
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_stream") && strings.Contains(logs, "session_id="+sessionID) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_stream on write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeMessageWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "msg-wf"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	msg := json_rpc.MessageEvent{
		ID:        "0190a3b7-0001-7c8a-9000-000000000099",
		Timestamp: "2026-09-23T13:00:00.789Z",
		Message: json_rpc.Message{
			Role:    "user",
			Content: []json_rpc.MessageContentPart{{Type: "text", Text: "hi"}},
		},
	}
	raw, _ := json.Marshal(msg)
	wrapped, _ := json.Marshal(struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "message", Version: 2, Entry: raw})
	lines := []string{
		`{"kind":"session","version":2,"id":"msg-wf","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-23T13:00:00.000Z"}`,
		string(wrapped),
	}
	if err := os.WriteFile(sessionPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	req, _ := json_rpc.NewRequest("R-MSG-WF", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	rawReq, _ := json.Marshal(req)
	conn := dialAndWrite(t, socketPath, string(rawReq)+"\n")
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_stream") && strings.Contains(logs, "session_id=msg-wf") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_stream on message write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeCustomWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "custom-wf"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	ce := json_rpc.CustomEvent{
		ID:         "0190a3b7-0001-7c8a-9000-000000000077",
		Timestamp:  "2026-09-23T13:00:02.000Z",
		CustomType: "abort",
		Data:       json.RawMessage(`{"reason":"user cancelled"}`),
	}
	raw, _ := json.Marshal(ce)
	wrapped, _ := json.Marshal(struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "custom", Version: 2, Entry: raw})
	lines := []string{
		`{"kind":"session","version":2,"id":"custom-wf","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-23T13:00:00.000Z"}`,
		string(wrapped),
	}
	if err := os.WriteFile(sessionPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	req, _ := json_rpc.NewRequest("R-CUSTOM-WF", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	rawReq, _ := json.Marshal(req)
	conn := dialAndWrite(t, socketPath, string(rawReq)+"\n")
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_stream") && strings.Contains(logs, "session_id=custom-wf") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_stream on custom write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeCustomMessageWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "custom-msg-wf"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	cm := json_rpc.CustomMessageEvent{
		ID:         "0190c0de-0001-7c8a-9000-000000000055",
		Timestamp:  "2026-09-23T14:00:00.000Z",
		CustomType: "summary",
		Content:    "all good",
	}
	raw, _ := json.Marshal(cm)
	wrapped, _ := json.Marshal(struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "custom_message", Version: 2, Entry: raw})
	lines := []string{
		`{"kind":"session","version":2,"id":"custom-msg-wf","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-23T13:00:00.000Z"}`,
		string(wrapped),
	}
	if err := os.WriteFile(sessionPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	req, _ := json_rpc.NewRequest("R-CM-WF", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	rawReq, _ := json.Marshal(req)
	conn := dialAndWrite(t, socketPath, string(rawReq)+"\n")
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_stream") && strings.Contains(logs, "session_id=custom-msg-wf") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_stream on custom_message write failure; got:\n%s", logBuf.String())
}

func TestHandleResumeSessionResumedWriteFailureLogsDebug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AWP_HOME", dir)
	sessionsDir := paths.SessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionID := "resumed-wf"
	sessionPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	headerLine := `{"kind":"session","version":2,"id":"resumed-wf","model":"MiniMax-M3","max_turns":200,"system":"helpful","tools":["read"],"started_at":"2026-09-23T13:00:00.000Z"}`
	if err := os.WriteFile(sessionPath, []byte(headerLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, socketPath := startTestServerWithAgent(t)
	defer s.Shutdown(t.Context())

	logBuf := &safeBuffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(logBuf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	req, _ := json_rpc.NewRequest("R-DONE-WF", json_rpc.MethodResume, json_rpc.ResumeParams{SessionID: sessionID})
	rawReq, _ := json.Marshal(req)
	conn := dialAndWrite(t, socketPath, string(rawReq)+"\n")
	_ = conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		logs := logBuf.String()
		if strings.Contains(logs, "stage=resume_done") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("expected slog.Debug stage=resume_done on write failure; got:\n%s", logBuf.String())
}
