package swarm

import "time"

// Hard caps enforced server-side in internal/agent-server/swarm. These
// values mirror ~/Project/jcode defaults (see ADR §15 D10-D13) and
// are the single source of truth. Server-side guards return
// swarm.too_large / swarm.capacity_exceeded when limits are exceeded.
const (
	// MaxTLDRChars caps the length of a `tldr` field on messages and
	// completion reports. Clients sending longer values get a
	// swarm.too_large error.
	MaxTLDRChars = 200

	// TLDRRequiredOverChars is the body-length threshold above which
	// a `tldr` field becomes mandatory. Server rejects messages
	// shorter than this without a tldr.
	TLDRRequiredOverChars = 240

	// MaxCompletionReportChars caps comm.report message body.
	MaxCompletionReportChars = 4000

	// MaxPlanItems caps SeedGraph input; deeper DAGs require multiple
	// ExpandNode calls.
	MaxPlanItems = 1024

	// MaxSwarmMembers caps concurrent members per swarm.
	MaxSwarmMembers = 1000

	// MaxTaskLabelChars caps the friendly task label.
	MaxTaskLabelChars = 48

	// MaxSharedContextKeyBytes caps each shared-context entry value.
	MaxSharedContextKeyBytes = 4 * 1024

	// MaxSharedContextKeysPerSwarm caps the number of distinct keys
	// per swarm.
	MaxSharedContextKeysPerSwarm = 100

	// MaxChannelNameBytes caps channel names.
	MaxChannelNameBytes = 64

	// MaxChannelMessageBodyBytes caps comm.message body.
	MaxChannelMessageBodyBytes = 32 * 1024

	// MaxArtifactJSONBytes caps the artifact_json payload on
	// comm.complete_node in deep mode.
	MaxArtifactJSONBytes = 16 * 1024

	// SpawnAdmissionPerSwarm mirrors jcode's
	// agents.swarm_max_concurrent_agents default.
	// See crates/jcode-base/src/config/default_file.rs:421.
	SpawnAdmissionPerSwarm = 32

	// AwaitDefaultTimeoutSeconds is the default deadline for
	// comm.await_members when the caller omits timeout_secs.
	AwaitDefaultTimeoutSeconds = 3600

	// AwaitTimeoutMaxSeconds caps caller-requested timeouts so a
	// crashed server cannot accumulate forever-awaiting watchers.
	AwaitTimeoutMaxSeconds = 24 * 3600
)

// Heartbeat / idle-reap intervals. jcode defaults from
// crates/jcode-app-core/src/server/swarm.rs:89-90.
// Configurable via env: AWP_SWARM_HEARTBEAT_SECS, AWP_SWARM_STALE_SECS,
// AWP_SWARM_IDLE_REAP_SECS. The persisted state survives restart at
// the configured value.
var (
	HeartbeatInterval = 10 * time.Second
	StaleAfter        = 45 * time.Second
	IdleReapAfter     = 30 * time.Minute
	PersistBatchAfter = 5 * time.Second
)

// Method names on the wire. The awp transport uses dotted names; jcode
// uses snake_case enum variants. Translation is mechanical:
//
//	jcode Request::CommSpawn -> wire "comm.spawn"
//	jcode Request::CommAwaitMembers -> wire "comm.await_members"
//	...
//
// All 29 method constants below are exhaustive: the wire dispatcher in
// internal/agent-server/swarm/handle.go accepts exactly this set and
// 4xx's everything else.
const (
	MethodCommShare              = "comm.share"
	MethodCommRead               = "comm.read"
	MethodCommMessage            = "comm.message"
	MethodCommList               = "comm.list"
	MethodCommListChannels       = "comm.list_channels"
	MethodCommChannelMembers     = "comm.channel_members"
	MethodCommProposePlan        = "comm.propose_plan"
	MethodCommApprovePlan        = "comm.approve_plan"
	MethodCommRejectPlan         = "comm.reject_plan"
	MethodCommSeedGraph          = "comm.seed_graph"
	MethodCommExpandNode         = "comm.expand_node"
	MethodCommCompleteNode       = "comm.complete_node"
	MethodCommInjectGap          = "comm.inject_gap"
	MethodCommSpawn              = "comm.spawn"
	MethodCommListModels         = "comm.list_models"
	MethodCommStop               = "comm.stop"
	MethodCommAssignRole         = "comm.assign_role"
	MethodCommSummary            = "comm.summary"
	MethodCommStatus             = "comm.status"
	MethodCommReport             = "comm.report"
	MethodCommReadContext        = "comm.read_context"
	MethodCommResyncPlan         = "comm.resync_plan"
	MethodCommPlanStatus         = "comm.plan_status"
	MethodCommAssignTask         = "comm.assign_task"
	MethodCommAssignNext         = "comm.assign_next"
	MethodCommTaskControl        = "comm.task_control"
	MethodCommSubscribeChannel   = "comm.subscribe_channel"
	MethodCommUnsubscribeChannel = "comm.unsubscribe_channel"
	MethodCommAwaitMembers       = "comm.await_members"
)

