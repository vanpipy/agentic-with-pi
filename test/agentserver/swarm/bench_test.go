package swarm_test

import (
	"strings"
	"testing"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
	"github.com/vanpiyp/awp/internal/agent-server/swarm"
)

func BenchmarkRegisterUnregister(b *testing.B) {
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sid := "sess-" + itoa(i)
		swarmID := "swarm-A"
		rec := swarmproto.MemberRecord{
			SessionID: sid,
			SwarmID:   swarmID,
			Status:    swarmproto.StatusReady,
		}
		if _, err := s.Register(rec); err != nil {
			b.Fatalf("Register: %v", err)
		}
		if err := s.Unregister(sid); err != nil {
			b.Fatalf("Unregister: %v", err)
		}
	}
}

func BenchmarkSpawnConcurrent(b *testing.B) {
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		b.Fatalf("self Spawn: %v", err)
	}

	b.ResetTimer()
	i := 0
	for n := 0; n < b.N; n++ {
		id := "child-" + itoa(i)
		i++
		if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: id}); err != nil {
			if err == swarm.ErrCapacityExceeded {
				break
			}
			b.Fatalf("Spawn: %v", err)
		}
	}
}

func BenchmarkSpawnAdmitAtCap(b *testing.B) {
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		b.Fatalf("self Spawn: %v", err)
	}
	for i := 0; i < int(opts.MaxMembersPerSwarm-1); i++ {
		if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "c-" + itoa(i)}); err != nil {
			b.Fatalf("priming Spawn %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "overflow"})
		if err != swarm.ErrCapacityExceeded {
			b.Fatalf("err = %v, want ErrCapacityExceeded", err)
		}
	}
}

func BenchmarkPublish(b *testing.B) {
	opts := swarm.DefaultRuntimeOpts()
	s := swarm.NewSwarmState(opts)
	defer s.Close()

	if _, err := s.Register(swarmproto.MemberRecord{SessionID: "sender", SwarmID: "swarm-A", Status: swarmproto.StatusRunning}); err != nil {
		b.Fatalf("Register: %v", err)
	}
	for i := 0; i < 32; i++ {
		sid := "sub-" + itoa(i)
		if _, err := s.Register(swarmproto.MemberRecord{SessionID: sid, SwarmID: "swarm-A", Status: swarmproto.StatusRunning}); err != nil {
			b.Fatalf("Register: %v", err)
		}
		if err := s.Subscribe("swarm-A", "general", sid); err != nil {
			b.Fatalf("Subscribe: %v", err)
		}
	}
	subID := "sub-0"
	sink, unsub := s.Broadcaster().Subscribe(subID)
	defer unsub()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := swarmproto.ChannelMessage{
			FromID: "sender",
			Body:   "bench",
		}
		if err := s.Publish("swarm-A", "general", msg); err != nil {
			b.Fatalf("Publish: %v", err)
		}
		select {
		case <-sink:
		default:
		}
	}
}

func BenchmarkBroadcasterSubscribe(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc := swarm.NewBroadcaster()
		sid := "s-" + itoa(i)
		sink, unsub := bc.Subscribe(sid)
		if sink == nil {
			b.Fatalf("nil sink")
		}
		unsub()
		bc.Close()
	}
}

func BenchmarkBroadcasterPublish(b *testing.B) {
	bc := swarm.NewBroadcaster()
	const N = 32
	for i := 0; i < N; i++ {
		_, _ = bc.Subscribe("sub-" + itoa(i))
	}
	ev := jsonrpc.Response{
		JSONRPC: "2.0",
		Event:   swarmproto.EventSwarmChannelMessage,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.Publish(ev)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

var _ = time.Second
var _ = strings.Repeat
