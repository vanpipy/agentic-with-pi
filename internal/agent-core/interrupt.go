package agentcore

import (
	"context"
	"errors"
	"sync"

	"github.com/vanpiyp/awp/internal/llm"
)

var ErrInterrupted = errors.New("agent interrupted by SoftInterrupt")

type Signal int

const (
	SignalNone Signal = iota
	SignalUser
	SignalSystem
	SignalBackground
)

func (s Signal) String() string {
	switch s {
	case SignalUser:
		return "user"
	case SignalSystem:
		return "system"
	case SignalBackground:
		return "background"
	default:
		return "none"
	}
}

type SoftInterrupt struct {
	mu   sync.Mutex
	ch   chan Signal
	last Signal
}

func NewSoftInterrupt() *SoftInterrupt {
	return &SoftInterrupt{ch: make(chan Signal, 1)}
}

func (s *SoftInterrupt) Raise(sig Signal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = sig
	select {
	case s.ch <- sig:
	default:
	}
}

func (s *SoftInterrupt) Channel() <-chan Signal { return s.ch }

func (s *SoftInterrupt) Last() Signal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *SoftInterrupt) Fired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last != SignalNone
}

func (s *SoftInterrupt) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.ch:
	default:
	}
	s.last = SignalNone
}

type interruptAwareCore struct {
	inner llm.Core
	intr  *SoftInterrupt
}

func (c *interruptAwareCore) StreamChat(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	innerCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		select {
		case <-c.intr.Channel():
			cancel()
		case <-innerCtx.Done():
		}
	}()
	return c.inner.StreamChat(innerCtx, req)
}
