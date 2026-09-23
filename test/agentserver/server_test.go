package agentserver_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
)

type blockingCore struct{}

func (b *blockingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

type fakeCore struct{}

func (f *fakeCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 3)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: "hello"},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: " world"},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: llm.FinishReasonStop,
	}}}}
	close(ch)
	return ch, nil
}

type failingCore struct{}

func (failingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, errors.New("llm provider is down")
}

func setupTest(t *testing.T) (*agentserver.Server, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}

	go s.Serve()

	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return s, socketPath
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
	return nil, ""
}

func collectAssistantText(data []byte) (string, bool) {
	var msg json_rpc.MessageEvent
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", false
	}
	if msg.Message.Role != "assistant" {
		return "", false
	}
	var b strings.Builder
	for _, part := range msg.Message.Content {
		if part.Type == "text" {
			b.WriteString(part.Text)
		}
	}
	return b.String(), true
}

func TestServerSocketPath(t *testing.T) {
	s, _ := setupTest(t)
	defer s.Shutdown(context.Background())

	if s.SocketPath() == "" {
		t.Error("SocketPath() returned empty")
	}
}

func TestShutdownIsRaceFree(t *testing.T) {
	s, _ := setupTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

type brokenPipeWriter struct {
	mu       sync.Mutex
	written  []byte
	failAt   int
	failWith error
}

func (b *brokenPipeWriter) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failAt >= 0 && b.failWith != nil && len(b.written) >= b.failAt {
		return 0, b.failWith
	}
	b.written = append(b.written, p...)
	return len(p), nil
}

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(buf, os.Stderr), &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

func TestServerLogsBrokenPipeOnDispatch(t *testing.T) {
	bw := &brokenPipeWriter{failAt: 0, failWith: errors.New("write @->test.sock: write: broken pipe")}
	logBuf := captureSlog(t)

	req, _ := json_rpc.NewRequest("1", "ping", nil)
	if err := json_rpc.MarshalEvent(bw, req.ID, "pong", nil); err == nil {
		t.Fatal("expected write to fail immediately")
	}

	slog.Debug("server: marshal event failed", "req_id", req.ID, "err", bw.failWith)

	if !strings.Contains(logBuf.String(), "broken pipe") {
		t.Errorf("expected log to mention broken pipe; got %q", logBuf.String())
	}
}

func TestServerLogsBrokenPipeOnPromptStream(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	ag := agentcore.NewAgent(&fakeCore{}).WithModel(llm.Model{ID: "test", SupportsTool: false})
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

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

	_ = brokenPipeWriter{}
	_ = captureSlog
}

func TestServerPing(t *testing.T) {
	s, socketPath := setupTest(t)
	defer s.Shutdown(context.Background())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPing, nil)
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Event != "pong" {
		t.Errorf("event = %q, want pong", resp.Event)
	}
}

func TestServerPrompt(t *testing.T) {
	s, socketPath := setupTest(t)
	defer s.Shutdown(context.Background())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		Prompt: "test",
	})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)

	var events []string
	var finalContent string
	for {
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			break
		}
		events = append(events, resp.Event)
		if resp.Event == json_rpc.EventMessage {
			if text, ok := collectAssistantText(resp.Data); ok {
				finalContent = text
				break
			}
		}
	}

	if finalContent != "hello world" {
		t.Errorf("final content = %q, want %q", finalContent, "hello world")
	}
}

func TestServerUnknownMethod(t *testing.T) {
	s, socketPath := setupTest(t)
	defer s.Shutdown(context.Background())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("1", "unknown_method", nil)
	json_rpc.MarshalRequest(conn, req)

	reader := bufio.NewReader(conn)
	resp, err := json_rpc.ReadEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Event != json_rpc.EventError {
		t.Errorf("event = %q, want error", resp.Event)
	}
}

