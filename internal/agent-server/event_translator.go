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
}

// StreamTranslator is the exported alias of streamTranslator so external
// test packages can hold a typed reference. The two names denote the
// same type; production code may use either.
type StreamTranslator = streamTranslator

func newStreamTranslator() *streamTranslator {
	return &streamTranslator{}
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
	return nil
}

func (t *streamTranslator) onThoughtChunk(ev agentcore.Event) []WireEmit {
	if t.streamBuf != nil {
		t.streamBuf.AppendThinking(ev.Reasoning, "")
	}
	return nil
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
	return []WireEmit{{EventName: json_rpc.EventMessage, Payload: msg}}
}

func (t *streamTranslator) onFinalAnswer(ev agentcore.Event) []WireEmit {
	if t.streamBuf == nil {
		return nil
	}
	t.streamBuf.SetRole("assistant")
	t.streamBuf.AppendText(ev.Content)
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
