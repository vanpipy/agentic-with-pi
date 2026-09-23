package agentcore

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

type StreamBuffer struct {
	mu       sync.Mutex
	msg      json_rpc.MessageEvent
	parentID string
}

func NewStreamBuffer(parentID string) *StreamBuffer {
	return &StreamBuffer{parentID: parentID}
}

func (b *StreamBuffer) AppendThinking(text, signature string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if text == "" && signature == "" {
		return
	}
	if n := len(b.msg.Message.Content); n > 0 && b.msg.Message.Content[n-1].Type == "thinking" {
		last := &b.msg.Message.Content[n-1]
		last.Thinking += text
		if signature != "" {
			last.ThinkingSignature = signature
		}
		return
	}
	b.msg.Message.Content = append(b.msg.Message.Content, json_rpc.MessageContentPart{
		Type:              "thinking",
		Thinking:          text,
		ThinkingSignature: signature,
	})
}

func (b *StreamBuffer) AppendText(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if text == "" {
		return
	}
	if n := len(b.msg.Message.Content); n > 0 && b.msg.Message.Content[n-1].Type == "text" {
		last := &b.msg.Message.Content[n-1]
		last.Text += text
		return
	}
	b.msg.Message.Content = append(b.msg.Message.Content, json_rpc.MessageContentPart{
		Type: "text",
		Text: text,
	})
}

func (b *StreamBuffer) AppendToolCall(id, name, intent string, args json.RawMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msg.Message.Content = append(b.msg.Message.Content, json_rpc.MessageContentPart{
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
	b.msg.Usage = &usage
}

func (b *StreamBuffer) SetStopReason(reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msg.StopReason = reason
}

func (b *StreamBuffer) SetDetails(d json_rpc.MessageDetails) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msg.Details = &d
}

func (b *StreamBuffer) SetRole(role string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.msg.Message.Role = role
}

func (b *StreamBuffer) ParentID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.parentID
}

func (b *StreamBuffer) Finalize() json_rpc.MessageEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.msg.ID == "" {
		b.msg.ID = json_rpc.NewV7()
	}
	if b.msg.ParentID == "" && b.parentID != "" {
		b.msg.ParentID = b.parentID
	}
	if b.msg.Timestamp == "" {
		b.msg.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return b.msg
}
