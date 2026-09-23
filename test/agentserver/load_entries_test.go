package agentserver_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
)

func TestLoadEntriesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")

	const legacyJSONL = `{"kind":"session","version":1,"id":"f7173577969c88148cb5bf3c16767d87","model":"MiniMax-M3","max_turns":200,"system":"you are helpful","tools":["read","write"],"started_at":"2026-09-22T16:26:47.803329214Z"}
{"kind":"event","seq":1,"at":"2026-09-22T16:26:47.803436334Z","category":"thought_start"}
{"kind":"event","seq":2,"at":"2026-09-22T16:26:48.675664287Z","category":"thought_chunk","reasoning":"The user said"}
{"kind":"event","seq":3,"at":"2026-09-22T16:26:48.675678694Z","category":"thought_chunk","content":"hi"}
{"kind":"event","seq":4,"at":"2026-09-22T16:26:48.675679977Z","category":"thought_end","reasoning":"final","content":"hi","tool_calls_count":1}
{"kind":"event","seq":5,"at":"2026-09-22T16:26:48.71773254Z","category":"tool","tool_name":"bash","tool_args":"{\"command\":\"date\"}"}
{"kind":"event","seq":6,"at":"2026-09-22T16:26:48.923685841Z","category":"observe","tool_name":"bash","tool_result":"ok","tool_error":""}
{"kind":"event","seq":7,"at":"2026-09-22T16:26:49.114038355Z","category":"final_answer","content":"done"}
{"kind":"event","seq":8,"at":"2026-09-22T16:26:58.013837338Z","category":"error","tool_error":"failed"}
`
	if err := os.WriteFile(path, []byte(legacyJSONL), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 9 {
		t.Fatalf("got %d entries, want 9", len(entries))
	}

	cases := []struct {
		idx      int
		kind     string
		version  int
		category string
	}{
		{0, "session", 1, ""},
		{1, "event", 1, "thought_start"},
		{2, "event", 1, "thought_chunk"},
		{3, "event", 1, "thought_chunk"},
		{4, "event", 1, "thought_end"},
		{5, "event", 1, "tool"},
		{6, "event", 1, "observe"},
		{7, "event", 1, "final_answer"},
		{8, "event", 1, "error"},
	}

	for _, c := range cases {
		entry := entries[c.idx]
		if entry.Kind != c.kind {
			t.Errorf("entry[%d].Kind = %q, want %q", c.idx, entry.Kind, c.kind)
		}
		if entry.SchemaVersion != c.version {
			t.Errorf("entry[%d].SchemaVersion = %d, want %d", c.idx, entry.SchemaVersion, c.version)
		}
		if c.kind == "event" {
			legacy, ok := entry.Parsed.(agentserver.LegacyEvent)
			if !ok {
				t.Errorf("entry[%d].Parsed type = %T, want LegacyEvent", c.idx, entry.Parsed)
				continue
			}
			if legacy.Category != c.category {
				t.Errorf("entry[%d].Parsed.Category = %q, want %q", c.idx, legacy.Category, c.category)
			}
		}
	}

	if entries[0].Kind == "session" {
		meta, ok := entries[0].Parsed.(agentserver.SessionMeta)
		if !ok {
			t.Errorf("session entry parsed type = %T, want SessionMeta", entries[0].Parsed)
		} else if meta.SessionID != "f7173577969c88148cb5bf3c16767d87" {
			t.Errorf("session ID = %q", meta.SessionID)
		}
	}

	legacy5 := entries[5].Parsed.(agentserver.LegacyEvent)
	if legacy5.ToolName != "bash" {
		t.Errorf("tool event name = %q, want bash", legacy5.ToolName)
	}
	if legacy5.ToolArgs != `{"command":"date"}` {
		t.Errorf("tool args = %q", legacy5.ToolArgs)
	}
}

