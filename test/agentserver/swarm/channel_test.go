package swarm_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
	"github.com/vanpiyp/awp/internal/agent-server/swarm"
)

func TestSubscribeCreatesChannel(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	channels := s.ListChannels("swarm-A")
	if len(channels) != 1 {
		t.Fatalf("ListChannels len = %d, want 1", len(channels))
	}
	if channels[0].Channel != "general" {
		t.Errorf("Channel = %s, want general", channels[0].Channel)
	}
	if channels[0].MemberCount != 1 {
		t.Errorf("MemberCount = %d, want 1", channels[0].MemberCount)
	}
}

func TestSubscribeMultipleSessions(t *testing.T) {
	t.Parallel()
	s := newState()
	for _, id := range []string{"s1", "s2"} {
		if _, err := s.Register(newRecord(id, "swarm-A")); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}
	for _, id := range []string{"s1", "s2"} {
		if err := s.Subscribe("swarm-A", "general", id); err != nil {
			t.Fatalf("Subscribe %s: %v", id, err)
		}
	}
	members, err := s.ChannelMembers("swarm-A", "general")
	if err != nil {
		t.Fatalf("ChannelMembers: %v", err)
	}
	if !reflect.DeepEqual(members, []string{"s1", "s2"}) {
		t.Errorf("members = %v, want [s1 s2]", members)
	}
}

