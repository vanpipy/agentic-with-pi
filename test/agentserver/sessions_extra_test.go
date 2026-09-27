package agentserver_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentserver "github.com/vanpiyp/awp/internal/agent-server"
)

func TestWriteCompactionPersistsSummary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := agentserver.NewStore(path)

	if err := s.WriteCompaction(agentserver.CompactionRecord{Summary: "first-half", At: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Compactions) != 1 {
		t.Fatalf("Compactions len = %d", len(loaded.Compactions))
	}
	if loaded.Compactions[0].Summary != "first-half" {
		t.Errorf("Summary = %q", loaded.Compactions[0].Summary)
	}
}

func TestWriteCompactionAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := agentserver.NewStore(path)

	if err := s.WriteCompaction(agentserver.CompactionRecord{Summary: "a", At: "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteCompaction(agentserver.CompactionRecord{Summary: "b", At: "t2"}); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Compactions) != 2 {
		t.Errorf("expected 2 compactions, got %d", len(loaded.Compactions))
	}
}

func TestWriteEventMarshalFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := agentserver.NewStore(path)

	ch := make(chan int)
	if err := s.WriteEvent("evt", ch); err == nil {
		t.Fatal("expected marshal error for chan data")
	}
}

func TestWriteEventNilData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	s := agentserver.NewStore(path)

	if err := s.WriteEvent("plain-event", nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"event":"plain-event"`) {
		t.Errorf("missing event marker: %s", data)
	}
}

func TestLoadEntriesMissingKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"at":"now","event":"old"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected missing-kind error")
	}
	if !strings.Contains(err.Error(), "missing kind") {
		t.Errorf("error = %q, want 'missing kind'", err.Error())
	}
}

func TestLoadEntriesEmptyKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"","at":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected empty-kind error")
	}
	if !strings.Contains(err.Error(), "empty kind") {
		t.Errorf("error = %q, want 'empty kind'", err.Error())
	}
}

func TestLoadEntriesMissingKindButParseable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"at":"now","event":"old"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected missing-kind error")
	}
	if !strings.Contains(err.Error(), "missing kind") {
		t.Errorf("error = %q, want 'missing kind'", err.Error())
	}
}

func TestLoadEntriesUnknownKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"mystery","at":"now"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected unknown-kind error")
	}
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("error = %q, want 'unknown kind'", err.Error())
	}
}

func TestLoadEntriesMessageEnvelopeMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"message","version":2,"entry":"not-an-object"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected message-body error")
	}
}

func TestLoadEntriesCustomEnvelopeMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"custom","version":2,"entry":"oops"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := agentserver.LoadEntries(path); err == nil {
		t.Fatal("expected custom-body error")
	}
}

func TestLoadEntriesCustomMessageEnvelopeMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"custom_message","version":2,"entry":"oops"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := agentserver.LoadEntries(path); err == nil {
		t.Fatal("expected custom_message-body error")
	}
}

func TestLoadEntriesCompactionAltFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"compaction","version":2,"summary":"alt","at":"t0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Kind != "compaction" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestLoadEntriesCompactionDataMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"compaction","version":2,"data":"not-a-record"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := agentserver.LoadEntries(path); err == nil {
		t.Fatal("expected compaction-data error")
	}
}

func TestLoadEntriesCompactionEnvelopeMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"compaction","version":2}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestLoadEntriesSessionEnvelopeMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"session","version":1,"session":"oops"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := agentserver.LoadEntries(path); err == nil {
		t.Fatal("expected session-body error")
	}
}

func TestLoadEntriesEventMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"event","at":"t","oops":}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := agentserver.LoadEntries(path); err == nil {
		t.Fatal("expected event-body error")
	}
}

func TestLoadEntriesKindUnmarshalFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected kind-unmarshal error")
	}
}

func TestLoadEntriesSessionFallbackToLegacyFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"session","id":"legacy-1","model":"m1","max_turns":7,"started_at":"t0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d", len(entries))
	}
	meta, ok := entries[0].Parsed.(agentserver.SessionMeta)
	if !ok {
		t.Fatalf("parsed = %T", entries[0].Parsed)
	}
	if meta.SessionID != "legacy-1" {
		t.Errorf("SessionID = %q", meta.SessionID)
	}
	if meta.MaxTurns != 7 {
		t.Errorf("MaxTurns = %d", meta.MaxTurns)
	}
}

func TestLoadFiltersNonLegacyEventEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"event","at":"t","category":"chat"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 {
		t.Fatalf("Events len = %d", len(loaded.Events))
	}
	if loaded.Events[0].Kind != "chat" {
		t.Errorf("Kind = %q", loaded.Events[0].Kind)
	}
}

func TestLoadIgnoresSessionWithoutID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"session","version":1,"session":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Meta.SessionID != "" {
		t.Errorf("Meta.SessionID = %q", loaded.Meta.SessionID)
	}
}

func TestLoadEventFallbackToEventField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	if err := os.WriteFile(path, []byte(`{"kind":"event","at":"t","event":"fallback"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].Kind != "fallback" {
		t.Fatalf("events = %+v", loaded.Events)
	}
}

func TestLoadEntriesReadsAllKinds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	lines := []string{
		`{"kind":"session","version":1,"session":{"session_id":"s1"}}`,
		`{"kind":"event","at":"t","category":"e1"}`,
		`{"kind":"message","version":2,"entry":{"id":"m1"}}`,
		`{"kind":"custom","version":2,"entry":{"event":"c1"}}`,
		`{"kind":"custom_message","version":2,"entry":{}}`,
		`{"kind":"compaction","version":2,"summary":"x","at":"t"}`,
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(lines) {
		t.Errorf("entries len = %d, want %d", len(entries), len(lines))
	}
}

func TestExtractLegacyDataField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	payload := json.RawMessage(`{"a":1}`)
	body := `{"kind":"event","at":"t","category":"chat","data":` + string(payload) + `}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 1 {
		t.Fatalf("Events len = %d", len(loaded.Events))
	}
	if string(loaded.Events[0].Data) != string(payload) {
		t.Errorf("Data = %s, want %s", loaded.Events[0].Data, payload)
	}
}

func TestExtractLegacyDataFieldFallbackToNull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	body := `{"kind":"event","at":"t","category":"chat"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := agentserver.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Events[0].Data) != "null" {
		t.Errorf("Data = %s, want null", loaded.Events[0].Data)
	}
}

func TestWriteEventMarshalSuccessForCustomType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.jsonl")
	s := agentserver.NewStore(path)

	type custom struct {
		Name string `json:"name"`
	}
	if err := s.WriteEvent("evt", custom{Name: "test"}); err != nil {
		t.Fatal(err)
	}
}

func TestSocketPathNoPathMethod(t *testing.T) {
	server := &agentserver.Server{}
	if got := server.SocketPath(); got != "" {
		t.Errorf("SocketPath = %q", got)
	}
}
