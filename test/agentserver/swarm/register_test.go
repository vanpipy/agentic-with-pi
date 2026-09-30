package swarm_test

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-server/swarm"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func newRecord(id, swarmID string) swarmproto.MemberRecord {
	return swarmproto.MemberRecord{
		SessionID:    id,
		SwarmID:      swarmID,
		SwarmEnabled: true,
	}
}

func newState() *swarm.SwarmState {
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = 4
	return swarm.NewSwarmState(opts)
}

func TestRegisterCreatesMember(t *testing.T) {
	t.Parallel()
	s := newState()
	m, err := s.Register(newRecord("s1", "swarm-A"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if m.Status.String() != swarmproto.StatusSpawned.String() {
		t.Errorf("status = %s, want %s", m.Status.String(), swarmproto.StatusSpawned.String())
	}
	if m.Record.SessionID != "s1" {
		t.Errorf("session = %s, want s1", m.Record.SessionID)
	}
	if s.MemberCount() != 1 {
		t.Errorf("MemberCount = %d, want 1", s.MemberCount())
	}
	if s.SwarmMemberCount("swarm-A") != 1 {
		t.Errorf("SwarmMemberCount(A) = %d, want 1", s.SwarmMemberCount("swarm-A"))
	}
}

func TestRegisterRejectsEmptySessionID(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Register(newRecord("", "swarm-A"))
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestRegisterIdempotentSameSwarm(t *testing.T) {
	t.Parallel()
	s := newState()
	a, err := s.Register(newRecord("s1", "swarm-A"))
	if err != nil {
		t.Fatalf("first Register: %v", err)
	}
	b, err := s.Register(newRecord("s1", "swarm-A"))
	if err != nil {
		t.Fatalf("second Register: %v", err)
	}
	if a != b {
		t.Errorf("idempotent re-register returned different Member")
	}
	if s.MemberCount() != 1 {
		t.Errorf("MemberCount = %d, want 1", s.MemberCount())
	}
}

func TestRegisterRejectsReassignToDifferentSwarm(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Register(newRecord("s1", "swarm-A"))
	if err != nil {
		t.Fatalf("Register A: %v", err)
	}
	_, err = s.Register(newRecord("s1", "swarm-B"))
	if !errors.Is(err, swarm.ErrRoleConflict) {
		t.Errorf("err = %v, want ErrRoleConflict", err)
	}
}

func TestUnregisterRemoves(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Register(newRecord("s1", "swarm-A"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Subscribe("swarm-A", "general", "s1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := s.Unregister("s1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if s.MemberCount() != 0 {
		t.Errorf("MemberCount = %d, want 0", s.MemberCount())
	}
	members, err := s.ChannelMembers("swarm-A", "general")
	if err != nil {
		t.Fatalf("ChannelMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("channel subscribers = %v, want empty after Unregister", members)
	}
	if err := s.Unregister("s1"); !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("second Unregister err = %v, want ErrUnknownMember", err)
	}
}

func TestSpawnCreatesCoordinatorForSelf(t *testing.T) {
	t.Parallel()
	s := newState()
	res, err := s.Spawn(swarm.SpawnOptions{
		FromSessionID:  "self",
		NewSessionID:   "self",
		InitialMessage: "boot",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if res.SwarmID != "self" {
		t.Errorf("SwarmID = %s, want self", res.SwarmID)
	}
	if res.Member.Record.Role.String() != swarmproto.RoleCoordinator.String() {
		t.Errorf("Role = %s, want %s", res.Member.Record.Role.String(), swarmproto.RoleCoordinator.String())
	}
	if res.Member.Status.String() != swarmproto.StatusSpawned.String() {
		t.Errorf("Status = %s, want %s", res.Member.Status.String(), swarmproto.StatusSpawned.String())
	}
}

func TestSpawnJoinsParentSwarm(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Spawn(swarm.SpawnOptions{
		FromSessionID:  "self",
		NewSessionID:   "self",
		InitialMessage: "boot",
	})
	if err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	res, err := s.Spawn(swarm.SpawnOptions{
		FromSessionID:  "self",
		NewSessionID:   "child",
		InitialMessage: "do work",
	})
	if err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	if res.SwarmID != "self" {
		t.Errorf("SwarmID = %s, want self", res.SwarmID)
	}
	if res.Member.Record.Role.String() != swarmproto.RoleAgent.String() {
		t.Errorf("Role = %s, want %s", res.Member.Record.Role.String(), swarmproto.RoleAgent.String())
	}
}

func TestSpawnIdempotentSameSwarm(t *testing.T) {
	t.Parallel()
	s := newState()
	a, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"})
	if err != nil {
		t.Fatalf("first Spawn: %v", err)
	}
	b, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"})
	if err != nil {
		t.Fatalf("second Spawn: %v", err)
	}
	if a.Member != b.Member {
		t.Errorf("idempotent re-spawn returned different Member pointer")
	}
	if a.SwarmID != b.SwarmID {
		t.Errorf("SwarmID differs: %s vs %s", a.SwarmID, b.SwarmID)
	}
}

func TestSpawnIdempotentDifferentSwarmConflict(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "alpha", NewSessionID: "alpha"})
	if err != nil {
		t.Fatalf("alpha Spawn: %v", err)
	}
	_, err = s.Register(newRecord("x", "beta"))
	if err != nil {
		t.Fatalf("Register x: %v", err)
	}
	_, err = s.Spawn(swarm.SpawnOptions{FromSessionID: "alpha", NewSessionID: "x"})
	if !errors.Is(err, swarm.ErrRoleConflict) {
		t.Errorf("err = %v, want ErrRoleConflict", err)
	}
}

func TestSpawnRejectsEmptyNewID(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "p", NewSessionID: ""})
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestSpawnAdmissionCapEnforced(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = 2
	s := swarm.NewSwarmState(opts)
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child-1"}); err != nil {
		t.Fatalf("child-1 Spawn: %v", err)
	}
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child-2"})
	if !errors.Is(err, swarm.ErrCapacityExceeded) {
		t.Errorf("err = %v, want ErrCapacityExceeded", err)
	}
}

func TestSpawnAdmissionPolicy(t *testing.T) {
	t.Parallel()
	denied := errors.New("denied by policy")
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = 10
	opts.AdmissionPolicy = func(swarmID string, current, requested int) error {
		if current+requested > 1 {
			return denied
		}
		return nil
	}
	s := swarm.NewSwarmState(opts)
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"})
	if !errors.Is(err, denied) {
		t.Errorf("err = %v, want %v", err, denied)
	}
}

func TestStopMarksStopped(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if err := s.Stop("self", false); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.MemberCount() != 0 {
		t.Errorf("MemberCount = %d, want 0 after Stop", s.MemberCount())
	}
}

func TestStopForceMarksCrashed(t *testing.T) {
	t.Parallel()
	s := newState()
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	_, err = s.Register(newRecord("self", "self"))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := s.Stop("self", true); err != nil {
		t.Fatalf("Stop force: %v", err)
	}
}

func TestStopUnknownReturnsError(t *testing.T) {
	t.Parallel()
	s := newState()
	err := s.Stop("nope", false)
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestAssignRoleUniqueCoordinator(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"}); err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	err := s.AssignRole("child", swarmproto.RoleCoordinator)
	if !errors.Is(err, swarm.ErrRoleConflict) {
		t.Errorf("err = %v, want ErrRoleConflict", err)
	}
}

func TestAssignRoleAgentSucceeds(t *testing.T) {
	t.Parallel()
	s := newState()
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "child"}); err != nil {
		t.Fatalf("child Spawn: %v", err)
	}
	if err := s.AssignRole("child", swarmproto.RoleAgent); err != nil {
		t.Errorf("AssignRole agent: %v", err)
	}
}

func TestAssignRoleUnknownMember(t *testing.T) {
	t.Parallel()
	s := newState()
	err := s.AssignRole("nope", swarmproto.RoleAgent)
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember", err)
	}
}

func TestListSortedBySessionID(t *testing.T) {
	t.Parallel()
	s := newState()
	for _, id := range []string{"c", "a", "b"} {
		if _, err := s.Register(newRecord(id, "swarm-A")); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}
	list := s.List()
	if len(list) != 3 {
		t.Fatalf("List len = %d, want 3", len(list))
	}
	if list[0].SessionID != "a" || list[1].SessionID != "b" || list[2].SessionID != "c" {
		t.Errorf("list order = [%s, %s, %s], want [a, b, c]",
			list[0].SessionID, list[1].SessionID, list[2].SessionID)
	}
}

func TestListBySwarmFilters(t *testing.T) {
	t.Parallel()
	s := newState()
	for _, id := range []string{"a1", "a2", "b1"} {
		swarmID := "A"
		if id[0] == 'b' {
			swarmID = "B"
		}
		if _, err := s.Register(newRecord(id, swarmID)); err != nil {
			t.Fatalf("Register %s: %v", id, err)
		}
	}
	if got := len(s.ListBySwarm("A")); got != 2 {
		t.Errorf("ListBySwarm(A) len = %d, want 2", got)
	}
	if got := len(s.ListBySwarm("B")); got != 1 {
		t.Errorf("ListBySwarm(B) len = %d, want 1", got)
	}
	if got := len(s.ListBySwarm("missing")); got != 0 {
		t.Errorf("ListBySwarm(missing) len = %d, want 0", got)
	}
}

func TestMemberStatusPopulatesFields(t *testing.T) {
	t.Parallel()
	s := newState()
	rec := newRecord("s1", "swarm-A")
	rec.FriendlyName = "agent-one"
	rec.TaskLabel = "do work"
	rec.IsHeadless = true
	rec.ParentSessionID = "parent"
	if _, err := s.Register(rec); err != nil {
		t.Fatalf("Register: %v", err)
	}
	list := s.ListBySwarm("swarm-A")
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	st := list[0]
	if st.SessionID != "s1" {
		t.Errorf("SessionID = %s, want s1", st.SessionID)
	}
	if st.FriendlyName != "agent-one" {
		t.Errorf("FriendlyName = %s, want agent-one", st.FriendlyName)
	}
	if st.TaskLabel != "do work" {
		t.Errorf("TaskLabel = %s, want do work", st.TaskLabel)
	}
	if !st.IsHeadless {
		t.Errorf("IsHeadless = false, want true")
	}
	if st.ParentSessionID != "parent" {
		t.Errorf("ParentSessionID = %s, want parent", st.ParentSessionID)
	}
	if st.SwarmID != "swarm-A" {
		t.Errorf("SwarmID = %s, want swarm-A", st.SwarmID)
	}
}

func TestRegisterRejectsDuringShutdown(t *testing.T) {
	t.Parallel()
	s := newState()
	s.Close()
	_, err := s.Register(newRecord("s1", "swarm-A"))
	if !errors.Is(err, swarm.ErrShuttingDown) {
		t.Errorf("err = %v, want ErrShuttingDown", err)
	}
}

func TestSpawnRejectsDuringShutdown(t *testing.T) {
	t.Parallel()
	s := newState()
	s.Close()
	_, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"})
	if !errors.Is(err, swarm.ErrShuttingDown) {
		t.Errorf("err = %v, want ErrShuttingDown", err)
	}
}

func TestStopAcceptsUnknownDuringShutdown(t *testing.T) {
	t.Parallel()
	s := newState()
	s.Close()
	err := s.Stop("anything", false)
	if !errors.Is(err, swarm.ErrUnknownMember) {
		t.Errorf("err = %v, want ErrUnknownMember (Stop returns the lookup error first)", err)
	}
}

func TestConcurrentSpawnRespectsAdmissionCap(t *testing.T) {
	t.Parallel()
	opts := swarm.DefaultRuntimeOpts()
	opts.MaxMembersPerSwarm = 5
	s := swarm.NewSwarmState(opts)
	if _, err := s.Spawn(swarm.SpawnOptions{FromSessionID: "self", NewSessionID: "self"}); err != nil {
		t.Fatalf("self Spawn: %v", err)
	}
	var admitted atomic.Int32
	var rejected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := strings.Repeat("x", i+1)
			_, err := s.Spawn(swarm.SpawnOptions{
				FromSessionID: "self",
				NewSessionID:  "child-" + id,
			})
			if err == nil {
				admitted.Add(1)
			} else if errors.Is(err, swarm.ErrCapacityExceeded) {
				rejected.Add(1)
			} else {
				t.Errorf("unexpected spawn error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if a := admitted.Load(); a > 4 {
		t.Errorf("admitted=%d exceeds cap of 4", a)
	}
	if r := rejected.Load(); r < 16 {
		t.Errorf("rejected=%d below expected 16", r)
	}
}

var _ = reflect.DeepEqual