func TestSubscribeUnknownSessionRejected(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := s.Subscribe("swarm-A", "general", "ghost")
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestSubscribeUnknownSwarmRejected(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := s.Subscribe("ghost-swarm", "general", "s1")
	if !errors.Is(err, swarm.ErrUnknownSwarm) {
		t.Errorf("err = %v, want ErrUnknownSwarm", err)
	}
}

func TestSubscribeEmptyChannelName(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := s.Subscribe("swarm-A", "", "s1")
	if !errors.Is(err, swarm.ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
}

func TestSubscribeNameTooLong(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	longName := strings.Repeat("x", swarmproto.MaxChannelNameBytes+1)
	err := s.Subscribe("swarm-A", longName, "s1")
	if !errors.Is(err, swarm.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestSubscribeIdempotent(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}
	members, err := s.ChannelMembers("swarm-A", "general")
	if err != nil {
		t.Fatalf("ChannelMembers: %v", err)
	}
	if !reflect.DeepEqual(members, []string{"s1"}) {
		t.Errorf("members = %v, want [s1]", members)
	}
}

func TestUnsubscribeRemoves(t *testing.T) {
	t.Parallel()
	s := newState()
	for _, id := range []string{"s1", "s2"} {
		if _, err := s.Register(newRecord(id, "swarm-A")); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
		if err := s.Subscribe("swarm-A", "general", id); err != nil {
			t.Fatalf("Subscribe %s: %v", id, err)
		}
	}
	if err := s.Unsubscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	members, err := s.ChannelMembers("swarm-A", "general")
	if err != nil {
		t.Fatalf("ChannelMembers: %v", err)
	}
	if !reflect.DeepEqual(members, []string{"s2"}) {
		t.Errorf("members = %v, want [s2]", members)
	}
}

func TestUnsubscribeUnknownChannel(t *testing.T) {
	t.Parallel()
	s := newState()
	err := s.Unsubscribe("swarm-A", "missing", "s1")
	if !errors.Is(err, swarm.ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
}

func TestUnsubscribeEmptyChannelName(t *testing.T) {
	t.Parallel()
	s := newState()
	err := s.Unsubscribe("swarm-A", "", "s1")
	if !errors.Is(err, swarm.ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
}

func TestPublishFansOutToSubscribers(t *testing.T) {
	t.Parallel()
	s := newState()
	for _, id := range []string{"s1", "s2"} {
		if _, err := s.Register(newRecord(id, "swarm-A")); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
		if err := s.Subscribe("swarm-A", "general", id); err != nil {
			t.Fatalf("Subscribe %s: %v", id, err)
		}
	}
	b := s.Broadcaster()
	sub1, unsub1 := b.Subscribe("conn-1")
	defer unsub1()
	sub2, unsub2 := b.Subscribe("conn-2")
	defer unsub2()
	if err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{
		FromID: "s1",
		Body:   "hello",
		TLDR:   "hi",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	deadline := time.After(500 * time.Millisecond)
	recv := 0
	for recv < 2 {
		select {
		case ev := <-sub1:
			if ev.Event != swarmproto.EventSwarmChannelMessage {
				t.Errorf("sub1 Event = %s, want %s", ev.Event, swarmproto.EventSwarmChannelMessage)
			}
			recv++
		case ev := <-sub2:
			if ev.Event != swarmproto.EventSwarmChannelMessage {
				t.Errorf("sub2 Event = %s, want %s", ev.Event, swarmproto.EventSwarmChannelMessage)
			}
			recv++
		case <-deadline:
			t.Fatalf("only received %d/2 events", recv)
		}
	}
}

func TestPublishUnknownChannel(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := s.Publish("swarm-A", "missing", swarmproto.ChannelMessage{FromID: "s1", Body: "x"})
	if !errors.Is(err, swarm.ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
}

func TestPublishBodyTooLarge(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	body := strings.Repeat("x", swarmproto.MaxChannelMessageBodyBytes+1)
	err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{FromID: "s1", Body: body})
	if !errors.Is(err, swarm.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestPublishRequiresTLDRForLongBody(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	body := strings.Repeat("x", swarmproto.TLDRRequiredOverChars+1)
	err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{FromID: "s1", Body: body})
	if !errors.Is(err, swarm.ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestPublishAcceptsBodyAtThreshold(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	body := strings.Repeat("x", swarmproto.MaxChannelMessageBodyBytes)
	if err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{FromID: "s1", Body: body, TLDR: "big"}); err != nil {
		t.Errorf("err = %v, want nil (body at threshold should be accepted)", err)
	}
}

func TestPublishAcceptsBodyAtTLDRThreshold(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	body := strings.Repeat("x", swarmproto.TLDRRequiredOverChars)
	if err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{FromID: "s1", Body: body}); err != nil {
		t.Errorf("err = %v, want nil (body at TLDR threshold should not require TLDR)", err)
	}
}

func TestPublishRejectsDuringShutdown(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	s.Close()
	err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{FromID: "s1", Body: "x"})
	if !errors.Is(err, swarm.ErrShuttingDown) {
		t.Errorf("err = %v, want ErrShuttingDown", err)
	}
}

func TestListChannelsSortedByName(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if err := s.Subscribe("swarm-A", name, "s1"); err != nil {
			t.Fatalf("Subscribe %s: %v", name, err)
		}
	}
	channels := s.ListChannels("swarm-A")
	if len(channels) != 3 {
		t.Fatalf("len = %d, want 3", len(channels))
	}
	if channels[0].Channel != "alpha" || channels[1].Channel != "bravo" || channels[2].Channel != "charlie" {
		t.Errorf("order = [%s, %s, %s], want [alpha, bravo, charlie]",
			channels[0].Channel, channels[1].Channel, channels[2].Channel)
	}
}

func TestChannelMembersEmptyChannel(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := s.ChannelMembers("swarm-A", "missing")
	if !errors.Is(err, swarm.ErrUnknownChannel) {
		t.Errorf("err = %v, want ErrUnknownChannel", err)
	}
}

func TestPublishEventShape(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	b := s.Broadcaster()
	sub, unsub := b.Subscribe("conn")
	defer unsub()
	if err := s.Publish("swarm-A", "general", swarmproto.ChannelMessage{
		FromID: "s1",
		Body:   "hello",
		TLDR:   "hi",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case ev := <-sub:
		if ev.Event != swarmproto.EventSwarmChannelMessage {
			t.Errorf("Event = %s, want %s", ev.Event, swarmproto.EventSwarmChannelMessage)
		}
		var payload swarmproto.ChannelMessageEvent
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if payload.Body != "hello" {
			t.Errorf("Body = %s, want hello", payload.Body)
		}
		if payload.TLDR != "hi" {
			t.Errorf("TLDR = %s, want hi", payload.TLDR)
		}
		if payload.Channel != "general" {
			t.Errorf("Channel = %s, want general", payload.Channel)
		}
		if payload.SwarmID != "swarm-A" {
			t.Errorf("SwarmID = %s, want swarm-A", payload.SwarmID)
		}
		if payload.FromID != "s1" {
			t.Errorf("FromID = %s, want s1", payload.FromID)
		}
		if payload.MessageID == "" {
			t.Error("MessageID not assigned")
		}
		if payload.Timestamp == "" {
			t.Error("Timestamp not assigned")
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive publish event")
	}
}