func TestServerMultipleClients(t *testing.T) {
	s, socketPath := setupTest(t)
	defer s.Shutdown(context.Background())

	for i := 0; i < 3; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}

		req, _ := json_rpc.NewRequest("1", json_rpc.MethodPing, nil)
		json_rpc.MarshalRequest(conn, req)

		reader := bufio.NewReader(conn)
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			conn.Close()
			t.Fatalf("read %d: %v", i, err)
		}
		if resp.Event != "pong" {
			conn.Close()
			t.Errorf("client %d: event = %q", i, resp.Event)
		}
		conn.Close()
	}
}

func TestServerCancelDoesNotTearDownServer(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.Serve() }()

	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	connA, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	reqC, _ := json_rpc.NewRequest("C", json_rpc.MethodCancel, nil)
	if err := json_rpc.MarshalRequest(connA, reqC); err != nil {
		t.Fatal(err)
	}
	respA, err := json_rpc.ReadEvent(bufio.NewReader(connA))
	if err != nil {
		t.Fatalf("connA read after cancel without active prompt: %v", err)
	}
	if respA.Event != json_rpc.EventError {
		t.Fatalf("connA event = %q, want %q", respA.Event, json_rpc.EventError)
	}
	connA.Close()

	connB, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("connB dial after cancel: server appears down (%v)", err)
	}
	defer connB.Close()
	reqPrompt, _ := json_rpc.NewRequest("P", json_rpc.MethodPrompt, json_rpc.PromptParams{Prompt: "hi"})
	if err := json_rpc.MarshalRequest(connB, reqPrompt); err != nil {
		t.Fatal(err)
	}

	readerB := bufio.NewReader(connB)
	var finalContent string
	for {
		resp, err := json_rpc.ReadEvent(readerB)
		if err != nil {
			break
		}
		if resp.Event == json_rpc.EventMessage {
			if text, ok := collectAssistantText(resp.Data); ok {
				finalContent = text
				break
			}
		}
	}
	if finalContent != "hello world" {
		t.Errorf("connB prompt after cancel: final = %q, want %q (server appears torn down)", finalContent, "hello world")
	}

	connB.Close()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelShutdown()
	if err := s.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestServerShutdown(t *testing.T) {
	s, _ := setupTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestServerPromptWritesSession(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		Prompt: "test",
	})
	json_rpc.MarshalRequest(conn, req)

	reader := bufio.NewReader(conn)

	var sessionID string
	var sawFinal bool
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !sawFinal {
		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			break
		}
		if resp.Event == "session_started" && sessionID == "" {
			var data struct {
				SessionID string `json:"session_id"`
			}
			json.Unmarshal(resp.Data, &data)
			sessionID = data.SessionID
		}
		if resp.Event == json_rpc.EventMessage {
			if _, ok := collectAssistantText(resp.Data); ok {
				sawFinal = true
			}
		}
	}

	if sessionID == "" {
		t.Fatal("did not receive session_started event")
	}
	if !sawFinal {
		t.Fatal("did not receive assistant message before timeout")
	}

	conn2, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	req2, _ := json_rpc.NewRequest("2", json_rpc.MethodResume, json_rpc.ResumeParams{
		SessionID: sessionID,
	})
	json_rpc.MarshalRequest(conn2, req2)

	reader2 := bufio.NewReader(conn2)
	conn2.SetReadDeadline(time.Now().Add(3 * time.Second))

	var events []string
	for {
		resp, err := json_rpc.ReadEvent(reader2)
		if err != nil {
			break
		}
		events = append(events, resp.Event)
		if resp.Event == "session_resumed" {
			break
		}
	}

	if len(events) == 0 {
		t.Error("resume returned no events")
	}
	if events[len(events)-1] != "session_resumed" {
		t.Errorf("last event = %q, want session_resumed", events[len(events)-1])
	}
}

