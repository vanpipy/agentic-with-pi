package agentserver

import (
	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
)

type WireEmit struct {
	EventName string
	Payload   any
}

func MarshalAgentEventForWireForTest(ev agentcore.Event, parentID *string, streamBuf **stream.StreamBuffer) []WireEmit {
	t := &streamTranslator{
		parentID:  *parentID,
		streamBuf: *streamBuf,
	}
	internal := t.Translate(ev)
	*parentID = t.parentID
	*streamBuf = t.streamBuf
	return internal
}
