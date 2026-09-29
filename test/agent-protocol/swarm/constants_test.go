package swarm_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

func TestCommMethodCount(t *testing.T) {
	// Per ADR §5, there should be exactly 29 comm methods.
	const expected = 29
	all := []string{
		swarm.MethodCommShare,
		swarm.MethodCommRead,
		swarm.MethodCommMessage,
		swarm.MethodCommList,
		swarm.MethodCommListChannels,
		swarm.MethodCommChannelMembers,
		swarm.MethodCommProposePlan,
		swarm.MethodCommApprovePlan,
		swarm.MethodCommRejectPlan,
		swarm.MethodCommSeedGraph,
		swarm.MethodCommExpandNode,
		swarm.MethodCommCompleteNode,
		swarm.MethodCommInjectGap,
		swarm.MethodCommSpawn,
		swarm.MethodCommListModels,
		swarm.MethodCommStop,
		swarm.MethodCommAssignRole,
		swarm.MethodCommSummary,
		swarm.MethodCommStatus,
		swarm.MethodCommReport,
		swarm.MethodCommReadContext,
		swarm.MethodCommResyncPlan,
		swarm.MethodCommPlanStatus,
		swarm.MethodCommAssignTask,
		swarm.MethodCommAssignNext,
		swarm.MethodCommTaskControl,
		swarm.MethodCommSubscribeChannel,
		swarm.MethodCommUnsubscribeChannel,
		swarm.MethodCommAwaitMembers,
	}
	if len(all) != expected {
		t.Errorf("comm method count = %d, want %d", len(all), expected)
	}
	seen := map[string]bool{}
	for _, m := range all {
		if m == "" {
			t.Error("comm method is empty string")
		}
		if !strings.HasPrefix(m, "comm.") {
			t.Errorf("comm method %q does not start with comm.", m)
		}
		if seen[m] {
			t.Errorf("duplicate comm method: %q", m)
		}
		seen[m] = true
	}
}

func TestSwarmEventCount(t *testing.T) {
	// Per ADR §5, there should be 25 ServerEvent additions.
	const expected = 25
	all := []string{
		swarm.EventSwarmStatus,
		swarm.EventSwarmMemberUpdate,
		swarm.EventSwarmChannelMessage,
		swarm.EventCommSpawnResponse,
		swarm.EventCommMessageResponse,
		swarm.EventCommListResponse,
		swarm.EventCommListChannelsResponse,
		swarm.EventCommChannelMembersResponse,
		swarm.EventCommListModelsResponse,
		swarm.EventCommSummaryResponse,
		swarm.EventCommStatusResponse,
		swarm.EventCommReportResponse,
		swarm.EventCommReadContextResponse,
		swarm.EventCommAwaitMembersResponse,
		swarm.EventCommResyncPlanResponse,
		swarm.EventCommPlanStatusResponse,
		swarm.EventCommAssignTaskResponse,
		swarm.EventCommAssignNextResponse,
		swarm.EventCommTaskControlResponse,
		swarm.EventCommShareResponse,
		swarm.EventCommReadResponse,
		swarm.EventCommError,
		swarm.EventSwarmPlan,
		swarm.EventSwarmPlanProposal,
		swarm.EventSwarmPlanProposalResponse,
	}
	if len(all) != expected {
		t.Errorf("swarm event count = %d, want %d", len(all), expected)
	}
	seen := map[string]bool{}
	for _, e := range all {
		if e == "" {
			t.Error("swarm event is empty string")
		}
		if seen[e] {
			t.Errorf("duplicate swarm event: %q", e)
		}
		seen[e] = true
	}
}

func TestIsCommMethod(t *testing.T) {
	if !swarm.IsCommMethod(swarm.MethodCommSpawn) {
		t.Error("IsCommMethod(MethodCommSpawn) = false, want true")
	}
	if !swarm.IsCommMethod(swarm.MethodCommRead) {
		t.Error("IsCommMethod(MethodCommRead) = false, want true")
	}
	if swarm.IsCommMethod("session.create") {
		t.Error("IsCommMethod(\"session.create\") = true, want false")
	}
	if swarm.IsCommMethod("") {
		t.Error("IsCommMethod(\"\") = true, want false")
	}
}

