package agentserver

import (
	"encoding/json"
	"fmt"
	"time"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-core/stream"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

type streamTranslator struct {
	parentID  string
	streamBuf *stream.StreamBuffer
	// Stable id for the interim (streaming) Message event produced by
	// onThoughtChunk. Regenerated whenever the underlying buffer is reset so
	// that downstream consumers can recognise per-step streams.
	interimID string
}

// StreamTranslator is the exported alias of streamTranslator so external
// test packages can hold a typed reference. The two names denote the
// same type; production code may use either.
type StreamTranslator = streamTranslator

func newStreamTranslator() *streamTranslator {
	return &streamTranslator{interimID: json_rpc.NewV7()}
}

// NewStreamTranslator is the exported constructor. It exists for
// external test packages that need direct access to the translator.
func NewStreamTranslator() *streamTranslator {
	return newStreamTranslator()
}

func (t *streamTranslator) ParentIDForTest() string {
	return t.parentID
}

func (t *streamTranslator) StreamBufForTest() *stream.StreamBuffer {
	return t.streamBuf
}

func (t *streamTranslator) InterimIDForTest() string {
	return t.interimID
}

func (t *streamTranslator) resetInterimID() {
	t.interimID = json_rpc.NewV7()
}

func (t *streamTranslator) Translate(ev agentcore.Event) []WireEmit {
	switch ev.Category {
	case agentcore.EventUserMessage:
		return t.onUserMessage(ev)
	case agentcore.EventThoughtStart:
		return t.onThoughtStart(ev)
	case agentcore.EventThoughtChunk:
		return t.onThoughtChunk(ev)
	case agentcore.EventThoughtEnd:
		return t.onThoughtEnd(ev)
	case agentcore.EventTool:
		return t.onTool(ev)
	case agentcore.EventObserve:
		return t.onObserve(ev)
	case agentcore.EventFinalAnswer:
		return t.onFinalAnswer(ev)
	case agentcore.EventError:
		return t.onError(ev)
	case agentcore.EventCompaction:
		return t.onCompaction(ev)
	case agentcore.EventInvalid:
		return nil
	}
	return nil
}

func (t *streamTranslator) onUserMessage(ev agentcore.Event) []WireEmit {
	buf := stream.NewStreamBuffer(t.parentID)
	buf.SetRole("user")
	buf.AppendText(ev.Content)
	buf.SetStopReason("end_turn")
	msg := buf.Finalize()
	t.parentID = msg.ID
	return []WireEmit{{EventName: json_rpc.EventMessage, Payload: msg}}
}

func (t *streamTranslator) onThoughtStart(ev agentcore.Event) []WireEmit {
	t.streamBuf = stream.NewStreamBuffer(t.parentID)
	t.resetInterimID()
	return nil
}

// interimStopReason is the sentinel stop reason emitted on interim Message
// events during streaming. It is neither "end_turn" nor empty so the TUI's
// handleMessageEvent will not commit the streaming buffers prematurely.
const interimStopReason = "streaming"

func (t *streamTranslator) onThoughtChunk(ev agentcore.Event) []WireEmit {
	if t.streamBuf == nil {
		return nil
	}
	var parts []json_rpc.MessageContentPart
	if ev.Reasoning != "" {
		t.streamBuf.AppendThinking(ev.Reasoning, "")
		parts = append(parts, json_rpc.MessageContentPart{Type: "thinking", Thinking: ev.Reasoning})
	}
	if ev.Content != "" {
		t.streamBuf.AppendText(ev.Content)
		parts = append(parts, json_rpc.MessageContentPart{Type: "text", Text: ev.Content})
	}
	if len(parts) == 0 {
		return nil
	}
	return []WireEmit{{EventName: json_rpc.EventMessage, Payload: json_rpc.MessageEvent{
		ID:         t.interimID,
		ParentID:   t.parentID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Message:    json_rpc.Message{Role: "assistant", Content: parts},
		StopReason: interimStopReason,
	}}}
}

func (t *streamTranslator) onThoughtEnd(ev agentcore.Event) []WireEmit {
	if t.streamBuf != nil && ev.Usage != nil {
		t.streamBuf.SetUsage(json_rpc.UsageStats{
			PromptTokens:     ev.Usage.PromptTokens,
			CompletionTokens: ev.Usage.CompletionTokens,
			TotalTokens:      ev.Usage.TotalTokens,
		})
	}
	return nil
}

func (t *streamTranslator) onTool(ev agentcore.Event) []WireEmit {
	if t.streamBuf == nil {
		return nil
	}
	toolCallID := json_rpc.NewV7()
	intent := ev.ToolIntent
	if intent == "" {
		intent = extractIntent(ev.ToolArgs)
	}
	args := json.RawMessage(ev.ToolArgs)
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	t.streamBuf.SetRole("assistant")
	t.streamBuf.AppendToolCall(toolCallID, ev.ToolName, intent, args)
	t.streamBuf.SetStopReason("toolUse")
	assistantMsg := t.streamBuf.Finalize()
	t.parentID = assistantMsg.ID
	emits := []WireEmit{{EventName: json_rpc.EventMessage, Payload: assistantMsg}}
	toolResultBuf := stream.NewStreamBuffer(toolCallID)
	toolResultBuf.SetRole("toolResult")
	if ev.ToolError != "" {
		toolResultBuf.AppendText(ev.ToolError)
	} else if ev.ToolResult != "" {
		toolResultBuf.AppendText(ev.ToolResult)
	}
	toolResultBuf.SetDetails(json_rpc.MessageDetails{
		ToolName: ev.ToolName,
		Intent:   ev.ToolIntent,
		Error:    ev.ToolError,
	})
	toolResultBuf.SetStopReason("toolUse")
	toolResultMsg := toolResultBuf.Finalize()
	t.parentID = toolResultMsg.ID
	emits = append(emits, WireEmit{EventName: json_rpc.EventMessage, Payload: toolResultMsg})
	t.streamBuf = stream.NewStreamBuffer(toolResultMsg.ID)
	t.resetInterimID()
	return emits
}

func (t *streamTranslator) onObserve(ev agentcore.Event) []WireEmit {
	if t.streamBuf == nil {
		return nil
	}
	t.streamBuf.SetStopReason("toolUse")
	msg := t.streamBuf.Finalize()
	t.parentID = msg.ID
	t.streamBuf = stream.NewStreamBuffer(msg.ID)
	t.resetInterimID()
	return []WireEmit{{EventName: json_rpc.EventMessage, Payload: msg}}
}

func (t *streamTranslator) onFinalAnswer(ev agentcore.Event) []WireEmit {
	if t.streamBuf == nil {
		return nil
	}
	t.streamBuf.SetRole("assistant")
	if ev.Usage != nil {
		t.streamBuf.SetUsage(json_rpc.UsageStats{
			PromptTokens:     ev.Usage.PromptTokens,
			CompletionTokens: ev.Usage.CompletionTokens,
			TotalTokens:      ev.Usage.TotalTokens,
		})
	}
	t.streamBuf.SetStopReason("end_turn")
	msg := t.streamBuf.Finalize()
	t.parentID = msg.ID
	t.streamBuf = nil
	return []WireEmit{{EventName: json_rpc.EventMessage, Payload: msg}}
}

func (t *streamTranslator) onError(ev agentcore.Event) []WireEmit {
	if t.streamBuf == nil {
		return nil
	}
	t.streamBuf.AppendText(ev.ToolError)
	t.streamBuf.SetStopReason("end_turn")
	msg := t.streamBuf.Finalize()
	t.parentID = msg.ID
	t.streamBuf = nil
	customEv := json_rpc.CustomEvent{
		ID:         json_rpc.NewV7(),
		ParentID:   t.parentID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CustomType: "tool_error",
		Data:       json.RawMessage(fmt.Sprintf(`{"error":%q}`, ev.ToolError)),
	}
	return []WireEmit{
		{EventName: json_rpc.EventMessage, Payload: msg},
		{EventName: json_rpc.EventCustom, Payload: customEv},
	}
}

func (t *streamTranslator) onCompaction(ev agentcore.Event) []WireEmit {
	customEv := json_rpc.CustomEvent{
		ID:         json_rpc.NewV7(),
		ParentID:   t.parentID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		CustomType: "compaction",
		Data: json.RawMessage(fmt.Sprintf(
			`{"summary":%q,"tokens_before":%d,"tokens_after":%d,"first_kept_seq":%d,"model":%q}`,
			ev.Summary,
			ev.TokensBefore,
			ev.TokensAfter,
			ev.FirstKeptSeq,
			ev.CompactionModel,
		)),
	}
	return []WireEmit{{EventName: json_rpc.EventCustom, Payload: customEv}}
}
