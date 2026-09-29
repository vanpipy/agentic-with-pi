package swarm_test

import (
	"encoding/json"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestDeliveryModeParse(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.DeliveryMode
	}{
		{"notify", swarm.DeliveryNotify},
		{"interrupt", swarm.DeliveryInterrupt},
		{"wake", swarm.DeliveryWake},
	}
	for _, c := range cases {
		got := swarm.ParseDeliveryMode(c.wire)
		if got != c.want {
			t.Errorf("ParseDeliveryMode(%q) = %v, want %v", c.wire, got, c.want)
		}
		if got.IsOther() {
			t.Errorf("ParseDeliveryMode(%q).IsOther() = true, want false", c.wire)
		}
	}
	for _, s := range []string{"", "force", "poll"} {
		got := swarm.ParseDeliveryMode(s)
		if !got.IsOther() {
			t.Errorf("ParseDeliveryMode(%q).IsOther() = false, want true", s)
		}
	}
}

func TestDeliveryModeJSON(t *testing.T) {
	for _, want := range []swarm.DeliveryMode{
		swarm.DeliveryNotify, swarm.DeliveryInterrupt, swarm.DeliveryWake,
		swarm.DeliveryOther,
	} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got swarm.DeliveryMode
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}

func TestAwaitModeParse(t *testing.T) {
	cases := []struct {
		wire string
		want swarm.AwaitMode
	}{
		{"all", swarm.AwaitModeAll},
		{"any", swarm.AwaitModeAny},
	}
	for _, c := range cases {
		got := swarm.ParseAwaitMode(c.wire)
		if got != c.want {
			t.Errorf("ParseAwaitMode(%q) = %v, want %v", c.wire, got, c.want)
		}
		if !got.IsAll() && c.want == swarm.AwaitModeAll {
			t.Errorf("ParseAwaitMode(%q).IsAll() = false, want true", c.wire)
		}
		if !got.IsAny() && c.want == swarm.AwaitModeAny {
			t.Errorf("ParseAwaitMode(%q).IsAny() = false, want true", c.wire)
		}
	}
	for _, s := range []string{"", "race", "first"} {
		got := swarm.ParseAwaitMode(s)
		if got == swarm.AwaitModeAll {
			t.Errorf("ParseAwaitMode(%q) returned AwaitModeAll, want AwaitModeOther (caller must default)", s)
		}
		if !got.IsOther() {
			t.Errorf("ParseAwaitMode(%q).IsOther() = false, want true (unknown -> Other)", s)
		}
		if got.IsAny() {
			t.Errorf("ParseAwaitMode(%q).IsAny() = true, want false", s)
		}
	}
}

func TestAwaitModeJSON(t *testing.T) {
	for _, want := range []swarm.AwaitMode{
		swarm.AwaitModeAll, swarm.AwaitModeAny, swarm.AwaitModeOther,
	} {
		b, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var got swarm.AwaitMode
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}

func TestAwaitOptionsDefaults(t *testing.T) {
	// All three flags nil => all default to true.
	o := swarm.AwaitOptions{}
	if !o.EffectiveBackground() {
		t.Error("EffectiveBackground() = false, want true (default)")
	}
	if !o.EffectiveNotify() {
		t.Error("EffectiveNotify() = false, want true (default)")
	}
	if !o.EffectiveWake() {
		t.Error("EffectiveWake() = false, want true (default)")
	}
	// Explicit false is preserved.
	b := false
	o.Background = &b
	if o.EffectiveBackground() {
		t.Error("EffectiveBackground() = true with explicit false, want false")
	}
	// Reassignment retains the pointer.
	o = swarm.AwaitOptions{Background: &b}
	if o.EffectiveBackground() {
		t.Error("EffectiveBackground() = true with explicit false, want false")
	}
	o = swarm.AwaitOptions{Notify: &b}
	if o.EffectiveNotify() {
		t.Error("EffectiveNotify() = true with explicit false, want false")
	}
	o = swarm.AwaitOptions{Wake: &b}
	if o.EffectiveWake() {
		t.Error("EffectiveWake() = true with explicit false, want false")
	}
	// Explicit true is preserved.
	tr := true
	o = swarm.AwaitOptions{Background: &tr}
	if !o.EffectiveBackground() {
		t.Error("EffectiveBackground() = false with explicit true, want true")
	}
}

func TestAwaitOptionsJSON(t *testing.T) {
	in := swarm.AwaitOptions{
		SwarmID:       "sw1",
		FromSessionID: "s1",
		TargetStatus:  []swarm.LifecycleStatus{swarm.StatusCompleted, swarm.StatusFailed},
		SessionIDs:    []string{"s2", "s3"},
		Mode:          swarm.AwaitModeAll,
		TimeoutSecs:   60,
		RequestNonce:  "n1",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.AwaitOptions
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.SwarmID != in.SwarmID {
		t.Errorf("SwarmID = %q, want %q", got.SwarmID, in.SwarmID)
	}
	if len(got.TargetStatus) != 2 {
		t.Errorf("len(TargetStatus) = %d, want 2", len(got.TargetStatus))
	}
	if got.Mode != in.Mode {
		t.Errorf("Mode = %v, want %v", got.Mode, in.Mode)
	}
	if got.TimeoutSecs != 60 {
		t.Errorf("TimeoutSecs = %d, want 60", got.TimeoutSecs)
	}
	// Omitted pointers must remain nil on round-trip.
	if got.Background != nil || got.Notify != nil || got.Wake != nil {
		t.Error("Background/Notify/Wake should be nil when omitted")
	}
}

func TestAwaitedMemberStatusJSON(t *testing.T) {
	in := swarm.AwaitedMemberStatus{
		SessionID:        "s1",
		FriendlyName:     "alice",
		Status:           swarm.StatusCompleted,
		Done:             true,
		CompletionReport: "done",
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.AwaitedMemberStatus
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round-trip: got %+v, want %+v", got, in)
	}
}

func TestAwaitResultJSON(t *testing.T) {
	in := swarm.AwaitResult{
		Completed: true,
		TimedOut:  false,
		Members: []swarm.AwaitedMemberStatus{
			{SessionID: "s1", Status: swarm.StatusCompleted, Done: true},
			{SessionID: "s2", Status: swarm.StatusFailed, Done: true},
		},
		Summary:           "2 of 2 reached target",
		BackgroundStarted: false,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var got swarm.AwaitResult
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Completed {
		t.Error("Completed = false, want true")
	}
	if got.TimedOut {
		t.Error("TimedOut = true, want false")
	}
	if len(got.Members) != 2 {
		t.Errorf("len(Members) = %d, want 2", len(got.Members))
	}
}
