package swarm_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestChannelInfoJSON(t *testing.T) {
	in := swarm.ChannelInfo{
		Channel:     "main",
		SwarmID:     "sw1",
		MemberCount: 3,
		CreatedAt:   "2026-01-15T10:00:00Z",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ChannelInfo
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Channel != in.Channel {
		t.Errorf("Channel = %q, want %q", got.Channel, in.Channel)
	}
	if got.MemberCount != in.MemberCount {
		t.Errorf("MemberCount = %d, want %d", got.MemberCount, in.MemberCount)
	}
}

func TestChannelMessageJSON(t *testing.T) {
	in := swarm.ChannelMessage{
		MessageID: "m1",
		SwarmID:   "sw1",
		Channel:   "main",
		FromID:    "s1",
		FromName:  "alice",
		Body:      "hello",
		TLDR:      "greet",
		Delivery:  swarm.DeliveryNotify,
		Timestamp: "2026-01-15T10:00:00Z",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.ChannelMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

func TestChannelMessageEventJSON(t *testing.T) {
	in := swarm.ChannelMessageEvent{
		MessageID: "m1",
		SwarmID:   "sw1",
		Channel:   "main",
		FromID:    "s1",
		FromName:  "alice",
		Body:      "hello",
		Delivery:  swarm.DeliveryNotify,
		Timestamp: "2026-01-15T10:00:00Z",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"channel":"main"`) {
		t.Errorf("channel missing: %s", b)
	}
	var got swarm.ChannelMessageEvent
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Channel != "main" {
		t.Errorf("Channel = %q, want main", got.Channel)
	}
	if got.Body != "hello" {
		t.Errorf("Body = %q, want hello", got.Body)
	}
}
