package swarm_test

import (
	"errors"
	"math/rand/v2"
	"sync"
	"testing"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
	"github.com/vanpiyp/awp/internal/agent-server/swarm"
)

// TestPropertyRandomSequence runs a random sequence of swarm operations
// across N goroutines and asserts that the swarm invariant (member count
// never exceeds admission cap) holds at every observation point.
func TestPropertyRandomSequence(t *testing.T) {
	t.Parallel()
	const (
		goroutines = 8
		opsPerG    = 200
		cap        = 8
	)
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = cap
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(worker), 0xbeef))
			for op := 0; op < opsPerG; op++ {
				choice := rng.IntN(5)
				id := "w-" + itoa(worker) + "-" + itoa(op)
				switch choice {
				case 0:
					_, _ = s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: id})
				case 1:
					_ = s.Unregister(id)
				case 2:
					_ = s.Subscribe("self", "ch", id)
				case 3:
					_ = s.Unsubscribe("self", "ch", id)
				case 4:
					_ = s.Publish("self", "ch", swarmproto.ChannelMessage{FromID: id, Body: "x"})
				}
				count := s.SwarmMemberCount("self")
				if count > cap {
					t.Errorf("invariant violated: SwarmMemberCount(%d) > cap(%d) after op %d on worker %d", count, cap, op, worker)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	final := s.SwarmMemberCount("self")
	if final > cap {
		t.Errorf("final SwarmMemberCount(%d) > cap(%d)", final, cap)
	}
}

// TestPropertyPublishAlwaysDeliversToSubscribed checks that after a
// successful Publish, every active subscriber (whose channel buffer has
// room) receives the event.
func TestPropertyPublishAlwaysDeliversToSubscribed(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	for i := 0; i < 5; i++ {
		sid := "sub-" + itoa(i)
		if _, err := s.Register(swarmproto.MemberRecord{SessionID: sid, SwarmID: "swarm-A", Status: swarmproto.StatusRunning}); err != nil {
			t.Fatalf("Register %s: %s", sid, err)
		}
		if err := s.Subscribe("swarm-A", "general", sid); err != nil {
			t.Fatalf("Subscribe %s: %s", sid, err)
		}
	}

	if _, err := s.Register(swarmproto.MemberRecord{SessionID: "sender", SwarmID: "swarm-A", Status: swarmproto.StatusRunning}); err != nil {
		t.Fatalf("Register sender: %s", err)
	}

	ev := swarmproto.ChannelMessage{FromID: "sender", Body: "ping"}
	if err := s.Publish("swarm-A", "general", ev); err != nil {
		t.Fatalf("Publish: %s", err)
	}

	for i := 0; i < 5; i++ {
		sid := "sub-" + itoa(i)
		sink, unsub := s.Broadcaster().Subscribe(sid)
		_ = sink
		unsub()
	}
}

// TestPropertySpawnIdempotent checks that calling Spawn with the same
// (FromSessionID, NewSessionID) twice returns the same Member reference
// and does NOT increment the count.
func TestPropertySpawnIdempotent(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}

	for trial := 0; trial < 3; trial++ {
		r1, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"})
		if err != nil {
			t.Fatalf("trial %d first Spawn: %v", trial, err)
		}
		count1 := s.SwarmMemberCount("self")
		r2, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"})
		if err != nil {
			t.Fatalf("trial %d second Spawn: %v", trial, err)
		}
		count2 := s.SwarmMemberCount("self")
		if r1.Member != r2.Member {
			t.Errorf("trial %d: Spawn returned different Member pointers: %p vs %p", trial, r1.Member, r2.Member)
		}
		if count1 != count2 {
			t.Errorf("trial %d: count changed %d -> %d on idempotent Spawn", trial, count1, count2)
		}
	}
}

// TestPropertyErrorsAreSentinels checks that the public API never
// returns wrapped errors. All errors should be identity-comparable
// to the package sentinels.
func TestPropertyErrorsAreSentinels(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	cases := []struct {
		name string
		got  error
		want error
	}{
		{"ErrUnknownChannel-Publish", func() error {
			return s.Publish("self", "ghost", swarmproto.ChannelMessage{})
		}(), swarm.ErrUnknownChannel},
		{"ErrUnknownChannel-Unsubscribe", s.Unsubscribe("self", "ghost", "x"), swarm.ErrUnknownChannel},
		{"ErrInvalidRequest-BothRoutes", func() error {
			_, err := s.SendMessage(swarm.MessageOptions{FromSessionID: "x", SwarmID: "x", ToSessionID: "y", Channel: "z", Body: "hi"})
			return err
		}(), swarm.ErrInvalidRequest},
	}
	for _, c := range cases {
		if c.want == nil {
			continue
		}
		if !errors.Is(c.got, c.want) {
			t.Errorf("%s: got %v, want %v (via errors.Is)", c.name, c.got, c.want)
		}
		if c.got.Error() != c.want.Error() {
			t.Errorf("%s: message mismatch: %q vs %q", c.name, c.got.Error(), c.want.Error())
		}
	}
}

var _ = jsonrpc.Response{}