func TestLoadEntriesNewSchemaMessageAndCustom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.jsonl")

	msgEvent := json_rpc.MessageEvent{
		ID:        "0190a3b7-0002-7c8a-9000-000000000002",
		ParentID:  "0190a3b7-0001-7c8a-9000-000000000001",
		Timestamp: "2026-09-23T13:00:01.234Z",
		Message: json_rpc.Message{
			Role: "assistant",
			Content: []json_rpc.MessageContentPart{
				{Type: "thinking", Thinking: "the user wants x"},
				{Type: "text", Text: "I'll search"},
				{Type: "toolCall", ID: "call_x", Name: "agentgrep", Intent: "find x", Arguments: json.RawMessage(`{"pattern":"x"}`)},
			},
		},
		StopReason: "toolUse",
		Usage: &json_rpc.UsageStats{
			PromptTokens:     1024,
			CompletionTokens: 171,
			TotalTokens:      1195,
		},
	}
	msgLine, err := json.Marshal(msgEvent)
	if err != nil {
		t.Fatal(err)
	}

	customEvent := json_rpc.CustomEvent{
		ID:         "0190a3b7-0006-7c8a-9000-000000000006",
		ParentID:   "0190a3b7-0005-7c8a-9000-000000000005",
		Timestamp:  "2026-09-23T13:00:30.000Z",
		CustomType: "abort",
		Data:       json.RawMessage(`{"reason":"tool failed 3 times"}`),
	}
	customLine, err := json.Marshal(customEvent)
	if err != nil {
		t.Fatal(err)
	}

	wrappedMsg := struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "message", Version: 2, Entry: msgLine}
	wrappedCustom := struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "custom", Version: 2, Entry: customLine}

	out, err := json.Marshal(wrappedMsg)
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')
	customBytes, _ := json.Marshal(wrappedCustom)
	out = append(out, customBytes...)
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	if entries[0].Kind != "message" {
		t.Errorf("entry[0].Kind = %q, want message", entries[0].Kind)
	}
	if entries[0].SchemaVersion != 2 {
		t.Errorf("entry[0].SchemaVersion = %d, want 2", entries[0].SchemaVersion)
	}

	msg, ok := entries[0].Parsed.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("entry[0].Parsed type = %T, want json_rpc.MessageEvent", entries[0].Parsed)
	}
	if msg.Message.Role != "assistant" {
		t.Errorf("role = %q", msg.Message.Role)
	}
	if len(msg.Message.Content) != 3 {
		t.Errorf("content parts = %d, want 3", len(msg.Message.Content))
	}
	if msg.Message.Content[1].Text != "I'll search" {
		t.Errorf("text content = %q", msg.Message.Content[1].Text)
	}
	if msg.StopReason != "toolUse" {
		t.Errorf("stopReason = %q", msg.StopReason)
	}

	if entries[1].Kind != "custom" {
		t.Errorf("entry[1].Kind = %q, want custom", entries[1].Kind)
	}
	cust, ok := entries[1].Parsed.(json_rpc.CustomEvent)
	if !ok {
		t.Fatalf("entry[1].Parsed type = %T, want json_rpc.CustomEvent", entries[1].Parsed)
	}
	if cust.CustomType != "abort" {
		t.Errorf("customType = %q", cust.CustomType)
	}
}

func TestLoadEntriesMixedSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.jsonl")

	const mixed = `{"kind":"session","version":2,"id":"sess-mixed","model":"MiniMax-M3","max_turns":200,"system":"x","tools":["read"],"started_at":"2026-09-23T00:00:00Z"}
{"kind":"event","seq":1,"at":"2026-09-23T00:00:01Z","category":"thought_start"}
{"kind":"message","version":2,"entry":{"id":"m1","parentId":null,"timestamp":"2026-09-23T00:00:02Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}}
{"kind":"event","seq":2,"at":"2026-09-23T00:00:03Z","category":"final_answer","content":"hello there"}
`
	if err := os.WriteFile(path, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}

	wantKinds := []string{"session", "event", "message", "event"}
	wantVersions := []int{2, 1, 2, 1}
	for i := range entries {
		if entries[i].Kind != wantKinds[i] {
			t.Errorf("entries[%d].Kind = %q, want %q", i, entries[i].Kind, wantKinds[i])
		}
		if entries[i].SchemaVersion != wantVersions[i] {
			t.Errorf("entries[%d].SchemaVersion = %d, want %d", i, entries[i].SchemaVersion, wantVersions[i])
		}
	}

	if msg, ok := entries[2].Parsed.(json_rpc.MessageEvent); ok {
		if msg.Message.Role != "user" {
			t.Errorf("user role = %q", msg.Message.Role)
		}
	} else {
		t.Errorf("entries[2].Parsed type = %T, want MessageEvent", entries[2].Parsed)
	}

	if legacy, ok := entries[3].Parsed.(agentserver.LegacyEvent); ok {
		if legacy.Category != "final_answer" {
			t.Errorf("legacy category = %q", legacy.Category)
		}
		if legacy.Content != "hello there" {
			t.Errorf("legacy content = %q", legacy.Content)
		}
	} else {
		t.Errorf("entries[3].Parsed type = %T, want LegacyEvent", entries[3].Parsed)
	}
}

func TestLoadEntriesBadLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")

	const corrupted = `{"kind":"session","version":2,"id":"abc","model":"m","max_turns":10,"system":"","tools":[],"started_at":"2026-09-23T00:00:00Z"}
not-a-json-line
{"kind":"message","version":2,"entry":{"id":"m1"}}
`
	if err := os.WriteFile(path, []byte(corrupted), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agentserver.LoadEntries(path)
	if err == nil {
		t.Fatal("expected error for malformed line, got nil")
	}
	if !strings.Contains(err.Error(), "line") {
		t.Errorf("expected error to mention line number, got %q", err.Error())
	}
}

func TestLoadEntriesCustomMessageEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cm.jsonl")

	customMsg := json_rpc.CustomMessageEvent{
		ID:         "0190a3b7-0007-7c8a-9000-000000000007",
		ParentID:   "0190a3b7-0006-7c8a-9000-000000000006",
		Timestamp:  "2026-09-23T13:00:35.000Z",
		CustomType: "subagent_notification",
		Content:    "subagent T-42 finished",
		Display:    true,
		Details:    json.RawMessage(`{}`),
	}
	raw, _ := json.Marshal(customMsg)
	wrapped := struct {
		Kind    string          `json:"kind"`
		Version int             `json:"version"`
		Entry   json.RawMessage `json:"entry"`
	}{Kind: "custom_message", Version: 2, Entry: raw}
	out, _ := json.Marshal(wrapped)
	out = append(out, '\n')

	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Kind != "custom_message" {
		t.Errorf("Kind = %q", entries[0].Kind)
	}
	cm, ok := entries[0].Parsed.(json_rpc.CustomMessageEvent)
	if !ok {
		t.Fatalf("Parsed type = %T, want CustomMessageEvent", entries[0].Parsed)
	}
	if cm.Content != "subagent T-42 finished" {
		t.Errorf("content = %q", cm.Content)
	}
}

func TestLoadEntriesCompactionEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compact.jsonl")

	const compact = `{"kind":"session","version":2,"id":"abc","model":"m","max_turns":10,"system":"","tools":[],"started_at":"2026-09-23T00:00:00Z"}
{"kind":"compaction","at":"2026-09-23T12:00:00Z","summary":"learned about jcode","data":{"summary":"learned about jcode","at":"2026-09-23T12:00:00Z"}}
`
	if err := os.WriteFile(path, []byte(compact), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[1].Kind != "compaction" {
		t.Errorf("entries[1].Kind = %q", entries[1].Kind)
	}
	comp, ok := entries[1].Parsed.(agentserver.CompactionRecord)
	if !ok {
		t.Fatalf("entries[1].Parsed type = %T, want CompactionRecord", entries[1].Parsed)
	}
	if comp.Summary != "learned about jcode" {
		t.Errorf("summary = %q", comp.Summary)
	}
}

func TestLoadEntriesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := agentserver.LoadEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

// guard against unused imports if other tests are removed during refactor.
var _ = agentcore.Event{}
var _ = llm.Model{}
