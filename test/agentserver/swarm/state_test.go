package swarm_test

import (
	"errors"
	"testing"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/agent-server/swarm"
)

func TestNewSwarmStateDefaults(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = 16
	s := swarm.NewSwarmState(opts)
	if s.MemberCount() != 0 {
		t.Errorf("MemberCount = %d, want 0", s.MemberCount())
	}
	if s.SwarmMemberCount("anything") != 0 {
		t.Errorf("SwarmMemberCount = %d, want 0", s.SwarmMemberCount("anything"))
	}
	if s.IsShuttingDown() {
		t.Error("IsShuttingDown = true, want false")
	}
}

func TestSpawnZeroMaxMembersDefaults(t *testing.T) {
	t.Parallel()
	s := swarm.NewSwarmState(swarm.RuntimeOpts{})
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	for i := 0; i < 31; i++ {
		if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: idFor(i + 1)}); err != nil {
			t.Fatalf("child Spawn %d: %v", i, err)
		}
	}
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "overflow"})
	if !errors.Is(err, swarm.ErrCapacityExceeded) {
		t.Errorf("err = %v, want ErrCapacityExceeded", err)
	}
}

func idFor(i int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	if i < len(alphabet) {
		return string(alphabet[i])
	}
	return idFor(i/len(alphabet)) + string(alphabet[i%len(alphabet)])
}

func TestCloseMarksShuttingDown(t *testing.T) {
	t.Parallel()
	s := newState()
	s.Close()
	if !s.IsShuttingDown() {
		t.Error("IsShuttingDown = false after Close, want true")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	s := newState()
	s.Close()
	s.Close()
	if !s.IsShuttingDown() {
		t.Error("IsShuttingDown = false after double Close, want true")
	}
}

func TestDoneChannelClosesOnClose(t *testing.T) {
	t.Parallel()
	s := newState()
	select {
	case <-s.Done():
		t.Fatal("Done closed prematurely")
	default:
	}
	s.Close()
	select {
	case <-s.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Done did not close after Close")
	}
}

func TestSetClockAffectsNow(t *testing.T) {
	t.Parallel()
	s := newState()
	fixed := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	s.SetClock(func() time.Time { return fixed })
	got := s.Now()
	if got.Unix() != fixed.Unix() {
		t.Errorf("Now = %v, want %v", got, fixed)
	}
}

func TestSetClockNilRestoresDefault(t *testing.T) {
	t.Parallel()
	s := newState()
	s.SetClock(func() time.Time { return time.Unix(0, 0) })
	s.SetClock(nil)
	got := s.Now()
	if got.Unix() <= 0 {
		t.Errorf("default clock not restored, got %v", got)
	}
}

func TestBroadcasterAccessible(t *testing.T) {
	t.Parallel()
	s := newState()
	b := s.Broadcaster()
	if b == nil {
		t.Fatal("Broadcaster must be non-nil")
	}
	_, unsub := b.Subscribe("test")
	unsub()
}

func TestCloseClosesBroadcaster(t *testing.T) {
	t.Parallel()
	s := newState()
	b := s.Broadcaster()
	ch, unsub := b.Subscribe("test")
	unsub()
	_ = ch
	s.Close()
	b.Publish(jsonrpc.Response{JSONRPC: "2.0", Event: "after-close"})
}

func TestGetMemberReturnsMember(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Register(newRecord("s1", "swarm-A")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	m, ok := s.GetMember("s1")
	if !ok {
		t.Fatal("GetMember(s1) ok=false")
	}
	if m.Record.SessionID != "s1" {
		t.Errorf("SessionID = %s, want s1", m.Record.SessionID)
	}
	if _, ok := s.GetMember("ghost"); ok {
		t.Error("GetMember(ghost) ok=true, want false")
	}
}

func TestSwarmMemberCountIgnoresOtherSwarm(t *testing.T) {
	t.Parallel()
	s := newState()
	for _, rec := range []swarmRecordPair{
		{"a1", "A"},
		{"a2", "A"},
		{"b1", "B"},
	} {
		if _, err := s.Register(newRecord(rec.id, rec.swarmID)); err != nil {
			t.Fatalf("Register %s: %v", rec.id, err)
		}
	}
	if got := s.SwarmMemberCount("A"); got != 2 {
		t.Errorf("SwarmMemberCount(A) = %d, want 2", got)
	}
	if got := s.SwarmMemberCount("B"); got != 1 {
		t.Errorf("SwarmMemberCount(B) = %d, want 1", got)
	}
	if got := s.SwarmMemberCount("ghost"); got != 0 {
		t.Errorf("SwarmMemberCount(ghost) = %d, want 0", got)
	}
}

type swarmRecordPair struct {
	id, swarmID string
}

func TestRuntimeOptsValidateRejectsZero(t *testing.T) {
	t.Parallel()
	opts := swarm.RuntimeOpts{}
	if err := opts.Validate(); err == nil {
		t.Error("Validate() = nil, want error for zero-value opts")
	}
}

func TestRuntimeOptsValidateAcceptsDefaults(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	if err := opts.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestRuntimeOptsValidateRejectsStaleShorterThanHeartbeat(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	opts.HeartbeatInterval = 10 * time.Second
	opts.StaleAfter = 5 * time.Second
	if err := opts.Validate(); err == nil {
		t.Error("Validate() = nil, want error when StaleAfter < HeartbeatInterval")
	}
}

func TestRuntimeOptsValidateRejectsTooManyMembers(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = 99999
	if err := opts.Validate(); err == nil {
		t.Error("Validate() = nil, want error when MaxMembersPerSwarm > MaxSwarmMembers")
	}
}
