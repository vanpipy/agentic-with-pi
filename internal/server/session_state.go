package server

import (
	"sync"

	"github.com/vanpiyp/awp/internal/llm"
)

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
