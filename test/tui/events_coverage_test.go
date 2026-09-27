package tui_test

import (
	"encoding/json"
	"strings"
	"testing"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

func TestHandleServerEventSessionStartedUpdatesSessionID(t *testing.T) {
	sessionID := ""
	c := tui.NewChatModelForTest()
	data := []byte(`{"session_id":"0190a3b7-dead-beef-9000-000000000001"}`)
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{Kind: "session_started", Data: data})
	if sessionID != "0190a3b7-dead-beef-9000-000000000001" {
		t.Errorf("session_started: sessionID = %q, want the parsed value", sessionID)
	}
	if got := len(c.MessagesForTest()); got != 0 {
		t.Errorf("session_started should not append any chatMsg, got %d", got)
	}
}

func TestHandleServerEventSessionStartedEmptyDataNoPanic(t *testing.T) {
	sessionID := ""
	c := tui.NewChatModelForTest()
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{Kind: "session_started", Data: []byte(`{}`)})
	if sessionID != "" {
		t.Errorf("empty payload should leave sessionID empty, got %q", sessionID)
	}
}

func TestHandleServerEventCustomMessageIsNoop(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000010",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"stream_chunk",
		"content":"hello",
		"display":false
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: json_rpc.EventCustomMessage, Data: data})
	if got := len(c.MessagesForTest()); got != before {
		t.Errorf("custom_message should not append chatMsg, before=%d after=%d", before, got)
	}
}

func TestHandleServerEventCancelledCommitsStreamAndAppendsSystem(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendStreamForTest("partial reply")
	before := len(c.MessagesForTest())
	data := []byte(`{"reason":"user pressed esc"}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "cancelled", Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) <= before {
		t.Fatalf("cancelled should append a system message, before=%d after=%d", before, len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleSystem {
		t.Errorf("cancelled: last role = %v, want %v", last.Role, tui.RoleSystem)
	}
	if !strings.Contains(last.Text, "user pressed esc") {
		t.Errorf("cancelled: last text should include reason, got %q", last.Text)
	}
	if c.StreamingLenForTest() != 0 {
		t.Errorf("cancelled: expected commitStream to flush streaming buffer, len=%d", c.StreamingLenForTest())
	}
}

func TestHandleServerEventCancelledMalformedReasonStillAppends(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	data := []byte(`not-valid-json`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: "cancelled", Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) <= before {
		t.Fatalf("cancelled with malformed data should still append a system message, before=%d after=%d", before, len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleSystem {
		t.Errorf("cancelled malformed: last role = %v, want %v", last.Role, tui.RoleSystem)
	}
}

func TestHandleCustomEventAbortAppendsSystem(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000020",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"abort",
		"data":{"reason":"timeout"}
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: json_rpc.EventCustom, Data: data})
	msgs := c.MessagesForTest()
	if len(msgs) <= before {
		t.Fatalf("abort should append a system message, before=%d after=%d", before, len(msgs))
	}
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleSystem {
		t.Errorf("abort: last role = %v, want %v", last.Role, tui.RoleSystem)
	}
	if !strings.Contains(last.Text, "timeout") {
		t.Errorf("abort: last text should include reason, got %q", last.Text)
	}
}

func TestHandleCustomEventMalformedJSONReturnsSilently(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: json_rpc.EventCustom, Data: []byte(`not-valid-json`)})
	if got := len(c.MessagesForTest()); got != before {
		t.Errorf("malformed custom JSON should append nothing, before=%d after=%d", before, got)
	}
}

func TestHandleCustomEventUnknownTypeIsNoop(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000021",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"some_other_type",
		"data":{"foo":"bar"}
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: json_rpc.EventCustom, Data: data})
	if got := len(c.MessagesForTest()); got != before {
		t.Errorf("unknown customType should append nothing, before=%d after=%d", before, got)
	}
}

func TestHandleCustomEventToolErrorEmptyMessageNoAppend(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := len(c.MessagesForTest())
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000022",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"tool_error",
		"data":{}
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: json_rpc.EventCustom, Data: data})
	if got := len(c.MessagesForTest()); got != before {
		t.Errorf("tool_error with empty error should not append (defensive guard), before=%d after=%d", before, got)
	}
}

func TestHandleCustomEventToolErrorMergedIntoRunningTool(t *testing.T) {
	c := tui.NewChatModelForTest()
	tui.AppendToolForTest(c, json_rpc.MessageContentPart{
		Type:      "toolCall",
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"false"}`),
	})
	before := len(c.MessagesForTest())
	data := []byte(`{
		"id":"0190a3b7-0001-7c8a-9000-000000000023",
		"timestamp":"2026-09-23T13:00:00.000Z",
		"customType":"tool_error",
		"data":{"error":"exit 1"}
	}`)
	tui.HandleServerEventForTest(c, new(string), agentclient.Event{Kind: json_rpc.EventCustom, Data: data})
	if got := len(c.MessagesForTest()); got != before {
		t.Errorf("tool_error with running tool should merge into existing tool row, not append, before=%d after=%d", before, got)
	}
	last := c.MessagesForTest()[len(c.MessagesForTest())-1]
	if last.Result != "exit 1" {
		t.Errorf("tool_error merge: last result = %q, want %q", last.Result, "exit 1")
	}
	if !last.ResultFailed {
		t.Errorf("tool_error merge: last resultFailed = false, want true")
	}
}
