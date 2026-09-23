package agentclient_test

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
)

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

func setupTestServer(t *testing.T) (*agentserver.Server, string) {
	t.Helper()
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "test.sock")

	ag := agentcore.NewAgent(&fakeCore{}).
		WithModel(llm.Model{ID: "test", SupportsTool: false})

	srv, err := agentserver.New(ag, socketPath)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()

	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return srv, socketPath
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return nil, ""
}

func TestDial(t *testing.T) {
	_, socketPath := setupTestServer(t)

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
}

func TestDialFailsForMissingServer(t *testing.T) {
	_, err := agentclient.Dial("/nonexistent.sock")
	if err == nil {
		t.Error("expected dial to fail")
	}
}

func TestCloseIdempotent(t *testing.T) {
	_, socketPath := setupTestServer(t)

	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Close(); err != nil {
		t.Errorf("first Close returned error: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close returned error: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("third Close returned error: %v", err)
	}
}

func TestPing(t *testing.T) {
	_, socketPath := setupTestServer(t)
	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		t.Errorf("ping: %v", err)
	}
}

func TestPromptStreamEvents(t *testing.T) {
	_, socketPath := setupTestServer(t)
	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	events, err := c.Prompt(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}

	var kinds []string
	var finalContent string
	for ev := range events {
		kinds = append(kinds, ev.Kind)
		if ev.Kind == json_rpc.EventFinalAnswer {
			var data struct {
				Content string `json:"content"`
			}
			json.Unmarshal(ev.Data, &data)
			finalContent = data.Content
		}
	}

	if finalContent != "hello world" {
		t.Errorf("content = %q, want hello world", finalContent)
	}

	hasThought := false
	hasChunk := false
	hasFinal := false
	for _, k := range kinds {
		if k == json_rpc.EventThoughtStart {
			hasThought = true
		}
		if k == json_rpc.EventThoughtChunk {
			hasChunk = true
		}
		if k == json_rpc.EventFinalAnswer {
			hasFinal = true
		}
	}
	if !hasThought || !hasChunk || !hasFinal {
		t.Errorf("missing events: thought=%v chunk=%v final=%v", hasThought, hasChunk, hasFinal)
	}
}

func TestParseEventData(t *testing.T) {
	data := []byte(`{"name":"bash","args":"pwd"}`)
	var out struct {
		Name string `json:"name"`
		Args string `json:"args"`
	}
	if err := agentclient.ParseEventData(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "bash" || out.Args != "pwd" {
		t.Errorf("got %+v", out)
	}
}

func TestSendPromptOneShot(t *testing.T) {
	_, socketPath := setupTestServer(t)

	events, err := agentclient.SendPrompt(context.Background(), socketPath, "", "test")
	if err != nil {
		t.Fatal(err)
	}

	var finalContent string
	for ev := range events {
		if ev.Kind == json_rpc.EventFinalAnswer {
			var data struct {
				Content string `json:"content"`
			}
			json.Unmarshal(ev.Data, &data)
			finalContent = data.Content
		}
	}
	if finalContent != "hello world" {
		t.Errorf("content = %q, want hello world", finalContent)
	}
}

func TestSendPromptClosesSocketAfterStreamEnds(t *testing.T) {
	_, socketPath := setupTestServer(t)

	events, err := agentclient.SendPrompt(context.Background(), socketPath, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	c, err := agentclient.Dial(socketPath)
	if err != nil {
		t.Fatalf("second Dial after SendPrompt returned: %v", err)
	}
	if err := c.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	c.Close()
}