func TestIsSwarmEvent(t *testing.T) {
	if !swarm.IsSwarmEvent(swarm.EventSwarmPlan) {
		t.Error("IsSwarmEvent(EventSwarmPlan) = false, want true")
	}
	if !swarm.IsSwarmEvent(swarm.EventCommReadResponse) {
		t.Error("IsSwarmEvent(EventCommReadResponse) = false, want true")
	}
	if swarm.IsSwarmEvent("session.started") {
		t.Error("IsSwarmEvent(\"session.started\") = true, want false")
	}
	if swarm.IsSwarmEvent("") {
		t.Error("IsSwarmEvent(\"\") = true, want false")
	}
}

func TestHardCaps(t *testing.T) {
	if swarm.SpawnAdmissionPerSwarm != 32 {
		t.Errorf("SpawnAdmissionPerSwarm = %d, want 32", swarm.SpawnAdmissionPerSwarm)
	}
	if swarm.HeartbeatInterval.Seconds() != 10 {
		t.Errorf("HeartbeatInterval = %v, want 10s", swarm.HeartbeatInterval)
	}
	if swarm.StaleAfter.Seconds() != 45 {
		t.Errorf("StaleAfter = %v, want 45s", swarm.StaleAfter)
	}
	if swarm.IdleReapAfter.Minutes() != 30 {
		t.Errorf("IdleReapAfter = %v, want 30m", swarm.IdleReapAfter)
	}
	if swarm.MaxTLDRChars != 200 {
		t.Errorf("MaxTLDRChars = %d, want 200", swarm.MaxTLDRChars)
	}
	if swarm.TLDRRequiredOverChars != 240 {
		t.Errorf("TLDRRequiredOverChars = %d, want 240", swarm.TLDRRequiredOverChars)
	}
	if swarm.MaxPlanItems != 1024 {
		t.Errorf("MaxPlanItems = %d, want 1024", swarm.MaxPlanItems)
	}
	if swarm.MaxSharedContextKeyBytes != 4*1024 {
		t.Errorf("MaxSharedContextKeyBytes = %d, want %d", swarm.MaxSharedContextKeyBytes, 4*1024)
	}
	if swarm.MaxSharedContextKeysPerSwarm != 100 {
		t.Errorf("MaxSharedContextKeysPerSwarm = %d, want 100", swarm.MaxSharedContextKeysPerSwarm)
	}
	if swarm.MaxChannelMessageBodyBytes != 32*1024 {
		t.Errorf("MaxChannelMessageBodyBytes = %d, want %d", swarm.MaxChannelMessageBodyBytes, 32*1024)
	}
	if swarm.MaxArtifactJSONBytes != 16*1024 {
		t.Errorf("MaxArtifactJSONBytes = %d, want %d", swarm.MaxArtifactJSONBytes, 16*1024)
	}
	if swarm.AwaitDefaultTimeoutSeconds != 3600 {
		t.Errorf("AwaitDefaultTimeoutSeconds = %d, want 3600", swarm.AwaitDefaultTimeoutSeconds)
	}
	if swarm.AwaitTimeoutMaxSeconds != 24*3600 {
		t.Errorf("AwaitTimeoutMaxSeconds = %d, want %d", swarm.AwaitTimeoutMaxSeconds, 24*3600)
	}
}

func TestErrorCodes(t *testing.T) {
	if swarm.ErrUnknownSession == "" {
		t.Error("ErrUnknownSession is empty")
	}
	if swarm.ErrCapacityExceeded == "" {
		t.Error("ErrCapacityExceeded is empty")
	}
	if swarm.ErrCycleDetected == "" {
		t.Error("ErrCycleDetected is empty")
	}
	if swarm.ErrTLDRMissing == "" {
		t.Error("ErrTLDRMissing is empty")
	}
	if swarm.ErrNotCoordinator == "" {
		t.Error("ErrNotCoordinator is empty")
	}
}
