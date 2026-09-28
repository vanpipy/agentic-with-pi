package stream

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

type StreamBuffer struct {
	mu         sync.Mutex
	parts      []json_rpc.MessageContentPart
	role       string
	usage      *json_rpc.UsageStats
	stopReason string
	details    *json_rpc.MessageDetails
	parentID   string

	assignedID        string
	assignedParentID  string
	assignedTimestamp string
}

func NewStreamBuffer(parentID string) *StreamBuffer {
	return &StreamBuffer{parentID: parentID}
}

func (b *StreamBuffer) coalesceOrAppend(partType string, mutator func(part *json_rpc.MessageContentPart)) {
	if n := len(b.parts); n > 0 && b.parts[n-1].Type == partType {
		mutator(&b.parts[n-1])
		return
	}
	b.parts = append(b.parts, json_rpc.MessageContentPart{Type: partType})
	mutator(&b.parts[len(b.parts)-1])
}

func (b *StreamBuffer) AppendThinking(text, signature string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if text == "" && signature == "" {
		return
	}
	b.coalesceOrAppend("thinking", func(p *json_rpc.MessageContentPart) {
		p.Thinking += text
		if signature != "" {
			p.ThinkingSignature = signature
		}
	})
}

func (b *StreamBuffer) AppendText(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if text == "" {
		return
	}
	b.coalesceOrAppend("text", func(p *json_rpc.MessageContentPart) {
		p.Text += text
	})
}

func (b *StreamBuffer) AppendToolCall(id, name, intent string, args json.RawMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.parts = append(b.parts, json_rpc.MessageContentPart{
		Type:      "toolCall",
		ID:        id,
		Name:      name,
		Intent:    intent,
		Arguments: args,
	})
}

func (b *StreamBuffer) SetUsage(usage json_rpc.UsageStats) {
	b.mu.Lock()
	defer b.mu.Unlock()
	u := usage
	b.usage = &u
}

func (b *StreamBuffer) SetStopReason(reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopReason = reason
}

func (b *StreamBuffer) SetDetails(d json_rpc.MessageDetails) {
	b.mu.Lock()
	defer b.mu.Unlock()
	det := d
	b.details = &det
}

func (b *StreamBuffer) SetRole(role string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.role = role
}

func (b *StreamBuffer) ParentID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.parentID
}

func (b *StreamBuffer) Finalize() json_rpc.MessageEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.assignedID == "" {
		b.assignedID = json_rpc.NewV7()
	}
	if b.assignedParentID == "" && b.parentID != "" {
		b.assignedParentID = b.parentID
	}
	if b.assignedTimestamp == "" {
		b.assignedTimestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	content := make([]json_rpc.MessageContentPart, len(b.parts))
	copy(content, b.parts)
	return json_rpc.MessageEvent{
		ID:         b.assignedID,
		ParentID:   b.assignedParentID,
		Timestamp:  b.assignedTimestamp,
		Message:    json_rpc.Message{Role: b.role, Content: content},
		StopReason: b.stopReason,
		Usage:      b.usage,
		Details:    b.details,
	}
}
