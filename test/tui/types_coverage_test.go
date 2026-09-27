package tui_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestStateStringCoversAllKnownStates(t *testing.T) {
	cases := []struct {
		s    tui.State
		want string
	}{
		{tui.StateReady, "ready"},
		{tui.StateStreaming, "streaming"},
		{tui.StateCancelling, "cancelling"},
		{tui.StateError, "error"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("State(%d).String() = %q, want %q", int(c.s), got, c.want)
		}
	}
}

func TestStateStringUnknownValueFallsThrough(t *testing.T) {
	var s tui.State = 99
	if got := s.String(); got != "unknown" {
		t.Errorf("State(99).String() = %q, want %q", got, "unknown")
	}
	var zero tui.State
	if got := zero.String(); got != "ready" {
		t.Errorf("State(0).String() = %q, want %q", got, "ready")
	}
}

func TestStateStreamingForTestValueReturnsStreaming(t *testing.T) {
	if got := tui.StateStreamingForTestValue(); got != tui.StateStreaming {
		t.Errorf("StateStreamingForTestValue() = %v, want %v", got, tui.StateStreaming)
	}
}
