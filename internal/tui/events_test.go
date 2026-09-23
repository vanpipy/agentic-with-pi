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
}

func TestCancelTransitionsThroughCancellingToReady(t *testing.T) {
	m := NewModelForTest()
	m.state = StateStreamingForTestValue()
	events := make(chan agentclient.Event, 1)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}
	m.events = events
	m.conn = nil

	cmd := m.cancel()
	if cmd == nil {
		t.Fatalf("cancel: expected a follow-up readNextEvent cmd to be scheduled, got nil")
	}
	if m.StateForTest() != StateCancelling {
		t.Errorf("after cancel: expected StateCancelling, got %v", m.StateForTest())
	}
	if m.EventsForTest() == nil {
		t.Errorf("after cancel: m.events must stay referenced so the natural stream-close can deliver done=true")
	}

	out, _ := m.Update(streamEventMsg{ev: agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}})
	m = out.(*Model)
	if m.StateForTest() != StateReady {
		t.Errorf("after trailing streamEventMsg: expected StateReady, got %v", m.StateForTest())
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

func TestChatLineCountCachesPerMessage(t *testing.T) {
	c := newChatModel()
	c.SetSize(80, 40)
	c.submit("first prompt")
	c.submit("second prompt")
	c.submit("third prompt")
	if got := len(c.messages); got != 3 {
		t.Fatalf("setup: expected 3 messages, got %d", got)
	}
	// submit() calls refresh() which pre-warms the cache via content().
	// Each submit's refresh re-renders all current messages through renderedLines,
	// producing 1+2+3 = 6 cumulative cache misses (no longer lineCount-specific).
	missesBefore := c.LineCountMissesForTest()
	for i := 0; i < 100; i++ {
		for j := range c.messages {
			c.lineCount(c.messages[j])
		}
	}
	got := c.LineCountMissesForTest() - missesBefore
	if got != 0 {
		t.Errorf("expected 0 lineCount misses after pre-warm (per-message cache hits), got %d", got)
	}
	if total := c.LineCountMissesForTest(); total != 6 {
		t.Errorf("expected 6 cumulative renderedLines misses (1+2+3 from submit pre-warm), got %d", total)
	}
}
