package agentserver

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/llm"
)

type SessionMeta struct {
	SessionID string   `json:"session_id"`
	Model     string   `json:"model"`
	MaxTurns  int      `json:"max_turns"` // legacy field name; persisted as "max_turns" for backward compat
	System    string   `json:"system,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	StartedAt string   `json:"started_at"`
}

type Entry struct {
	Kind      string          `json:"kind"`
	ID        string          `json:"id,omitempty"`
	Version   int             `json:"version,omitempty"`
	At        string          `json:"at"`
	Session   SessionMeta     `json:"session,omitempty"`
	Event     string          `json:"event,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Model     string          `json:"model,omitempty"`
	MaxTurns  int             `json:"max_turns,omitempty"`
	System    string          `json:"system,omitempty"`
	Tools     []string        `json:"tools,omitempty"`
	StartedAt string          `json:"started_at,omitempty"`
}

type Store struct {
	mu   sync.Mutex
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) WriteHeader(meta SessionMeta) error {
	e := Entry{
		Kind:    "session",
		Version: 1,
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Session: meta,
	}
	return s.append(e)
}

type CompactionRecord struct {
	Summary string `json:"summary"`
	At      string `json:"at"`
}

func (s *Store) WriteCompaction(c CompactionRecord) error {
	e := Entry{
		Kind: "compaction",
		At:   time.Now().UTC().Format(time.RFC3339Nano),
		Data: mustMarshal(CompactionRecord{Summary: c.Summary, At: c.At}),
	}
	return s.append(e)
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

func (s *Store) WriteEvent(eventKind string, data any) error {
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		raw = b
	}
	e := Entry{
		Kind:  "event",
		At:    time.Now().UTC().Format(time.RFC3339Nano),
		Event: eventKind,
		Data:  raw,
	}
	return s.append(e)
}

func (s *Store) append(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = f.Write(b)
	return err
}

type Loaded struct {
	Meta        SessionMeta
	Events      []EventRecord
	Compactions []CompactionRecord
}

type EventRecord struct {
	Kind string
	At   string
	Data json.RawMessage
}

