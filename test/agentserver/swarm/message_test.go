package swarm_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
	"github.com/vanpiyp/awp/internal/agent-server/swarm"
)

// childSink retrieves the per-member Sink of sessionID. Direct delivery
// writes to m.Sink (a chan jsonrpc.Response of capacity 32). Test code
// reaches it via the public GetMember helper.
func childSink(sessionID string, s *swarm.SwarmState) <-chan jsonrpc.Response {
	m, ok := s.GetMember(sessionID)
	if !ok {
		ch := make(chan jsonrpc.Response)
		close(ch)
		return ch
	}
	return m.Sink
}

func TestSendMessageDirectDelivery(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"}); err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	id, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "child",
		Body:          "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if id == "" {
		t.Error("message id empty")
	}
	select {
	case <-childSink("child", s):
	case <-time.After(time.Second):
		t.Fatal("recipient did not receive message")
	}
}

func TestSendMessageRequiresRouteField(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	_, err := s.SendMessage(swarm.MessageOptions{FromSessionID: "self", SwarmID: "self"})
	if !errors.Is(err, swarm.ErrInvalidRequest) {
		t.Errorf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestSendMessageRejectsBothRoutes(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"}); err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	_, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "child",
		Channel:       "general",
	})
	if !errors.Is(err, swarm.ErrInvalidRequest) {
		t.Errorf("err = %v, want ErrInvalidRequest", err)
	}
}

func TestSendMessageRequiresFrom(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.SendMessage(swarm.MessageOptions{ToSessionID: "x"})
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestSendMessageUnknownRecipient(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	_, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "ghost",
		Body:          "x",
	})
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestSendMessageBodyTooLarge(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	body := strings.Repeat("x", swarmproto.MaxChannelMessageBodyBytes+1)
	_, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "self",
		Body:          body,
	})
	if !errors.Is(err, swarm.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestSendMessageAcceptsBodyAtThreshold(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "peer"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	body := strings.Repeat("x", swarmproto.MaxChannelMessageBodyBytes)
	if _, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "peer",
		Body:          body,
		TLDR:          "big",
	}); err != nil {
		t.Errorf("err = %v, want nil (body at threshold should be accepted)", err)
	}
}

func TestSendMessageAcceptsBodyAtTLDRThreshold(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "peer"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	body := strings.Repeat("x", swarmproto.TLDRRequiredOverChars)
	if _, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "peer",
		Body:          body,
	}); err != nil {
		t.Errorf("err = %v, want nil (body at TLDR threshold should not require TLDR)", err)
	}
}

func TestSendMessageRequiresTLDRForLongBody(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	body := strings.Repeat("x", swarmproto.TLDRRequiredOverChars+1)
	_, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "self",
		Body:          body,
	})
	if !errors.Is(err, swarm.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestSendMessageChannelRoute(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"}); err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	if err := s.Subscribe("self", "general", "self"); err != nil {
		t.Fatalf("Subscribe self: %v", err)
	}
	if err := s.Subscribe("self", "general", "child"); err != nil {
		t.Fatalf("Subscribe child: %v", err)
	}
	b := s.Broadcaster()
	sub, unsub := b.Subscribe("conn")
	defer unsub()
	id, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		Channel:       "general",
		Body:          "hi all",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if id == "" {
		t.Error("message id empty")
	}
	select {
	case ev := <-sub:
		if ev.Event != swarmproto.EventSwarmChannelMessage {
			t.Errorf("Event = %s, want %s", ev.Event, swarmproto.EventSwarmChannelMessage)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive channel broadcast")
	}
}

func TestSendMessageChannelUnknown(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	_, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		Channel:       "missing",
		Body:          "x",
	})
	if !errors.Is(err, swarm.ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
}

func TestSendMessageRejectsDuringShutdown(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	s.Close()
	_, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "self",
		Body:          "x",
	})
	if !errors.Is(err, swarm.ErrShuttingDown) {
		t.Errorf("err = %v, want ErrShuttingDown", err)
	}
}

func TestSendMessageDirectEventShape(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"}); err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	sink := childSink("child", s)
	if _, err := s.SendMessage(swarm.MessageOptions{
		FromSessionID: "self",
		SwarmID:       "self",
		ToSessionID:   "child",
		Body:          "ping",
	}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	select {
	case ev := <-sink:
		if ev.Event != swarmproto.EventCommMessageResponse {
			t.Errorf("Event = %s, want %s", ev.Event, swarmproto.EventCommMessageResponse)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive direct message")
	}
}
