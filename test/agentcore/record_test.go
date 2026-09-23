package agentcore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/llm"
)

type fakeCoreForRecord struct{}

func (f *fakeCoreForRecord) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 3)
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		Delta: llm.Message{Content: "ok"},
	}}}}
	ch <- llm.StreamEvent{Chunk: &llm.StreamChunk{Choices: []llm.StreamChoice{{
		FinishReason: llm.FinishReasonStop,
	}}}}
	close(ch)
	return ch, nil
}

func TestNewAgentOpensDefaultSessionLog(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)
	for range ag.RunStream(context.Background(), "hi") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected 1 session log file, got %d", len(files))
	}

	data, _ := os.ReadFile(files[0])
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected >= 2 lines (header + events), got %d", len(lines))
	}

	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header not JSON: %v", err)
	}
	if header["kind"] != "session" {
		t.Errorf("header.kind = %v, want session", header["kind"])
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &event); err != nil {
		t.Fatalf("event not JSON: %v", err)
	}
	if event["kind"] != "event" {
		t.Errorf("event.kind = %v, want event", event["kind"])
	}
}

func TestAwpNoSessionLogSuppressed(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)
	t.Setenv("AWP_NO_SESSION_LOG", "1")

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)
	for range ag.RunStream(context.Background(), "hi") {
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.jsonl"))
	if len(files) != 0 {
		t.Errorf("AWP_NO_SESSION_LOG=1 should suppress log, got %d files", len(files))
	}
}

func TestAwpSessionLogPathOverride(t *testing.T) {
	tmp := t.TempDir()
	customPath := filepath.Join(tmp, "custom.jsonl")
	t.Setenv("AWP_SESSION_LOG_PATH", customPath)
	t.Setenv("AWP_HOME", tmp)

	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core)
	for range ag.RunStream(context.Background(), "hi") {
	}

	if _, err := os.Stat(customPath); err != nil {
		t.Errorf("custom path not created: %v", err)
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.jsonl"))
	if len(files) != 0 {
		t.Errorf("default path should not be created, got %d", len(files))
	}
}

func TestWithLogWriterOverridesDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AWP_HOME", tmp)

	var buf bytes.Buffer
	core := &fakeCoreForRecord{}
	ag := agentcore.NewAgent(core).WithLogWriter(&buf)

	for range ag.RunStream(context.Background(), "hi") {
	}

	if buf.Len() == 0 {
		t.Error("expected WithLogWriter to receive log")
	}

	files, _ := filepath.Glob(filepath.Join(tmp, "logs", "sessions", "*.jsonl"))
	if len(files) != 0 {
		t.Errorf("WithLogWriter should override default file, got %d", len(files))
	}
}

var _ = context.Background
