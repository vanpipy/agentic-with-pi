// Package swarm — channel index.
//
// Channels are per-swarm named pub/sub lanes. Membership is tracked
// in SwarmState.channels[swarmID][channelName].Subscribers. The
// channel itself stores no message history; Publish fans out via the
// Broadcaster and the per-conn write loop pushes one
// EventSwarmChannelMessage per subscriber.
//
// A channel is created lazily on first Subscribe (mirrors
// jcode crates/jcode-app-core/src/server/swarm_channels.rs). Empty
// channels are not reaped in S2 (ADR §9: deferred to S3).
package swarm

import (
	"encoding/json"
	"time"

	jsonrpc "github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	swarmproto "github.com/vanpiyp/awp/internal/agent-protocol/swarm"
)

// Subscribe adds sessionID to the channel's subscriber set. Creates
// the channel if it does not exist (the caller becomes the first
// subscriber).
//
// Returns ErrUnknownSwarm when swarmID is not the swarm of any
// currently registered member (channels belong to a swarm; subscribing
// to a non-existent swarm is rejected so a stray client cannot create
// orphan channels).
func (s *SwarmState) Subscribe(swarmID, channelName, sessionID string) error {
	if channelName == "" || sessionID == "" {
		return ErrUnknownChannel
	}
	if len(channelName) > swarmproto.MaxChannelNameBytes {
		return ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return ErrShuttingDown
	}
	if _, ok := s.members[sessionID]; !ok {
		return ErrUnknownMember
	}
	if !s.swarmExistsLocked(swarmID) {
		return ErrUnknownSwarm
	}

	ch, ok := s.channels[swarmID][channelName]
	if !ok {
		ch = newChannel(channelName, sessionID, s.now())
		if s.channels[swarmID] == nil {
			s.channels[swarmID] = make(map[string]*Channel)
		}
		s.channels[swarmID][channelName] = ch
		return nil
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if _, exists := ch.Subscribers[sessionID]; exists {
		return nil
	}
	ch.Subscribers[sessionID] = struct{}{}
	ch.LastActivity = s.now()
	return nil
}

// Unsubscribe removes sessionID from the channel. Idempotent:
// unsubscribing from a channel the session is not on is a no-op.
// Returns ErrUnknownChannel when the channel does not exist.
func (s *SwarmState) Unsubscribe(swarmID, channelName, sessionID string) error {
	if channelName == "" {
		return ErrUnknownChannel
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return ErrShuttingDown
	}
	ch, ok := s.channels[swarmID][channelName]
	if !ok {
		return ErrUnknownChannel
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	delete(ch.Subscribers, sessionID)
	ch.LastActivity = s.now()
	return nil
}

// Publish fans msg out to every subscriber of swarmID/channelName via
// the Broadcaster. The msg's MessageID is generated server-side (a
// uuidv7) so subscribers can deduplicate. Returns ErrUnknownChannel
// when the channel does not exist.
//
// Each subscriber receives a JSON-RPC event named
// EventSwarmChannelMessage whose data is a ChannelMessageEvent with
// msg's fields + the channel name.
func (s *SwarmState) Publish(swarmID, channelName string, msg swarmproto.ChannelMessage) error {
	if channelName == "" {
		return ErrUnknownChannel
	}
	if len(msg.Body) > swarmproto.MaxChannelMessageBodyBytes {
		return ErrTooLarge
	}
	if err := swarmproto.ValidateTLDR(msg.TLDR, msg.Body); err != nil {
		return ErrTooLarge
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return ErrShuttingDown
	}
	ch, ok := s.channels[swarmID][channelName]
	if !ok {
		return ErrUnknownChannel
	}

	if msg.MessageID == "" {
		msg.MessageID = newMessageID()
	}
	if msg.Timestamp == "" {
		msg.Timestamp = s.now().UTC().Format(time.RFC3339Nano)
	}
	msg.SwarmID = swarmID
	msg.Channel = channelName

	ch.mu.Lock()
	subs := make([]string, 0, len(ch.Subscribers))
	for sub := range ch.Subscribers {
		subs = append(subs, sub)
	}
	ch.LastActivity = s.now()
	ch.mu.Unlock()

	ev := jsonrpc.Response{
		JSONRPC: "2.0",
		Event:   swarmproto.EventSwarmChannelMessage,
		Data:    mustMarshalChannelEvent(msg),
	}
	s.broadcaster.Publish(ev)

	// Touch each subscriber's LastActivity so the idle-reap watcher
	// does not reap a session that is actively receiving channel
	// messages.
	for _, sub := range subs {
		if m, ok := s.members[sub]; ok {
			m.mu.Lock()
			m.LastActivity = s.now()
			m.Record.LastUpdatedAt = s.now().UTC().Format(time.RFC3339Nano)
			m.mu.Unlock()
		}
	}
	return nil
}

// ListChannels returns one ChannelInfo per channel in swarmID,
// sorted by channel name for deterministic output.
func (s *SwarmState) ListChannels(swarmID string) []swarmproto.ChannelInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	channels := s.channels[swarmID]
	out := make([]swarmproto.ChannelInfo, 0, len(channels))
	for name, ch := range channels {
		ch.mu.Lock()
		out = append(out, swarmproto.ChannelInfo{
			Channel:     name,
			SwarmID:     swarmID,
			MemberCount: len(ch.Subscribers),
			CreatedAt:   ch.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
		ch.mu.Unlock()
	}
	sortChannelInfos(out)
	return out
}

// ChannelMembers returns the sorted list of session ids subscribed
// to swarmID/channelName. Returns ErrUnknownChannel when the channel
// does not exist.
func (s *SwarmState) ChannelMembers(swarmID, channelName string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch, ok := s.channels[swarmID][channelName]
	if !ok {
		return nil, ErrUnknownChannel
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	out := make([]string, 0, len(ch.Subscribers))
	for sub := range ch.Subscribers {
		out = append(out, sub)
	}
	sortStrings(out)
	return out, nil
}

// swarmExistsLocked reports whether any registered member is in
// swarmID. Caller must hold s.mu.
func (s *SwarmState) swarmExistsLocked(swarmID string) bool {
	if swarmID == "" {
		return false
	}
	for _, m := range s.members {
		if m.Record.SwarmID == swarmID {
			return true
		}
	}
	return false
}

// RemoveSessionLocked removes sessionID from every channel it is
// subscribed to. Caller must hold s.mu. Used during Unregister / Stop
// so a departed session does not receive future Publish events.
func (s *SwarmState) RemoveSessionLocked(sessionID string) {
	for _, channels := range s.channels {
		for _, ch := range channels {
			ch.mu.Lock()
			delete(ch.Subscribers, sessionID)
			ch.mu.Unlock()
		}
	}
}

// mustMarshalChannelEvent is a small helper that re-marshals a
// ChannelMessage into the ChannelMessageEvent payload shape. The two
// types share all fields today; if they diverge in the future the
// dispatcher converts explicitly.
func mustMarshalChannelEvent(msg swarmproto.ChannelMessage) json.RawMessage {
	ev := swarmproto.ChannelMessageEvent{
		MessageID: msg.MessageID,
		SwarmID:   msg.SwarmID,
		Channel:   msg.Channel,
		FromID:    msg.FromID,
		FromName:  msg.FromName,
		Body:      msg.Body,
		TLDR:      msg.TLDR,
		Delivery:  msg.Delivery,
		Timestamp: msg.Timestamp,
		InReplyTo: msg.InReplyTo,
	}
	return marshalAny(ev)
}
