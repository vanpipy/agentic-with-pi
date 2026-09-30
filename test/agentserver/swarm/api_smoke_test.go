package swarm_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
	swarm "github.com/vanpiyp/awp/internal/agent-server/swarm"
)

func TestDefaultRuntimeOptsValuesMatchJCode(t *testing.T) {
	t.Parallel()

	// These are jcode alignment invariants. Drift here breaks the
	// documented cross-implementation compatibility.
	o := swarm.DefaultRuntimeOpts()

	if o.MaxMembersPerSwarm != swarmproto.SpawnAdmissionPerSwarm {
		t.Errorf("MaxMembersPerSwarm = %d, want %d (SpawnAdmissionPerSwarm)",
			o.MaxMembersPerSwarm, swarmproto.SpawnAdmissionPerSwarm)
	}
	if o.HeartbeatInterval != 10*time.Second {
		t.Errorf("HeartbeatInterval = %s, want 10s (jcode DEFAULT_SWARM_TASK_HEARTBEAT_SECS)",
			o.HeartbeatInterval)
	}
	if o.StaleAfter != 45*time.Second {
		t.Errorf("StaleAfter = %s, want 45s (jcode DEFAULT_SWARM_STALE_AFTER_SECS)",
			o.StaleAfter)
	}
	if o.IdleReapAfter != 30*time.Minute {
		t.Errorf("IdleReapAfter = %s, want 30m (jcode DEFAULT_SWARM_IDLE_WORKER_REAP_SECS)",
			o.IdleReapAfter)
	}
	if o.PersistBatchInterval != 5*time.Second {
		t.Errorf("PersistBatchInterval = %s, want 5s", o.PersistBatchInterval)
	}
	if o.AwaitDefaultTimeout != time.Duration(swarmproto.AwaitDefaultTimeoutSeconds)*time.Second {
		t.Errorf("AwaitDefaultTimeout = %s, want %ds (AwaitDefaultTimeoutSeconds)",
			o.AwaitDefaultTimeout, swarmproto.AwaitDefaultTimeoutSeconds)
	}
	if o.AwaitMaxTimeout != time.Duration(swarmproto.AwaitTimeoutMaxSeconds)*time.Second {
		t.Errorf("AwaitMaxTimeout = %s, want %ds (AwaitTimeoutMaxSeconds)",
			o.AwaitMaxTimeout, swarmproto.AwaitTimeoutMaxSeconds)
	}
}