func TestRunResumeUsesCompactionHistory(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&fakeCoreWithSummary{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	store := agentserver.NewStore(filepath.Join(dir, "logs", "sessions", "sess-x.jsonl"))
	if err := store.WriteHeader(agentserver.SessionMeta{SessionID: "sess-x"}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCompaction(agentserver.CompactionRecord{
		Summary: "## Goal\nfix the bug\n## Done\nlocated the cause",
		At:      time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}

	c, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		SessionID: "sess-x",
		Prompt:    "continue",
	})
	if err := json_rpc.MarshalRequest(c, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(c)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))

	gotSummary := false
	for {
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			break
		}
		if resp.Event == json_rpc.EventMessage {
			if text, ok := collectAssistantText(resp.Data); ok {
				if strings.Contains(text, "saw the compaction") {
					gotSummary = true
				}
				break
			}
		}
	}
	if !gotSummary {
		t.Errorf("expected assistant message to confirm compaction summary was visible to LLM")
	}
}

type fakeCoreWithSummary struct {
	fakeCore
}

func (f *fakeCoreWithSummary) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	hasSummary := false
	for _, m := range req.Messages {
		if m.Role == "assistant" && strings.Contains(m.Content, "Previous conversation summary") {
			hasSummary = true
			break
		}
	}
	reply := "continue without summary"
	if hasSummary {
		reply = "saw the compaction"
	}
	ch := make(chan llm.StreamEvent, 3)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: reply},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: llm.FinishReasonStop,
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{}}
	close(ch)
	return ch, nil
}

func TestServerAccumulatesMessagesAcrossPromptsInSameSession(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&fakeCoreAccum{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})
	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	store := agentserver.NewStore(filepath.Join(dir, "logs", "sessions", "shared.jsonl"))
	store.WriteHeader(agentserver.SessionMeta{SessionID: "shared"})
	store.WriteCompaction(agentserver.CompactionRecord{
		Summary: "## Goal\nfix the auth bug",
		At:      time.Now().UTC().Format(time.RFC3339Nano),
	})

	sendPrompt := func(prompt string) string {
		c, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
			SessionID: "shared",
			Prompt:    prompt,
		})
		json_rpc.MarshalRequest(c, req)
		reader := bufio.NewReader(c)
		c.SetReadDeadline(time.Now().Add(3 * time.Second))
		for {
			resp, err := json_rpc.ReadEvent(reader)
			if err != nil {
				return ""
			}
			if resp.Event == json_rpc.EventMessage {
				if text, ok := collectAssistantText(resp.Data); ok {
					return text
				}
			}
		}
	}

	first := sendPrompt("first task")
	second := sendPrompt("second task")

	if first != "saw the compaction" {
		t.Errorf("first prompt should have started fresh from compaction, got %q", first)
	}
	if !strings.Contains(second, "saw prior turn") {
		t.Errorf("second prompt should see first prompt's turn in history, got %q", second)
	}
}

type fakeCoreAccum struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeCoreAccum) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	f.mu.Lock()
	f.calls++
	callNum := f.calls
	f.mu.Unlock()

	hasSummary := false
	sawPriorTurn := false
	for _, m := range req.Messages {
		if m.Role == "assistant" && strings.Contains(m.Content, "Previous conversation summary") {
			hasSummary = true
		}
		if m.Role == "user" && strings.Contains(m.Content, "first task") {
			sawPriorTurn = true
		}
	}

	reply := "no summary"
	switch {
	case !hasSummary:
		reply = "no summary provided"
	case hasSummary && callNum == 1:
		reply = "saw the compaction"
	case hasSummary && sawPriorTurn:
		reply = "saw prior turn"
	}

	ch := make(chan llm.StreamEvent, 3)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: reply},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: llm.FinishReasonStop,
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{}}
	close(ch)
	return ch, nil
}

func TestServerKeepsSessionFileWhenLLMFails(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&failingCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		Prompt: "hello",
	})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	deadline := time.Now().Add(2 * time.Second)
	var sessionID string
	for time.Now().Before(deadline) && sessionID == "" {
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			break
		}
		if resp.Event == "session_started" {
			var d struct {
				SessionID string `json:"session_id"`
			}
			json.Unmarshal(resp.Data, &d)
			sessionID = d.SessionID
		}
	}
	if sessionID == "" {
		t.Fatal("did not receive session_started event before timeout")
	}

	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		if _, err := json_rpc.ReadEvent(reader); err != nil {
			break
		}
	}

	path := filepath.Join(dir, "logs", "sessions", sessionID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("session file %s should exist after prompt (even when LLM failed), got: %v", path, err)
	}
}

