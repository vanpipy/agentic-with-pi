package swarm

// ChannelInfo describes one channel in a swarm. Returned in arrays
// by comm.list_channels and as the inner members map by
// comm.channel_members.
type ChannelInfo struct {
	Channel     string `json:"channel"`
	SwarmID     string `json:"swarm_id"`
	MemberCount int    `json:"member_count"`
	CreatedAt   string `json:"created_at,omitempty"` // RFC3339Nano
}

// ChannelMessage is the body of a single comm.message call. The
// MessageID is a uuidv7 assigned server-side; it round-trips in
// EventSwarmChannelMessage so subscribers can deduplicate.
type ChannelMessage struct {
	MessageID string       `json:"message_id"`
	SwarmID   string       `json:"swarm_id"`
	Channel   string       `json:"channel"`
	FromID    string       `json:"from_session_id"`
	FromName  string       `json:"from_name,omitempty"`
	Body      string       `json:"body"`
	TLDR      string       `json:"tldr,omitempty"`
	Delivery  DeliveryMode `json:"delivery,omitempty"`
	Timestamp string       `json:"timestamp"`             // RFC3339Nano
	InReplyTo string       `json:"in_reply_to,omitempty"` // MessageID being replied to
}

// ChannelMessageEvent is the data payload for EventSwarmChannelMessage.
// One event is delivered per subscriber (server fans out at publish
// time, not at receive time).
type ChannelMessageEvent struct {
	MessageID string       `json:"message_id"`
	SwarmID   string       `json:"swarm_id"`
	Channel   string       `json:"channel"`
	FromID    string       `json:"from_session_id"`
	FromName  string       `json:"from_name,omitempty"`
	Body      string       `json:"body"`
	TLDR      string       `json:"tldr,omitempty"`
	Delivery  DeliveryMode `json:"delivery,omitempty"`
	Timestamp string       `json:"timestamp"` // RFC3339Nano
	InReplyTo string       `json:"in_reply_to,omitempty"`
}
