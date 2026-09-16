package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type Loaded struct {
	Meta   SessionMeta
	Events []EventRecord
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
