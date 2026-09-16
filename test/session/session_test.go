package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/session"
)

func TestNewID(t *testing.T) {
	id1 := session.NewID()
	id2 := session.NewID()

	if id1 == id2 {
		t.Error("NewID should generate unique IDs")
	}
	if len(id1) != 32 {
		t.Errorf("NewID length = %d, want 32 (hex 16 bytes)", len(id1))
	}
}

func TestStorePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := session.NewStore(path)

	if s.Path() != path {
		t.Errorf("Path() = %q", s.Path())
	}
}

func TestWriteHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.jsonl")
	s := session.NewStore(path)

	meta := session.SessionMeta{
		SessionID: "abc-123",
		Model:     "test-model",
		MaxTurns:  10,
	}
	if err := s.WriteHeader(meta); err != nil {
		t.Fatal(err)
	}

	loaded, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Meta.SessionID != "abc-123" {
		t.Errorf("SessionID = %q", loaded.Meta.SessionID)
	}
	if loaded.Meta.Model != "test-model" {
		t.Errorf("Model = %q", loaded.Meta.Model)
	}
}

func TestWriteEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := session.NewStore(path)

	s.WriteHeader(session.SessionMeta{SessionID: "s1"})
	if err := s.WriteEvent("thought_chunk", map[string]string{"content": "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteEvent("final_answer", map[string]string{"content": "world"}); err != nil {
		t.Fatal(err)
	}

	loaded, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(loaded.Events))
	}
	if loaded.Events[0].Kind != "thought_chunk" {
		t.Errorf("event 0 kind = %q", loaded.Events[0].Kind)
	}
	if loaded.Events[1].Kind != "final_answer" {
		t.Errorf("event 1 kind = %q", loaded.Events[1].Kind)
	}

	var data struct {
		Content string `json:"content"`
	}
	json.Unmarshal(loaded.Events[0].Data, &data)
	if data.Content != "hello" {
		t.Errorf("event 0 content = %q", data.Content)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	if _, err := session.Load("/nonexistent/path.jsonl"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestRoundTripMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := session.NewStore(path)

	s.WriteHeader(session.SessionMeta{SessionID: "session-x"})

	events := []string{"thought_start", "thought_chunk", "thought_end", "tool", "observe", "final_answer"}
	for _, ev := range events {
		s.WriteEvent(ev, map[string]string{"data": ev})
	}

	loaded, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != len(events) {
		t.Errorf("got %d events, want %d", len(loaded.Events), len(events))
	}

	for i, want := range events {
		if loaded.Events[i].Kind != want {
			t.Errorf("event %d kind = %q, want %q", i, loaded.Events[i].Kind, want)
		}
	}
}

func TestAppendOnlyMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	s := session.NewStore(path)
	s.WriteHeader(session.SessionMeta{SessionID: "first"})
	s.WriteEvent("thought_start", nil)

	s = session.NewStore(path)
	s.WriteEvent("thought_chunk", map[string]string{"content": "second"})

	loaded, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 {
		t.Errorf("got %d events, want 2", len(loaded.Events))
	}
	if loaded.Events[0].Kind != "thought_start" {
		t.Errorf("event 0 = %q", loaded.Events[0].Kind)
	}
	if loaded.Events[1].Kind != "thought_chunk" {
		t.Errorf("event 1 = %q", loaded.Events[1].Kind)
	}
}

func TestDefaultPath(t *testing.T) {
	p := session.DefaultPath("/tmp/sessions", "session-id")
	want := "/tmp/sessions/session-id.jsonl"
	if p != want {
		t.Errorf("got %q, want %q", p, want)
	}
}

func TestDataIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := session.NewStore(path)

	data := struct {
		Name    string `json:"name"`
		Number  int    `json:"number"`
		Flag    bool   `json:"flag"`
	}{Name: "test", Number: 42, Flag: true}
	s.WriteHeader(session.SessionMeta{SessionID: "s"})
	s.WriteEvent("data", data)

	loaded, _ := session.Load(path)

	var got struct {
		Name   string `json:"name"`
		Number int    `json:"number"`
		Flag   bool   `json:"flag"`
	}
	if err := json.Unmarshal(loaded.Events[0].Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "test" || got.Number != 42 || got.Flag != true {
		t.Errorf("data = %+v", got)
	}
}

func TestFileIsOneEntryPerLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := session.NewStore(path)

	s.WriteHeader(session.SessionMeta{SessionID: "s"})
	s.WriteEvent("e1", nil)
	s.WriteEvent("e2", nil)
	s.WriteEvent("e3", nil)

	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines (header + 3 events), got %d", len(lines))
	}
	for i, line := range lines {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("line %d invalid JSON: %v", i, err)
		}
	}
}
