// Package swarm: message routing.
//
// SendMessage routes a comm.message call to either a single
// recipient (toSessionID set) or a channel (channelName set). Direct
// delivery pushes the message into the recipient's per-member Sink
// (non-blocking). Channel delivery goes through Publish so the
// channel fan-out semantics + Subscriber bookkeeping apply.
//
// Direct vs channel is exclusive: the caller (dispatcher) sets at
// most one of toSessionID / channelName.
package swarm

import (
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// MessageOptions is the params payload for comm.message (route only;
// the body / tldr / delivery are passed via SendMessage for clarity).
type MessageOptions struct {
	// FromSessionID is the sender. Must be a registered member of
	// swarmID; otherwise SendMessage returns ErrUnknownMember.
	FromSessionID string

	// SwarmID scopes the channel name lookup. Required even for
	// direct delivery so the dispatcher can locate the recipient
	// without a global session-id index.
	SwarmID string

	// ToSessionID, when non-empty, routes to a single member.
	ToSessionID string

	// Channel, when non-empty, routes to a channel.
	Channel string

	// Body is the message body. Capped at MaxChannelMessageBodyBytes
	// (same cap as channel delivery; mirrors jcode).
	Body string

	// TLDR is required when body > TLDRRequiredOverChars. Capped at
	// MaxTLDRChars.
	TLDR string

	// Delivery selects the recipient experience (notify/interrupt/wake).
	// Empty -> notify (default).
	Delivery swarmproto.DeliveryMode

	// InReplyTo is an optional MessageID the message replies to.
	// Forwarded verbatim in the wire payload.
	InReplyTo string
}

// SendMessage delivers a single message via the appropriate route.
//
// Returns:
//   - ErrUnknownMember when FromSessionID is not registered or
//     ToSessionID is set but not registered.
//   - ErrUnknownChannel when Channel is set but the channel does not
//     exist on the addressed swarm.
//   - ErrInvalidRequest when neither ToSessionID nor Channel is set,
//     or both are set.
//   - ErrTooLarge when body exceeds MaxChannelMessageBodyBytes or
//     tldr is required and missing / oversized.
//
// On success, returns the MessageID assigned to the message. The
// caller (dispatcher) echoes this back as the EventCommMessageResponse
// payload.
func (s *SwarmState) SendMessage(opts MessageOptions) (string, error) {
	if opts.FromSessionID == "" {
		return "", ErrUnknownMember
	}
	if (opts.ToSessionID == "" && opts.Channel == "") ||
		(opts.ToSessionID != "" && opts.Channel != "") {
		return "", ErrInvalidRequest
	}
	if len(opts.Body) > swarmproto.MaxChannelMessageBodyBytes {
		return "", ErrTooLarge
	}
	if err := swarmproto.ValidateTLDR(opts.TLDR, opts.Body); err != nil {
		return "", ErrTooLarge
	}

	s.mu.Lock()
	if s.shuttingDown {
		s.mu.Unlock()
		return "", ErrShuttingDown
	}
	from, ok := s.members[opts.FromSessionID]
	if !ok {
		s.mu.Unlock()
		return "", ErrUnknownMember
	}
	delivery := opts.Delivery
	if delivery.IsOther() {
		delivery = swarmproto.DeliveryNotify
	}
	nowStr := s.now().UTC().Format(time.RFC3339Nano)

	if opts.Channel != "" {
		ch, ok := s.channels[opts.SwarmID][opts.Channel]
		if !ok {
			s.mu.Unlock()
			return "", ErrUnknownChannel
		}
		msg := swarmproto.ChannelMessage{
			MessageID: newMessageID(),
			SwarmID:   opts.SwarmID,
			Channel:   opts.Channel,
			FromID:    opts.FromSessionID,
			FromName:  from.Record.FriendlyName,
			Body:      opts.Body,
			TLDR:      opts.TLDR,
			Delivery:  delivery,
			Timestamp: nowStr,
			InReplyTo: opts.InReplyTo,
		}
		ch.mu.Lock()
		subs := make([]string, 0, len(ch.Subscribers))
		for sub := range ch.Subscribers {
			subs = append(subs, sub)
		}
		ch.LastActivity = s.now()
		ch.mu.Unlock()

		data, err := marshalChannelEvent(msg)
		if err != nil {
			s.mu.Unlock()
			return "", ErrInvalidRequest
		}
		ev := jsonrpc.Response{
			JSONRPC: "2.0",
			Event:   swarmproto.EventSwarmChannelMessage,
			Data:    data,
		}
		s.broadcaster.Publish(ev)
		for _, sub := range subs {
			if m, ok := s.members[sub]; ok {
				m.mu.Lock()
				m.LastActivity = s.now()
				m.Record.LastUpdatedAt = nowStr
				m.mu.Unlock()
			}
		}
		from.mu.Lock()
		from.LastActivity = s.now()
		from.Record.LastUpdatedAt = nowStr
		from.mu.Unlock()
		s.mu.Unlock()
		return msg.MessageID, nil
	}

	// Direct delivery.
	to, ok := s.members[opts.ToSessionID]
	if !ok {
		s.mu.Unlock()
		return "", ErrUnknownMember
	}
	msg := swarmproto.ChannelMessage{
		MessageID: newMessageID(),
		SwarmID:   opts.SwarmID,
		Channel:   "",
		FromID:    opts.FromSessionID,
		FromName:  from.Record.FriendlyName,
		Body:      opts.Body,
		TLDR:      opts.TLDR,
		Delivery:  delivery,
		Timestamp: nowStr,
		InReplyTo: opts.InReplyTo,
	}
	data, err := marshalChannelEvent(msg)
	if err != nil {
		s.mu.Unlock()
		return "", ErrInvalidRequest
	}
	ev := jsonrpc.Response{
		JSONRPC: "2.0",
		Event:   swarmproto.EventCommMessageResponse,
		Data:    data,
	}
	to.mu.Lock()
	// Try to deliver to the recipient's per-member sink (non-blocking).
	select {
	case to.Sink <- ev:
	default:
		// Sink full. Drop. The Broadcaster will catch up when the
		// subscriber reconnects.
	}
	to.LastActivity = s.now()
	to.Record.LastUpdatedAt = nowStr
	to.mu.Unlock()
	from.mu.Lock()
	from.LastActivity = s.now()
	from.Record.LastUpdatedAt = nowStr
	from.mu.Unlock()
	s.mu.Unlock()
	return msg.MessageID, nil
}
