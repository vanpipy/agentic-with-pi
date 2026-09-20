package server

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
)

type SessionMeta struct {
	SessionID string   `json:"session_id"`
	Model     string   `json:"model"`
	MaxTurns  int      `json:"max_turns"`
	System    string   `json:"system,omitempty"`
	Tools     []string `json:"tools,omitempty"`
	StartedAt string   `json:"started_at"`
}

type Entry struct {
	Kind    string          `json:"kind"`
	ID      string          `json:"id,omitempty"`
	Version int             `json:"version,omitempty"`
	At      string          `json:"at"`
	Session SessionMeta     `json:"session,omitempty"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
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

func Load(path string) (*Loaded, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var loaded Loaded
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}

		switch e.Kind {
		case "session":
			loaded.Meta = e.Session
		case "event":
			loaded.Events = append(loaded.Events, EventRecord{
				Kind: e.Event,
				At:   e.At,
				Data: e.Data,
			})
		case "compaction":
			var c CompactionRecord
			if err := json.Unmarshal(e.Data, &c); err == nil {
				loaded.Compactions = append(loaded.Compactions, c)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &loaded, nil
}

func DefaultPath(sessionsDir, sessionID string) string {
	return fmt.Sprintf("%s/%s.jsonl", sessionsDir, sessionID)
}

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
