package tui_test

import (
	"encoding/json"
	"errors"
	"testing"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestHandleServerEventMessageWithThinkingAndTextAndToolCall(t *testing.T) {
	c := tui.NewChatModelForTest()
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
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "message", Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (tool call), got %d", len(msgs))
	}
	if msgs[0].Role != tui.RoleTool {
		t.Errorf("expected RoleTool, got %v", msgs[0].Role)
	}
}

func TestHandleServerEventToolResultAppearsAsObserve(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"date"}`),
	})
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
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "message", Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) != 1 {
		t.Fatalf("expected tool call entry merged with result (1 message), got %d", len(msgs))
	}
	if msgs[0].Role != tui.RoleTool {
		t.Errorf("expected RoleTool, got %v", msgs[0].Role)
	}
	if msgs[0].Result != "Wed Sep 23 14:00" {
		t.Errorf("expected tool result to be merged into call, got %q", msgs[0].Result)
	}
	if msgs[0].ResultFailed {
		t.Errorf("expected ResultFailed to be false, got true")
	}
}

func TestHandleServerEventToolResultErrorAppearsAsError(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"false"}`),
	})
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000003",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"message":{
			"role":"toolResult",
			"content":[{"type":"text","text":"boom"}]
		},
		"details":{"toolName":"bash","error":"boom"}
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "message", Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) != 1 {
		t.Fatalf("expected tool call entry merged with error (1 message), got %d", len(msgs))
	}
	if msgs[0].Role != tui.RoleTool {
		t.Errorf("expected RoleTool, got %v", msgs[0].Role)
	}
	if !msgs[0].ResultFailed {
		t.Errorf("expected ResultFailed to be true after tool error")
	}
}

func TestHandleServerEventCustomToolErrorAppendsError(t *testing.T) {
	c := tui.NewChatModelForTest()
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000004",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"tool_error",
		"data":{"error":"boom"}
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "custom", Data: data})
	msgs := c.MessagesForTest()
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleError {
		t.Errorf("expected RoleError when no running tool, got %v", last.Role)
	}
}

func TestHandleServerEventAssistantEndTurnCommitsStream(t *testing.T) {
	c := tui.NewChatModelForTest()
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
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "message", Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleAssistant {
		t.Errorf("expected RoleAssistant, got %v", last.Role)
	}
	if last.Usage == nil {
		t.Errorf("expected usage to be populated")
	}
}

func TestStreamEventSchedulesExactlyOneFollowUpRead(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 3)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"a"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"b"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"c"}`)}
	m.SetEventsForTest(events)

	out, _ := m.Update(tui.StreamEventMsgForTest(agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"a"}`)}, nil, false))
	m, _ = out.(*tui.Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 1 {
		t.Errorf("non-done streamEventMsg: expected 1 follow-up read, got %d", got)
	}

	out, _ = m.Update(tui.StreamEventMsgForTest(agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"b"}`)}, nil, false))
	m, _ = out.(*tui.Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 1 {
		t.Errorf("2nd non-done streamEventMsg: expected 1 follow-up read, got %d", got)
	}
}

func TestStreamEventDoneSchedulesNoFollowUpRead(t *testing.T) {
	m := tui.NewModelForTest()
	events := make(chan agentclient.Event, 1)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}
	m.SetEventsForTest(events)

	out, _ := m.Update(tui.StreamEventMsgForTest(agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}, nil, true))
	m, _ = out.(*tui.Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 0 {
		t.Errorf("done streamEventMsg: expected 0 follow-up reads, got %d", got)
	}
	if m.EventsForTest() != nil {
		t.Errorf("done streamEventMsg: m.events should be nil")
	}
}

func TestStreamEventErrorClearsEvents(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetStateForTest(tui.StateStreamingForTestValue())
	events := make(chan agentclient.Event, 1)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"orphan"}`)}
	m.SetEventsForTest(events)

	out, _ := m.Update(tui.StreamEventMsgForTest(agentclient.Event{}, errors.New("boom"), false))
	m, _ = out.(*tui.Model)
	if got := m.ReadsScheduledThisUpdateForTest(); got != 0 {
		t.Errorf("error streamEventMsg: expected 0 follow-up reads, got %d", got)
	}
	if m.EventsForTest() != nil {
		t.Errorf("error streamEventMsg: m.events should be nil")
	}
	if m.StateForTest() != tui.StateError {
		t.Errorf("error streamEventMsg: state should be StateError, got %v", m.StateForTest())
	}
}

func TestCancelDrainsInFlightEvents(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetStateForTest(tui.StateStreamingForTestValue())
	events := make(chan agentclient.Event, 4)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"1"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"2"}`)}
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"3"}`)}
	m.SetEventsForTest(events)
	m.SetConnForTest(nil)

	m.CancelForTest()

	if got := len(events); got != 0 {
		t.Errorf("cancel: expected events channel drained, %d buffered remain", got)
	}
}

func TestCancelTransitionsThroughCancellingToReady(t *testing.T) {
	m := tui.NewModelForTest()
	m.SetStateForTest(tui.StateStreamingForTestValue())
	events := make(chan agentclient.Event, 1)
	events <- agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}
	m.SetEventsForTest(events)
	m.SetConnForTest(nil)

	cmd := m.CancelForTest()
	if cmd == nil {
		t.Fatalf("cancel: expected a follow-up readNextEvent cmd to be scheduled, got nil")
	}
	if m.StateForTest() != tui.StateCancelling {
		t.Errorf("after cancel: expected StateCancelling, got %v", m.StateForTest())
	}
	if m.EventsForTest() == nil {
		t.Errorf("after cancel: m.events must stay referenced so the natural stream-close can deliver done=true")
	}

	out, _ := m.Update(tui.StreamEventMsgForTest(agentclient.Event{Kind: json_rpc.EventMessage, Data: []byte(`{"id":"final"}`)}, nil, false))
	m, _ = out.(*tui.Model)
	if m.StateForTest() != tui.StateReady {
		t.Errorf("after trailing streamEventMsg: expected StateReady, got %v", m.StateForTest())
	}
}

func TestGetMdRendererEvictsBeyondCap(t *testing.T) {
	tui.ResetMdRenderersForTest()
	defer tui.ResetMdRenderersForTest()

	const cap = 8

	r0 := tui.GetMdRendererForTest(10)
	for i := 1; i < cap; i++ {
		tui.GetMdRendererForTest(20 + i)
	}

	tui.GetMdRendererForTest(30)

	r0Again := tui.GetMdRendererForTest(10)
	if r0Again == r0 {
		t.Errorf("width=10 should have been evicted on the 9th distinct insertion; got same renderer pointer %p", r0)
	}
}

func TestChatLineCountCachesPerMessage(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.SetSizeForTest(80, 40)
	c.SubmitForTest("first prompt")
	c.SubmitForTest("second prompt")
	c.SubmitForTest("third prompt")
	msgs := c.MessagesForTest()
	if got := len(msgs); got != 3 {
		t.Fatalf("setup: expected 3 messages, got %d", got)
	}
	missesBefore := c.LineCountMissesForTest()
	for i := 0; i < 100; i++ {
		for j := range msgs {
			c.LineCountForTest(msgs[j])
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
