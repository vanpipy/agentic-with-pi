package server

import "sync"

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]bool
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]bool),
	}
}

func (s *SessionStore) Has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

func (s *SessionStore) Add(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = true
}

func (s *SessionStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}
