package tui

import (
	"errors"
	"testing"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
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

func TestStreamEventSchedulesExactlyOneFollowUpRead(t *testing.T) {
	m := NewModelForTest()
	events := make(chan agentclient.Event, 3)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"a"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"b"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"c"}`)}
	m.events = events

	out, _ := m.Update(streamEventMsg{ev: agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"a"}`)}})
	m = out.(*Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 1 {
		t.Errorf("non-done streamEventMsg: expected 1 follow-up read, got %d", got)
	}

	out, _ = m.Update(streamEventMsg{ev: agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"b"}`)}})
	m = out.(*Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 1 {
		t.Errorf("2nd non-done streamEventMsg: expected 1 follow-up read, got %d", got)
	}
}

func TestStreamEventDoneSchedulesNoFollowUpRead(t *testing.T) {
	m := NewModelForTest()
	events := make(chan agentclient.Event, 1)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}
	m.events = events

	out, _ := m.Update(streamEventMsg{ev: agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}, done: true})
	m = out.(*Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 0 {
		t.Errorf("done streamEventMsg: expected 0 follow-up reads, got %d", got)
	}
	if m.EventsForTest() != nil {
		t.Errorf("done streamEventMsg: m.events should be nil")
	}
}

func TestStreamEventErrorClearsEvents(t *testing.T) {
	m := NewModelForTest()
	m.state = StateStreamingForTestValue()
	events := make(chan agentclient.Event, 1)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"orphan"}`)}
	m.events = events

	out, _ := m.Update(streamEventMsg{err: errors.New("boom")})
	m = out.(*Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 0 {
		t.Errorf("error streamEventMsg: expected 0 follow-up reads, got %d", got)
	}
	if m.EventsForTest() != nil {
		t.Errorf("error streamEventMsg: m.events should be nil")
	}
	if m.StateForTest() != StateError {
		t.Errorf("error streamEventMsg: state should be StateError, got %v", m.StateForTest())
	}
}

func TestCancelDrainsInFlightEvents(t *testing.T) {
	m := NewModelForTest()
	m.state = StateStreamingForTestValue()
	events := make(chan agentclient.Event, 4)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"1"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"2"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"3"}`)}
	m.events = events
	// stub out the cancel RPC by leaving m.conn nil
	m.conn = nil

	m.cancel()

	if got := len(events); got != 0 {
		t.Errorf("cancel: expected events channel drained, %d buffered remain", got)
	}
	if m.EventsForTest() != nil {
		t.Errorf("cancel: m.events should be nil after cancel")
	}
	if m.StateForTest() != StateReady {
		t.Errorf("cancel: state should be StateReady, got %v", m.StateForTest())
	}
}

func TestGetMdRendererEvictsBeyondCap(t *testing.T) {
	resetMdRenderersForTest()
	defer resetMdRenderersForTest()

	const cap = 8

	r0 := getMdRenderer(10)
	for i := 1; i < cap; i++ {
		getMdRenderer(20 + i)
	}
	// cache now holds widths {10, 21..27} (=cap entries)

	getMdRenderer(30)
	// 10 should have been evicted (oldest insert)

	r0Again := getMdRenderer(10)
	if r0Again == r0 {
		t.Errorf("width=10 should have been evicted on the 9th distinct insertion; got same renderer pointer %p", r0)
	}
}