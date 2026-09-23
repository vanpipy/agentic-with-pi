package agentserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
)

type headerRecCore struct{}

func (c *headerRecCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 4)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Index: 0,
		Delta: llm.Message{Content: "ok"},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{FinishReason: llm.FinishReasonStop}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{}}
	close(ch)
	return ch, nil
}

func TestAgentHeaderRecognizableByServerLoad(t *testing.T) {
	sessionID := "115845c05e9754f4f3664bc71be621e6"
	core := &headerRecCore{}
	ag := agentcore.NewAgent(core).WithModel(llm.Model{ID: "test-model", SupportsTool: true}).WithSessionID(sessionID)

	buf := &bytes.Buffer{}
	ag.WithLogWriter(buf)

	ctx := context.Background()
	for range ag.RunStream(ctx, "hello") {
	}

	if buf.Len() == 0 {
		t.Fatal("expected agent to write a header, got empty buffer")
	}
	firstLine := buf.Bytes()
	if idx := bytes.IndexByte(firstLine, '\n'); idx >= 0 {
		firstLine = firstLine[:idx]
	}
	var first map[string]any
	if err := json.Unmarshal(firstLine, &first); err != nil {
		t.Fatalf("unmarshal first line: %v\nraw: %q", err, firstLine)
	}
	if first["kind"] != "session" {
		t.Fatalf("line[0].kind = %v, want session", first["kind"])
	}
	if id, _ := first["id"].(string); id != sessionID {
		t.Errorf("agent header id = %q, want %q (must equal the file/session id, not a random hex)", id, sessionID)
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := buf.WriteTo(f); err != nil {
		f.Close()
		t.Fatalf("write temp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatalf("agentserver.Load: %v", err)
	}
	if loaded.Meta.SessionID != sessionID {
		t.Errorf("agentserver.Load Meta.SessionID = %q, want %q (must match file stem so the session is locatable)", loaded.Meta.SessionID, sessionID)
	}
	if loaded.Meta.Model != "test-model" {
		t.Errorf("agentserver.Load Meta.Model = %q, want test-model", loaded.Meta.Model)
	}
}

func TestServerLoadAgentHeaderMissingModel(t *testing.T) {
	sessionID := "115845c05e9754f4f3664bc71be621e6"
	tmp := t.TempDir()
	path := filepath.Join(tmp, sessionID+".jsonl")
	header := map[string]any{
		"kind":       "session",
		"version":    1,
		"id":         sessionID,
		"started_at": "2026-09-22T11:33:11Z",
	}
	b, _ := json.Marshal(header)
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatalf("agentserver.Load: %v", err)
	}
	if loaded.Meta.SessionID != sessionID {
		t.Errorf("Meta.SessionID = %q, want %q", loaded.Meta.SessionID, sessionID)
	}
	if loaded.Meta.StartedAt == "" {
		t.Errorf("Meta.StartedAt empty, want from flat header")
	}
}

func TestServerLoadServerHeaderBeatsAgentHeader(t *testing.T) {
	sessionID := "115845c05e9754f4f3664bc71be621e6"
	agentID := "oldrandomhex"
	tmp := t.TempDir()
	path := filepath.Join(tmp, sessionID+".jsonl")
	lines := []map[string]any{
		{
			"kind":       "session",
			"version":    1,
			"id":         agentID,
			"model":      "agent-only-model",
			"started_at": "2026-09-22T11:00:00Z",
		},
		{
			"kind":    "session",
			"version": 1,
			"at":      "2026-09-22T11:33:00Z",
			"session": map[string]any{
				"session_id": sessionID,
				"model":      "server-only-model",
				"max_turns":  200,
				"started_at": "2026-09-22T11:33:00Z",
			},
		},
	}
	var b []byte
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b = append(b, j...)
		b = append(b, '\n')
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatalf("agentserver.Load: %v", err)
	}
	if loaded.Meta.SessionID != sessionID {
		t.Errorf("Meta.SessionID = %q, want %q (server header should win when both present)", loaded.Meta.SessionID, sessionID)
	}
	if loaded.Meta.Model != "server-only-model" {
		t.Errorf("Meta.Model = %q, want server-only-model", loaded.Meta.Model)
	}
}