// IsCommMethod reports whether a method constant is a swarm method.
// The internal/agent-server dispatcher uses this to route to swarm.Handle
// vs the legacy single-session path.
func IsCommMethod(method string) bool {
	switch method {
	case MethodCommShare, MethodCommRead,
		MethodCommMessage, MethodCommList,
		MethodCommListChannels, MethodCommChannelMembers,
		MethodCommProposePlan, MethodCommApprovePlan, MethodCommRejectPlan,
		MethodCommSeedGraph, MethodCommExpandNode,
		MethodCommCompleteNode, MethodCommInjectGap,
		MethodCommSpawn, MethodCommListModels,
		MethodCommStop, MethodCommAssignRole,
		MethodCommSummary, MethodCommStatus, MethodCommReport,
		MethodCommReadContext,
		MethodCommResyncPlan, MethodCommPlanStatus,
		MethodCommAssignTask, MethodCommAssignNext, MethodCommTaskControl,
		MethodCommSubscribeChannel, MethodCommUnsubscribeChannel,
		MethodCommAwaitMembers:
		return true
	}
	return false
}

// ServerEvent names emitted by the swarm dispatcher. The existing
// transport in internal/agent-protocol/json_rpc uses event-style
// messages for streaming (thought_chunk, tool, message). Swarm adds
// 25 more event names below. See ADR §2 for the wire shape and §6.3
// for how comm_*_response events relate to request ids.
const (
	// Swarm-wide state events.
	EventSwarmStatus         = "swarm_status"
	EventSwarmMemberUpdate   = "swarm_member_update"
	EventSwarmChannelMessage = "swarm_channel_message"

	// Direct-method responses (event-style, fired async by the
	// dispatcher when the operation completes or fails).
	EventCommSpawnResponse          = "comm.spawn_response"
	EventCommMessageResponse        = "comm.message_response"
	EventCommListResponse           = "comm.list_response"
	EventCommListChannelsResponse   = "comm.list_channels_response"
	EventCommChannelMembersResponse = "comm.channel_members_response"
	EventCommListModelsResponse     = "comm.list_models_response"
	EventCommSummaryResponse        = "comm.summary_response"
	EventCommStatusResponse         = "comm.status_response"
	EventCommReportResponse         = "comm.report_response"
	EventCommReadContextResponse    = "comm.read_context_response"
	EventCommAwaitMembersResponse   = "comm.await_members_response"
	EventCommResyncPlanResponse     = "comm.resync_plan_response"
	EventCommPlanStatusResponse     = "comm.plan_status_response"
	EventCommAssignTaskResponse     = "comm.assign_task_response"
	EventCommAssignNextResponse     = "comm.assign_next_response"
	EventCommTaskControlResponse    = "comm.task_control_response"
	EventCommShareResponse          = "comm.share_response"
	EventCommReadResponse           = "comm.read_response"
	EventCommError                  = "comm.error"

	// Plan-level events (S5+).
	EventSwarmPlan                 = "swarm_plan"
	EventSwarmPlanProposal         = "swarm_plan_proposal"
	EventSwarmPlanProposalResponse = "swarm_plan_proposal_response"
)

// IsSwarmEvent reports whether an incoming event is a swarm event.
// Used by the SDK to demultiplex comm.subscribe_channel streams into
// typed channels.
func IsSwarmEvent(event string) bool {
	switch event {
	case EventSwarmStatus, EventSwarmMemberUpdate, EventSwarmChannelMessage,
		EventCommSpawnResponse, EventCommMessageResponse,
		EventCommListResponse, EventCommListChannelsResponse,
		EventCommChannelMembersResponse, EventCommListModelsResponse,
		EventCommSummaryResponse, EventCommStatusResponse,
		EventCommReportResponse, EventCommReadContextResponse,
		EventCommAwaitMembersResponse,
		EventCommResyncPlanResponse, EventCommPlanStatusResponse,
		EventCommAssignTaskResponse, EventCommAssignNextResponse,
		EventCommTaskControlResponse,
		EventCommShareResponse, EventCommReadResponse,
		EventCommError,
		EventSwarmPlan, EventSwarmPlanProposal, EventSwarmPlanProposalResponse:
		return true
	}
	return false
}

// ErrorCode values returned in EventCommError.data.code (ADR §6.4).
const (
	ErrUnknownSession    = "swarm.unknown_session"
	ErrUnknownChannel    = "swarm.unknown_channel"
	ErrAlreadySubscribed = "swarm.already_subscribed"
	ErrNotSubscribed     = "swarm.not_subscribed"
	ErrCapacityExceeded  = "swarm.capacity_exceeded"
	ErrNotOwner          = "swarm.not_owner"
	ErrUnknownNode       = "swarm.unknown_node"
	ErrCycleDetected     = "swarm.cycle_detected"
	ErrUnknownDependency = "swarm.unknown_dependency"
	ErrArtifactInvalid   = "swarm.artifact_invalid"
	ErrDuplicateNonce    = "swarm.duplicate_nonce"
	ErrSessionGone       = "swarm.session_gone"
	ErrTooLarge          = "swarm.too_large"
	ErrInvalidMode       = "swarm.invalid_mode"
	ErrPlanMissing       = "swarm.plan_missing"
	ErrProposalMissing   = "swarm.proposal_missing"
	ErrNotCoordinator    = "swarm.not_coordinator"
	ErrTLDRMissing       = "swarm.tldr_missing"
	ErrInvalidRequest    = "swarm.invalid_request"
)