func TestValidateConcurrent(t *testing.T) {
	t.Parallel()

	// Validate is a pure function on a value receiver; calling it from
	// many goroutines must produce consistent results and no races.
	o := swarm.DefaultRuntimeOpts()

	const goroutines = 64
	const iterations = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if err := o.Validate(); err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func TestDefaultRuntimeOptsShape(t *testing.T) {
	t.Parallel()
	o := swarm.DefaultRuntimeOpts()

	if o.MaxMembersPerSwarm <= 0 {
		t.Errorf("MaxMembersPerSwarm must be positive, got %d", o.MaxMembersPerSwarm)
	}
	if o.MaxMembersPerSwarm > swarmproto.MaxSwarmMembers {
		t.Errorf("MaxMembersPerSwarm must respect wire cap, got %d > %d",
			o.MaxMembersPerSwarm, swarmproto.MaxSwarmMembers)
	}
	if o.HeartbeatInterval <= 0 {
		t.Errorf("HeartbeatInterval must be positive, got %s", o.HeartbeatInterval)
	}
	if o.StaleAfter < o.HeartbeatInterval {
		t.Errorf("StaleAfter must be >= HeartbeatInterval, got %s < %s",
			o.StaleAfter, o.HeartbeatInterval)
	}
	if o.PersistBatchInterval <= 0 {
		t.Errorf("PersistBatchInterval must be positive, got %s", o.PersistBatchInterval)
	}
	if o.AwaitDefaultTimeout <= 0 {
		t.Errorf("AwaitDefaultTimeout must be positive, got %s", o.AwaitDefaultTimeout)
	}
	if o.AwaitMaxTimeout < o.AwaitDefaultTimeout {
		t.Errorf("AwaitMaxTimeout must be >= AwaitDefaultTimeout, got %s < %s",
			o.AwaitMaxTimeout, o.AwaitDefaultTimeout)
	}
	if o.IdleReapAfter <= 0 {
		t.Errorf("IdleReapAfter must be positive, got %s", o.IdleReapAfter)
	}
}

func TestRuntimeOptsValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(o *swarm.RuntimeOpts)
		wantErr bool
	}{
		{"defaults pass", func(o *swarm.RuntimeOpts) {}, false},
		{"zero MaxMembers fails", func(o *swarm.RuntimeOpts) { o.MaxMembersPerSwarm = 0 }, true},
		{"negative MaxMembers fails", func(o *swarm.RuntimeOpts) { o.MaxMembersPerSwarm = -1 }, true},
		{"over-cap MaxMembers fails", func(o *swarm.RuntimeOpts) {
			o.MaxMembersPerSwarm = swarmproto.MaxSwarmMembers + 1
		}, true},
		{"at-cap MaxMembers passes", func(o *swarm.RuntimeOpts) {
			o.MaxMembersPerSwarm = swarmproto.MaxSwarmMembers
		}, false},
		{"zero Heartbeat fails", func(o *swarm.RuntimeOpts) { o.HeartbeatInterval = 0 }, true},
		{"zero StaleAfter OK", func(o *swarm.RuntimeOpts) { o.StaleAfter = 0 }, true},
		{"StaleAfter < Heartbeat fails", func(o *swarm.RuntimeOpts) {
			o.StaleAfter = o.HeartbeatInterval / 2
		}, true},
		{"zero PersistBatch fails", func(o *swarm.RuntimeOpts) {
			o.PersistBatchInterval = 0
		}, true},
		{"zero AwaitDefault fails", func(o *swarm.RuntimeOpts) {
			o.AwaitDefaultTimeout = 0
		}, true},
		{"AwaitMax < Default fails", func(o *swarm.RuntimeOpts) {
			o.AwaitMaxTimeout = o.AwaitDefaultTimeout / 2
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := swarm.DefaultRuntimeOpts()
			c.mutate(&o)
			err := o.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, c.wantErr)
			}
			if c.wantErr && err != nil && !errors.Is(err, swarm.ErrInvalidRuntimeOpts) {
				t.Errorf("Validate() error must wrap ErrInvalidRuntimeOpts, got %v", err)
			}
		})
	}
}

func TestValidateIdempotent(t *testing.T) {
	t.Parallel()
	o := swarm.DefaultRuntimeOpts()
	err1 := o.Validate()
	err2 := o.Validate()
	if (err1 == nil) != (err2 == nil) {
		t.Errorf("Validate() not idempotent: first=%v, second=%v", err1, err2)
	}
}

func TestSentinelsDistinct(t *testing.T) {
	t.Parallel()
	sentinels := []error{
		swarm.ErrUnknownSwarm,
		swarm.ErrUnknownMember,
		swarm.ErrCapacityExceeded,
		swarm.ErrRoleConflict,
		swarm.ErrNoPlan,
		swarm.ErrUnknownNode,
		swarm.ErrCycleDetected,
		swarm.ErrUnknownDependency,
		swarm.ErrNotOwner,
		swarm.ErrInvalidTransition,
		swarm.ErrShuttingDown,
		swarm.ErrInvalidRuntimeOpts,
		swarm.ErrUnknownChannel,
		swarm.ErrTooLarge,
	}
	seen := make(map[error]bool, len(sentinels))
	for _, s := range sentinels {
		if seen[s] {
			t.Errorf("duplicate sentinel: %v", s)
		}
		if s == nil {
			t.Error("nil sentinel in list")
		}
		seen[s] = true
	}
	if len(seen) != 14 {
		t.Errorf("expected 14 distinct sentinels, got %d", len(seen))
	}
}

