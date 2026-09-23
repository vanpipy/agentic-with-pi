package tui

import (
	"testing"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
)

func newTestChatModel() *chatModel {
	return newChatModel()
}

func TestHandleServerEventMessageWithThinkingAndTextAndToolCall(t *testing.T) {
	c := newTestChatModel()
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000001",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"message":{
			"role":"assistant",
			"content":[
				{"type":"thinking","thinking":"I should answer"},
				{"type":"text","text":"Hi there!"},
				{"type":"toolCall","id":"call-abc","name":"bash","intent":"Get time","arguments":{"command":"date"}}
			]
		},
		"stopReason":"toolUse"
	}`)
	handleServerEvent(c, new(string), agentclient.Event{Kind: "message", Data: data})
	if len(c.messages) != 1 {
		t.Fatalf("expected 1 message (tool call), got %d", len(c.messages))
	}
	if c.messages[0].role != roleTool {
		t.Errorf("expected roleTool, got %v", c.messages[0].role)
	}
}

func TestHandleServerEventToolResultAppearsAsObserve(t *testing.T) {
	c := newTestChatModel()
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000002",
		"parentId":"call-abc",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"message":{
			"role":"toolResult",
			"content":[{"type":"text","text":"Wed Sep 23 14:00"}]
		},
		"details":{"toolName":"bash","intent":"Get time"},
		"stopReason":"toolUse"
	}`)
	handleServerEvent(c, new(string), agentclient.Event{Kind: "message", Data: data})
	last := c.messages[len(c.messages)-1]
	if last.role != roleObserve {
		t.Errorf("expected roleObserve, got %v", last.role)
	}
}

func TestHandleServerEventToolResultErrorAppearsAsError(t *testing.T) {
	c := newTestChatModel()
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000003",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"message":{
			"role":"toolResult",
			"content":[{"type":"text","text":"boom"}]
		},
		"details":{"toolName":"bash","error":"boom"}
	}`)
	handleServerEvent(c, new(string), agentclient.Event{Kind: "message", Data: data})
	last := c.messages[len(c.messages)-1]
	if last.role != roleError {
		t.Errorf("expected roleError, got %v", last.role)
	}
}

func TestHandleServerEventCustomToolErrorAppendsError(t *testing.T) {
	c := newTestChatModel()
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000004",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"tool_error",
		"data":{"error":"boom"}
	}`)
	handleServerEvent(c, new(string), agentclient.Event{Kind: "custom", Data: data})
	last := c.messages[len(c.messages)-1]
	if last.role != roleError {
		t.Errorf("expected roleError, got %v", last.role)
	}
}

func TestHandleServerEventAssistantEndTurnCommitsStream(t *testing.T) {
	c := newTestChatModel()
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000005",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"message":{
			"role":"assistant",
			"content":[{"type":"text","text":"Hi!"}]
		},
		"stopReason":"end_turn",
		"usage":{"promptTokens":5,"completionTokens":1,"totalTokens":6}
	}`)
	handleServerEvent(c, new(string), agentclient.Event{Kind: "message", Data: data})
	if len(c.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	last := c.messages[len(c.messages)-1]
	if last.role != roleAssistant {
		t.Errorf("expected roleAssistant, got %v", last.role)
	}
	if last.usage == nil {
		t.Errorf("expected usage to be populated")
	}
}