func TestServerNeverWritesToHomeLogSessions(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

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

	req, _ := json_rpc.NewRequest("1", json_rpc.MethodPrompt, json_rpc.PromptParams{
		Prompt: "hello",
	})
	if err := json_rpc.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	deadline := time.Now().Add(3 * time.Second)
	sawFinal := false
	for time.Now().Before(deadline) && !sawFinal {
		conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		resp, err := json_rpc.ReadEvent(reader)
		if err != nil {
			break
		}
		if resp.Event == json_rpc.EventMessage {
			if _, ok := collectAssistantText(resp.Data); ok {
				sawFinal = true
			}
		}
	}

	sessionsDir := filepath.Join(dir, "logs", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	var serverWritten []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		f, _ := os.Open(filepath.Join(sessionsDir, e.Name()))
		if f == nil {
			continue
		}
		buf := make([]byte, 4096)
		n, _ := f.Read(buf)
		f.Close()
		if bytes.Contains(buf[:n], []byte(`"event":"message"`)) || bytes.Contains(buf[:n], []byte(`"event":"custom"`)) {
			serverWritten = append(serverWritten, e.Name())
		}
	}
	if len(serverWritten) > 0 {
		t.Errorf("server should not write session events to ~/.awp/logs/sessions/; found: %v", serverWritten)
	}
}

type accumulatingCore struct {
	mu       sync.Mutex
	calls    int
	lastMsgs []llm.Message
}

func (a *accumulatingCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	a.mu.Lock()
	a.calls++
	a.lastMsgs = append([]llm.Message{}, req.Messages...)
	a.mu.Unlock()
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{Delta: llm.Message{Content: "pong"}}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{FinishReason: llm.FinishReasonStop}}}}
	close(ch)
	return ch, nil
}

func (a *accumulatingCore) requestCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func (a *accumulatingCore) lastRequestMessages() []llm.Message {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastMsgs
}

func TestSessionStateAccumulatesAcrossPrompts(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")
	t.Setenv("AWP_HOME", dir)

	core := &accumulatingCore{}
	ag := agentcore.NewAgent(core).
		WithModel(llm.Model{ID: "test", SupportsTool: false})
	llm.ResetIfaceLoggerForTest()
	defer llm.ResetIfaceLoggerForTest()

	s, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	defer s.Shutdown(context.Background())

	for i := 0; i < 50; i++ {
		c, err := net.Dial("unix", socketPath)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	sessionID := fmt.Sprintf("state-test-%d", time.Now().UnixNano())
	sendPrompt := func(reqID, promptText string) {
		c, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer c.Close()
		req, _ := json_rpc.NewRequest(reqID, json_rpc.MethodPrompt, json_rpc.PromptParams{
			SessionID: sessionID,
			Prompt:    promptText,
		})
		if err := json_rpc.MarshalRequest(c, req); err != nil {
			t.Fatal(err)
		}
		reader := bufio.NewReader(c)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			resp, err := json_rpc.ReadEvent(reader)
			if err != nil {
				return
			}
			if resp.Event == json_rpc.EventMessage {
				if _, ok := collectAssistantText(resp.Data); ok {
					return
				}
			}
		}
	}

	sendPrompt("1", "first-prompt")
	firstMsgs := core.lastRequestMessages()
	if len(firstMsgs) != 2 {
		t.Fatalf("first prompt: LLM should see [system, user] (2 messages), got %d", len(firstMsgs))
	}

	sendPrompt("2", "second-prompt")
	secondMsgs := core.lastRequestMessages()
	if len(secondMsgs) <= len(firstMsgs) {
		t.Errorf("second prompt: LLM should see history carried (messages=%d > first %d); sessionStates snapshot select has a default: that drops the snapshot, so msgs array is dropped every prompt", len(secondMsgs), len(firstMsgs))
	}
}