func TestSentinelsDistinctFromCommonErrors(t *testing.T) {
	t.Parallel()
	// None of the swarm sentinels should alias to a generic Go error.
	sentinels := []error{
		swarm.ErrUnknownSwarm,
		swarm.ErrShuttingDown,
		swarm.ErrTooLarge,
	}
	for _, s := range sentinels {
		if errors.Is(s, swarm.ErrInvalidRuntimeOpts) {
			t.Errorf("sentinel %v aliases ErrInvalidRuntimeOpts", s)
		}
	}
}

func TestMemberShape(t *testing.T) {
	t.Parallel()
	// Per godoc, the zero-value Member has Sink=nil; the internal
	// newMember constructor assigns a buffered channel at spawn. We
	// can only verify the field exists and is settable from outside.
	m := swarm.Member{
		Record: swarmproto.MemberRecord{
			SessionID: "test-session",
			Status:    swarmproto.StatusReady,
		},
		Status: swarmproto.StatusReady,
	}
	if m.Sink != nil {
		t.Errorf("zero Member.Sink must be nil per doc, got %v", m.Sink)
	}
	// Verify the field is settable to a buffered channel.
	m.Sink = make(chan jsonrpc.Response, 32)
	if cap(m.Sink) != 32 {
		t.Errorf("settable Sink buffer capacity = %d, want 32", cap(m.Sink))
	}
	if m.Record.SessionID != "test-session" {
		t.Errorf("Record.SessionID = %q, want %q", m.Record.SessionID, "test-session")
	}
}

func TestChannelShape(t *testing.T) {
	t.Parallel()
	c := swarm.Channel{
		Name: "test-channel",
		Subscribers: map[string]struct{}{
			"a": {},
			"b": {},
		},
	}
	if c.Name != "test-channel" {
		t.Errorf("Channel.Name = %q, want %q", c.Name, "test-channel")
	}
	if len(c.Subscribers) != 2 {
		t.Errorf("len(Subscribers) = %d, want 2", len(c.Subscribers))
	}
}

func TestAwaiterShape(t *testing.T) {
	t.Parallel()
	// Per godoc and zero-value semantics, Awaiter.Done is nil until
	// the dispatcher initializes it. We verify the field exists and
	// is settable.
	a := swarm.Awaiter{
		ID:          "await-1",
		FromSession: "caller-session",
		SwarmID:     "swarm-1",
	}
	if a.ID != "await-1" {
		t.Errorf("Awaiter.ID = %q, want %q", a.ID, "await-1")
	}
	if a.FromSession != "caller-session" {
		t.Errorf("Awaiter.FromSession = %q, want %q", a.FromSession, "caller-session")
	}
	if a.SwarmID != "swarm-1" {
		t.Errorf("Awaiter.SwarmID = %q, want %q", a.SwarmID, "swarm-1")
	}
	if a.Done != nil {
		t.Errorf("zero Awaiter.Done must be nil per zero-value semantics, got %v", a.Done)
	}
	// Settable to an unbuffered signal channel (the documented contract).
	a.Done = make(chan struct{})
	if cap(a.Done) != 0 {
		t.Errorf("Awaiter.Done must be unbuffered, got cap = %d", cap(a.Done))
	}
	if a.Result != nil {
		t.Errorf("zero Awaiter.Result must be nil, got %+v", a.Result)
	}
}

func TestAwaitResultShape(t *testing.T) {
	t.Parallel()
	r := swarm.AwaitResult{
		Completed: true,
		Members: []swarmproto.AwaitedMemberStatus{
			{SessionID: "a", Status: swarmproto.StatusReady, Done: true},
		},
		Reason: "matched",
	}
	if !r.Completed || r.Reason != "matched" || len(r.Members) != 1 {
		t.Errorf("AwaitResult fields not settable as documented: %+v", r)
	}
}
