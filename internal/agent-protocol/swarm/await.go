package swarm

import "encoding/json"

// DeliveryMode controls how a comm.message recipient receives the
// payload. Mirrors jcode's CommDeliveryMode.
type DeliveryMode string

const (
	// DeliveryNotify pushes the event into the recipient's
	// EventSwarmChannelMessage stream but does not interrupt the
	// current turn. Cheapest, default for non-urgent messages.
	DeliveryNotify DeliveryMode = "notify"

	// DeliveryInterrupt enqueues the message at the head of the
	// recipient's pending queue; the recipient's current turn is
	// not aborted, but the next turn starts with the message
	// prepended to the prompt.
	DeliveryInterrupt DeliveryMode = "interrupt"

	// DeliveryWake soft-interrupts the recipient if it is idle
	// (no running turn). Equivalent to interrupt + soft-wake.
	// Default for high-priority messages.
	DeliveryWake DeliveryMode = "wake"

	DeliveryOther DeliveryMode = ""
)

// ParseDeliveryMode maps a wire string. Unknown -> DeliveryOther.
func ParseDeliveryMode(s string) DeliveryMode {
	switch DeliveryMode(s) {
	case DeliveryNotify, DeliveryInterrupt, DeliveryWake:
		return DeliveryMode(s)
	}
	return DeliveryOther
}

func (d DeliveryMode) String() string { return string(d) }
func (d DeliveryMode) IsOther() bool  { return d == DeliveryOther }

func (d DeliveryMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(d))
}

func (d *DeliveryMode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*d = ParseDeliveryMode(s)
	return nil
}

// AwaitMode controls how comm.await_members evaluates target_status:
// "all" requires every target to reach a target status (default),
// "any" returns as soon as the first target does.
type AwaitMode string

const (
	AwaitModeAll   AwaitMode = "all"
	AwaitModeAny   AwaitMode = "any"
	AwaitModeOther AwaitMode = ""
)

// ParseAwaitMode maps a wire string. Unknown -> AwaitModeOther.
func ParseAwaitMode(s string) AwaitMode {
	switch AwaitMode(s) {
	case AwaitModeAll, AwaitModeAny:
		return AwaitMode(s)
	}
	return AwaitModeOther
}

func (m AwaitMode) String() string { return string(m) }
func (m AwaitMode) IsAll() bool    { return m == AwaitModeAll }
func (m AwaitMode) IsAny() bool    { return m == AwaitModeAny }
func (m AwaitMode) IsOther() bool  { return m == AwaitModeOther }

func (m AwaitMode) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(m))
}

func (m *AwaitMode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*m = ParseAwaitMode(s)
	return nil
}

// AwaitOptions is the params payload for MethodCommAwaitMembers.
// Background + Wake default to true to match jcode's default behavior
// (per ADR §15 D3).
type AwaitOptions struct {
	SwarmID       string            `json:"swarm_id"`
	FromSessionID string            `json:"from_session"`
	TargetStatus  []LifecycleStatus `json:"target_status"`
	SessionIDs    []string          `json:"session_ids,omitempty"`  // empty = all non-self
	Mode          AwaitMode         `json:"mode,omitempty"`         // default all
	TimeoutSecs   uint64            `json:"timeout_secs,omitempty"` // default AwaitDefaultTimeoutSeconds
	Background    *bool             `json:"background,omitempty"`   // default true
	Notify        *bool             `json:"notify,omitempty"`       // default true (deliver response event)
	Wake          *bool             `json:"wake,omitempty"`         // default true (soft-interrupt requester on match)
	RequestNonce  string            `json:"request_nonce,omitempty"`
}

// Background returns the effective background flag (default true).
func (o AwaitOptions) EffectiveBackground() bool {
	if o.Background == nil {
		return true
	}
	return *o.Background
}

// Notify returns the effective notify flag (default true).
func (o AwaitOptions) EffectiveNotify() bool {
	if o.Notify == nil {
		return true
	}
	return *o.Notify
}

// Wake returns the effective wake flag (default true).
func (o AwaitOptions) EffectiveWake() bool {
	if o.Wake == nil {
		return true
	}
	return *o.Wake
}

// AwaitedMemberStatus is the per-member result row returned in
// EventCommAwaitMembersResponse.data.members. Done is true when the
// member reached one of the caller's target_status values (or has
// been observed in a terminal-equivalent state).
type AwaitedMemberStatus struct {
	SessionID        string          `json:"session_id"`
	FriendlyName     string          `json:"friendly_name,omitempty"`
	Status           LifecycleStatus `json:"status"`
	Done             bool            `json:"done"`
	CompletionReport string          `json:"completion_report,omitempty"`
}

// AwaitResult is the data payload for EventCommAwaitMembersResponse.
type AwaitResult struct {
	Completed         bool                  `json:"completed"`
	TimedOut          bool                  `json:"timed_out"`
	Members           []AwaitedMemberStatus `json:"members"`
	Summary           string                `json:"summary,omitempty"`
	BackgroundStarted bool                  `json:"background_started,omitempty"`
}
