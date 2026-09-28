package agentserver

import (
	agentcore "github.com/vanpiyp/awp/internal/agent-core"
)

type WireEmit struct {
	EventName string
	Payload   any
}

func MarshalAgentEventForWireForTest(ev agentcore.Event, parentID *string, streamBuf **agentcore.StreamBuffer) []WireEmit {
	t := &streamTranslator{
		parentID:  *parentID,
		streamBuf: *streamBuf,
	}
	internal := t.Translate(ev)
	*parentID = t.parentID
	*streamBuf = t.streamBuf
	return internal
}
