package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/vanpiyp/awp/internal/agent"
	"github.com/vanpiyp/awp/internal/llm"
	"github.com/vanpiyp/awp/internal/protocol"
	"github.com/vanpiyp/awp/internal/server"
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

func setupTest(t *testing.T) (*server.Server, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	ag := agent.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := server.New(ag, socketPath)
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

func TestServerSocketPath(t *testing.T) {
	s, _ := setupTest(t)
	defer s.Shutdown(context.Background())

	if s.SocketPath() == "" {
		t.Error("SocketPath() returned empty")
	}
}

func TestServerPing(t *testing.T) {
	s, socketPath := setupTest(t)
	defer s.Shutdown(context.Background())

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req, _ := protocol.NewRequest("1", protocol.MethodPing, nil)
	if err := protocol.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)
	resp, err := protocol.ReadEvent(reader)
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

	req, _ := protocol.NewRequest("1", protocol.MethodPrompt, protocol.PromptParams{
		Prompt: "test",
	})
	if err := protocol.MarshalRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(conn)

	var events []string
	var finalContent string
	for {
		resp, err := protocol.ReadEvent(reader)
		if err != nil {
			break
		}
		events = append(events, resp.Event)
		if resp.Event == protocol.EventFinalAnswer {
			var data struct {
				Content string `json:"content"`
			}
			json.Unmarshal(resp.Data, &data)
			finalContent = data.Content
			break
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

	req, _ := protocol.NewRequest("1", "unknown_method", nil)
	protocol.MarshalRequest(conn, req)

	reader := bufio.NewReader(conn)
	resp, err := protocol.ReadEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Event != protocol.EventError {
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

		req, _ := protocol.NewRequest("1", protocol.MethodPing, nil)
		protocol.MarshalRequest(conn, req)

		reader := bufio.NewReader(conn)
		resp, err := protocol.ReadEvent(reader)
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

	ag := agent.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := server.New(ag, socketPath)
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
	reqC, _ := protocol.NewRequest("C", protocol.MethodCancel, nil)
	if err := protocol.MarshalRequest(connA, reqC); err != nil {
		t.Fatal(err)
	}
	respA, err := protocol.ReadEvent(bufio.NewReader(connA))
	if err != nil {
		t.Fatalf("connA read after cancel without active prompt: %v", err)
	}
	if respA.Event != protocol.EventError {
		t.Fatalf("connA event = %q, want %q", respA.Event, protocol.EventError)
	}
	connA.Close()

	connB, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("connB dial after cancel: server appears down (%v)", err)
	}
	defer connB.Close()
	reqPrompt, _ := protocol.NewRequest("P", protocol.MethodPrompt, protocol.PromptParams{Prompt: "hi"})
	if err := protocol.MarshalRequest(connB, reqPrompt); err != nil {
		t.Fatal(err)
	}

	readerB := bufio.NewReader(connB)
	var finalContent string
	for {
		resp, err := protocol.ReadEvent(readerB)
		if err != nil {
			break
		}
		if resp.Event == protocol.EventFinalAnswer {
			var data struct {
				Content string `json:"content"`
			}
			json.Unmarshal(resp.Data, &data)
			finalContent = data.Content
			break
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

	ag := agent.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	s, err := server.New(ag, socketPath)
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

	req, _ := protocol.NewRequest("1", protocol.MethodPrompt, protocol.PromptParams{
		Prompt: "test",
	})
	protocol.MarshalRequest(conn, req)

	reader := bufio.NewReader(conn)

	var sessionID string
	for {
		resp, err := protocol.ReadEvent(reader)
		if err != nil {
			break
		}
		if resp.Event == "session_started" {
			var data struct {
				SessionID string `json:"session_id"`
			}
			json.Unmarshal(resp.Data, &data)
			sessionID = data.SessionID
			break
		}
	}

	if sessionID == "" {
		t.Fatal("did not receive session_started event")
	}

	conn2, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()

	req2, _ := protocol.NewRequest("2", protocol.MethodResume, protocol.ResumeParams{
		SessionID: sessionID,
	})
	protocol.MarshalRequest(conn2, req2)

	reader2 := bufio.NewReader(conn2)

	var events []string
	for {
		resp, err := protocol.ReadEvent(reader2)
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
