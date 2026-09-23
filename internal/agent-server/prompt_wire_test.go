package agentserver

import (
	"encoding/json"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

func TestMarshalAgentEventForWireUserMessageEmitsUserRoleMessage(t *testing.T) {
	var parentID string
	var buf *agentcore.StreamBuffer
	emits := marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventUserMessage,
		Content:  "what time is it",
	}, &parentID, &buf)
	if len(emits) != 1 {
		t.Fatalf("expected 1 emit, got %d", len(emits))
	}
	if emits[0].eventName != json_rpc.EventMessage {
		t.Errorf("expected event %q, got %q", json_rpc.EventMessage, emits[0].eventName)
	}
	msg, ok := emits[0].payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("expected MessageEvent, got %T", emits[0].payload)
	}
	if msg.Message.Role != "user" {
		t.Errorf("role=%q, want user", msg.Message.Role)
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("stopReason=%q", msg.StopReason)
	}
	if parentID != msg.ID {
		t.Errorf("parentID not updated")
	}
	if msg.Message.Content[0].Text != "what time is it" {
		t.Errorf("content[0].text=%q", msg.Message.Content[0].Text)
	}
}

func TestMarshalAgentEventForWireThoughtChunkAccumulatesNoEmit(t *testing.T) {
	var parentID string
	var buf *agentcore.StreamBuffer = agentcore.NewStreamBuffer("")
	emits := marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventThoughtChunk,
		Reasoning: "hello ",
	}, &parentID, &buf)
	if len(emits) != 0 {
		t.Errorf("expected 0 emits during chunk, got %d", len(emits))
	}
	emits = marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventThoughtChunk,
		Reasoning: "world",
	}, &parentID, &buf)
	if len(emits) != 0 {
		t.Errorf("expected 0 emits during chunk, got %d", len(emits))
	}
}

func TestMarshalAgentEventForWireFinalAnswerEmitsAssistantMessage(t *testing.T) {
	var parentID string
	var buf *agentcore.StreamBuffer = agentcore.NewStreamBuffer(parentID)
	marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventThoughtChunk,
		Reasoning: "I should answer",
	}, &parentID, &buf)
	emits := marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "It's 2 PM",
	}, &parentID, &buf)
	if len(emits) != 1 {
		t.Fatalf("expected 1 emit, got %d", len(emits))
	}
	msg, ok := emits[0].payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("expected MessageEvent, got %T", emits[0].payload)
	}
	if msg.Message.Role != "assistant" {
		t.Errorf("role=%q, want assistant", msg.Message.Role)
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("stopReason=%q", msg.StopReason)
	}
	if len(msg.Message.Content) != 2 {
		t.Fatalf("expected 2 parts (thinking+text), got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0].Type != "thinking" {
		t.Errorf("part 0 type=%q, want thinking", msg.Message.Content[0].Type)
	}
	if msg.Message.Content[1].Type != "text" {
		t.Errorf("part 1 type=%q, want text", msg.Message.Content[1].Type)
	}
	if msg.Message.Content[1].Text != "It's 2 PM" {
		t.Errorf("part 1 text=%q", msg.Message.Content[1].Text)
	}
}

func TestMarshalAgentEventForWireToolEmitsMessagePair(t *testing.T) {
	var parentID string
	var buf *agentcore.StreamBuffer = agentcore.NewStreamBuffer(parentID)
	emits := marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "bash",
		ToolArgs: `{"command":"date","intent":"Get time"}`,
		ToolIntent: "Get time",
	}, &parentID, &buf)
	if len(emits) != 2 {
		t.Fatalf("expected 2 emits (assistant+toolResult), got %d", len(emits))
	}
	assistant, ok := emits[0].payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("emit 0 expected MessageEvent, got %T", emits[0].payload)
	}
	if assistant.Message.Role != "assistant" {
		t.Errorf("assistant role=%q", assistant.Message.Role)
	}
	if assistant.StopReason != "toolUse" {
		t.Errorf("assistant stopReason=%q", assistant.StopReason)
	}
	toolResult, ok := emits[1].payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("emit 1 expected MessageEvent, got %T", emits[1].payload)
	}
	if toolResult.Message.Role != "toolResult" {
		t.Errorf("toolResult role=%q", toolResult.Message.Role)
	}
	if toolResult.Details == nil || toolResult.Details.ToolName != "bash" {
		t.Errorf("toolResult details missing")
	}
	if toolResult.ParentID == "" {
		t.Errorf("toolResult should have parent ID (tool call id)")
	}
}

func TestMarshalAgentEventForWireErrorEmitsMessageAndCustom(t *testing.T) {
	var parentID string
	var buf *agentcore.StreamBuffer = agentcore.NewStreamBuffer(parentID)
	emits := marshalAgentEventForWire(agentcore.Event{
		Category: agentcore.EventError,
		ToolError: "boom",
	}, &parentID, &buf)
	if len(emits) != 2 {
		t.Fatalf("expected 2 emits (message + custom), got %d", len(emits))
	}
	if emits[0].eventName != json_rpc.EventMessage {
		t.Errorf("emit 0 event=%q", emits[0].eventName)
	}
	if emits[1].eventName != json_rpc.EventCustom {
		t.Errorf("emit 1 event=%q", emits[1].eventName)
	}
	custom, ok := emits[1].payload.(json_rpc.CustomEvent)
	if !ok {
		t.Fatalf("emit 1 expected CustomEvent, got %T", emits[1].payload)
	}
	if custom.CustomType != "tool_error" {
		t.Errorf("customType=%q", custom.CustomType)
	}
	var data map[string]string
	if err := json.Unmarshal(custom.Data, &data); err != nil {
		t.Fatalf("custom.Data not valid JSON: %v", err)
	}
	if data["error"] != "boom" {
		t.Errorf("data.error=%q", data["error"])
	}
}