type LegacyEvent struct {
	Kind           string `json:"kind,omitempty"`
	Event          string `json:"event,omitempty"`
	Category       string `json:"category,omitempty"`
	At             string `json:"at"`
	Seq            int    `json:"seq,omitempty"`
	Reasoning      string `json:"reasoning,omitempty"`
	Content        string `json:"content,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	ToolArgs       string `json:"tool_args,omitempty"`
	ToolResult     string `json:"tool_result,omitempty"`
	ToolError      string `json:"tool_error,omitempty"`
	UserMessage    string `json:"user_message,omitempty"`
	ToolCallsCount int    `json:"tool_calls_count,omitempty"`
}

type LoadedEntry struct {
	SchemaVersion int
	Raw           json.RawMessage
	Parsed        any
	Kind          string
}

func Load(path string) (*Loaded, error) {
	entries, err := LoadEntries(path)
	if err != nil {
		return nil, err
	}
	var loaded Loaded
	for _, e := range entries {
		switch e.Kind {
		case "session":
			if meta, ok := e.Parsed.(SessionMeta); ok && meta.SessionID != "" {
				loaded.Meta = meta
			}
		case "event":
			legacy, ok := e.Parsed.(LegacyEvent)
			if !ok {
				continue
			}
			category := legacy.Category
			if category == "" {
				category = legacy.Event
			}
			data := extractLegacyDataField(e.Raw)
			if data == nil {
				data = json.RawMessage("null")
			}
			loaded.Events = append(loaded.Events, EventRecord{
				Kind: category,
				At:   legacy.At,
				Data: data,
			})
		case "compaction":
			if c, ok := e.Parsed.(CompactionRecord); ok {
				loaded.Compactions = append(loaded.Compactions, c)
			}
		}
	}
	return &loaded, nil
}

func LoadEntries(path string) ([]LoadedEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var entries []LoadedEntry
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		raw := json.RawMessage(append([]byte(nil), line...))

		var kindProbe struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(line, &kindProbe); err != nil {
			return nil, fmt.Errorf("line %d: unmarshal kind: %w", lineNum, err)
		}

		switch kindProbe.Kind {
		case "session":
			var probe struct {
				Version int `json:"version"`
			}
			_ = json.Unmarshal(line, &probe)
			version := probe.Version
			if version == 0 {
				version = 1
			}
			var entry Entry
			if err := json.Unmarshal(line, &entry); err != nil {
				return nil, fmt.Errorf("line %d: session: %w", lineNum, err)
			}
			meta := entry.Session
			if meta.SessionID == "" {
				meta = SessionMeta{
					SessionID: entry.ID,
					Model:     entry.Model,
					MaxTurns:  entry.MaxTurns,
					System:    entry.System,
					Tools:     entry.Tools,
					StartedAt: entry.StartedAt,
				}
			}
			entries = append(entries, LoadedEntry{
				SchemaVersion: version,
				Raw:           raw,
				Parsed:        meta,
				Kind:          "session",
			})

		case "event":
			var legacy LegacyEvent
			if err := json.Unmarshal(line, &legacy); err != nil {
				return nil, fmt.Errorf("line %d: event: %w", lineNum, err)
			}
			entries = append(entries, LoadedEntry{
				SchemaVersion: 1,
				Raw:           raw,
				Parsed:        legacy,
				Kind:          "event",
			})

		case "message":
			var wrap struct {
				Version int             `json:"version"`
				Entry   json.RawMessage `json:"entry"`
			}
			if err := json.Unmarshal(line, &wrap); err != nil {
				return nil, fmt.Errorf("line %d: message envelope: %w", lineNum, err)
			}
			var msg json_rpc.MessageEvent
			if err := json.Unmarshal(wrap.Entry, &msg); err != nil {
				return nil, fmt.Errorf("line %d: message body: %w", lineNum, err)
			}
			entries = append(entries, LoadedEntry{
				SchemaVersion: 2,
				Raw:           raw,
				Parsed:        msg,
				Kind:          "message",
			})

		case "custom":
			var wrap struct {
				Version int             `json:"version"`
				Entry   json.RawMessage `json:"entry"`
			}
			if err := json.Unmarshal(line, &wrap); err != nil {
				return nil, fmt.Errorf("line %d: custom envelope: %w", lineNum, err)
			}
			var evt json_rpc.CustomEvent
			if err := json.Unmarshal(wrap.Entry, &evt); err != nil {
				return nil, fmt.Errorf("line %d: custom body: %w", lineNum, err)
			}
			entries = append(entries, LoadedEntry{
				SchemaVersion: 2,
				Raw:           raw,
				Parsed:        evt,
				Kind:          "custom",
			})

		case "custom_message":
			var wrap struct {
				Version int             `json:"version"`
				Entry   json.RawMessage `json:"entry"`
			}
			if err := json.Unmarshal(line, &wrap); err != nil {
				return nil, fmt.Errorf("line %d: custom_message envelope: %w", lineNum, err)
			}
			var cm json_rpc.CustomMessageEvent
			if err := json.Unmarshal(wrap.Entry, &cm); err != nil {
				return nil, fmt.Errorf("line %d: custom_message body: %w", lineNum, err)
			}
			entries = append(entries, LoadedEntry{
				SchemaVersion: 2,
				Raw:           raw,
				Parsed:        cm,
				Kind:          "custom_message",
			})

		case "compaction":
			var wrap struct {
				Version int             `json:"version"`
				Data    json.RawMessage `json:"data"`
				Summary string          `json:"summary"`
				At      string          `json:"at"`
			}
			if err := json.Unmarshal(line, &wrap); err != nil {
				return nil, fmt.Errorf("line %d: compaction envelope: %w", lineNum, err)
			}
			var rec CompactionRecord
			if len(wrap.Data) > 0 {
				if err := json.Unmarshal(wrap.Data, &rec); err != nil {
					return nil, fmt.Errorf("line %d: compaction data: %w", lineNum, err)
				}
			} else {
				rec = CompactionRecord{Summary: wrap.Summary, At: wrap.At}
			}
			entries = append(entries, LoadedEntry{
				SchemaVersion: 2,
				Raw:           raw,
				Parsed:        rec,
				Kind:          "compaction",
			})

		case "":
			if err := json.Unmarshal(line, &LegacyEvent{}); err == nil {
				return nil, fmt.Errorf("line %d: missing kind field", lineNum)
			}
			return nil, fmt.Errorf("line %d: empty kind field", lineNum)

		default:
			return nil, fmt.Errorf("line %d: unknown kind %q", lineNum, kindProbe.Kind)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func DefaultPath(sessionsDir, sessionID string) string {
	return fmt.Sprintf("%s/%s.jsonl", sessionsDir, sessionID)
}

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func extractLegacyDataField(raw json.RawMessage) json.RawMessage {
	var d struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &d); err == nil && len(d.Data) > 0 {
		return d.Data
	}
	return nil
}

type sessionStateStore struct {
	mu sync.Mutex
	m  map[string][]llm.Message
}

func newSessionStateStore() *sessionStateStore {
	return &sessionStateStore{m: make(map[string][]llm.Message)}
}

func (s *sessionStateStore) snapshot(sessionID string) ([]llm.Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs, ok := s.m[sessionID]
	if !ok {
		return nil, false
	}
	cp := append([]llm.Message{}, msgs...)
	return cp, true
}

func (s *sessionStateStore) seed(sessionID string, msgs []llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sessionID] = append([]llm.Message{}, msgs...)
}

func (s *sessionStateStore) update(sessionID string, msgs []llm.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[sessionID] = append([]llm.Message{}, msgs...)
}

func (s *sessionStateStore) del(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, sessionID)
}
