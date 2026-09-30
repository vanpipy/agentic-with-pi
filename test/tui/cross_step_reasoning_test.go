package tui_test

import (
	"strings"
	"testing"

	agentclient "github.com/vanpiyp/awp/internal/agent-client"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/tui"
)

// streamingMessageJSON builds the JSON payload for a MessageEvent that the
// agent-server emits during a thinking/text stream. interimID and stopReason
// are taken verbatim from event_translator.go's onThoughtChunk
// (StopReason: "streaming", per-step interimID).
func streamingMessageJSON(id, parentID, reasoning, text string) []byte {
	return []byte(`{
		"id":"` + id + `",
		"parentId":"` + parentID + `",
		"timestamp":"2026-09-30T14:00:00.000Z",
		"message":{"role":"assistant","content":[
			{"type":"thinking","thinking":"` + jsonEscape(reasoning) + `"},
			{"type":"text","text":"` + jsonEscape(text) + `"}
		]},
		"stopReason":"streaming"
	}`)
}

// toolUseMessage builds the JSON for an assistant tool_use MessageEvent.
func toolUseMessage(id, parentID, toolName, argsJSON, intent string) []byte {
	return []byte(`{
		"id":"` + id + `",
		"parentId":"` + parentID + `",
		"timestamp":"2026-09-30T14:00:01.000Z",
		"message":{"role":"assistant","content":[
			{"type":"toolCall","id":"` + id + `-call","name":"` + toolName + `","intent":"` + jsonEscape(intent) + `","arguments":` + argsJSON + `}
		]},
		"stopReason":"toolUse"
	}`)
}

// toolResultMessage builds the JSON for a tool_result MessageEvent.
func toolResultMessage(id, parentID, toolName, intent, text string) []byte {
	return []byte(`{
		"id":"` + id + `",
		"parentId":"` + parentID + `",
		"timestamp":"2026-09-30T14:00:02.000Z",
		"message":{"role":"toolResult","content":[
			{"type":"text","text":"` + jsonEscape(text) + `"}
		]},
		"stopReason":"toolUse",
		"details":{"toolName":"` + toolName + `","intent":"` + jsonEscape(intent) + `"}
	}`)
}

func jsonEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\t", `\t`,
	)
	return r.Replace(s)
}

func TestHandleMessageEvent_ToolCallCommitsPriorReasoning(t *testing.T) {
	c := tui.NewChatModelForTest()
	sessionID := ""
	const parent = "0190a3b7-0000-7c8a-9000-000000000001"

	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: streamingMessageJSON("0190a3b7-0000-7c8a-9000-000000000010", parent, "first thought segment", ""),
	})
	if c.ReasoningLenForTest() == 0 {
		t.Fatal("reasoning buffer must be populated after streaming thinking part")
	}
	if got := len(c.MessagesForTest()); got != 0 {
		t.Fatalf("no committed chatMsg expected yet (only interim thinking), got %d", got)
	}

	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: toolUseMessage("0190a3b7-0000-7c8a-9000-000000000020", parent, "read", `{"file":"x.go"}`, "read x.go"),
	})
	msgs := c.MessagesForTest()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 chatMsgs (committed reasoning + tool), got %d: %+v", len(msgs), rolesOfMsgs(msgs))
	}
	if msgs[0].Role != tui.RoleThinking {
		t.Errorf("msgs[0].Role = %v, want RoleThinking (committed pre-tool reasoning)", msgs[0].Role)
	}
	if msgs[0].Text != "first thought segment" {
		t.Errorf("msgs[0].Text = %q, want %q", msgs[0].Text, "first thought segment")
	}
	if msgs[1].Role != tui.RoleTool {
		t.Errorf("msgs[1].Role = %v, want RoleTool", msgs[1].Role)
	}
	if c.ReasoningLenForTest() != 0 {
		t.Errorf("reasoning buffer must be empty after commit, got len=%d", c.ReasoningLenForTest())
	}
}

func TestHandleMessageEvent_SecondReasoningSegmentIsSeparateBlock(t *testing.T) {
	c := tui.NewChatModelForTest()
	sessionID := ""
	const parent = "0190a3b7-0000-7c8a-9000-000000000001"

	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: streamingMessageJSON("0190a3b7-0000-7c8a-9000-000000000010", parent, "step 1 reasoning", ""),
	})
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: toolUseMessage("0190a3b7-0000-7c8a-9000-000000000020", parent, "read", `{"file":"a.go"}`, ""),
	})
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: toolResultMessage("0190a3b7-0000-7c8a-9000-000000000030", "0190a3b7-0000-7c8a-9000-000000000020-call", "read", "read", "file contents"),
	})
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: streamingMessageJSON("0190a3b7-0000-7c8a-9000-000000000040", "0190a3b7-0000-7c8a-9000-000000000030", "step 2 reasoning", ""),
	})
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: toolUseMessage("0190a3b7-0000-7c8a-9000-000000000050", "0190a3b7-0000-7c8a-9000-000000000030", "read", `{"file":"b.go"}`, ""),
	})

	msgs := c.MessagesForTest()
	if len(msgs) < 4 {
		t.Fatalf("expected at least 4 chatMsgs ([think1, tool1, observe, think2, tool2]), got %d: %+v", len(msgs), rolesOfMsgs(msgs))
	}

	think1Idx, think2Idx := -1, -1
	for i, m := range msgs {
		if m.Role != tui.RoleThinking {
			continue
		}
		if m.Text == "step 1 reasoning" {
			think1Idx = i
		} else if m.Text == "step 2 reasoning" {
			think2Idx = i
		}
	}
	if think1Idx == -1 {
		t.Fatalf("first reasoning block (step 1) never committed; msgs=%+v", rolesOfMsgs(msgs))
	}
	if think2Idx == -1 {
		t.Fatalf("second reasoning block (step 2) never committed (likely merged into step 1); msgs=%+v", rolesOfMsgs(msgs))
	}
	if think1Idx == think2Idx {
		t.Fatalf("both step 1 and step 2 reasoning landed in the same chatMsg; msgs=%+v", rolesOfMsgs(msgs))
	}

	if c.ReasoningLenForTest() != 0 {
		t.Errorf("reasoning buffer must be empty after second tool call commit, got len=%d", c.ReasoningLenForTest())
	}
}

func TestHandleMessageEvent_ToolCallWithEmptyBuffersIsNoop(t *testing.T) {
	c := tui.NewChatModelForTest()
	sessionID := ""
	const parent = "0190a3b7-0000-7c8a-9000-000000000001"

	before := len(c.MessagesForTest())
	tui.HandleServerEventForTest(c, &sessionID, agentclient.Event{
		Kind: json_rpc.EventMessage,
		Data: toolUseMessage("0190a3b7-0000-7c8a-9000-000000000020", parent, "read", `{"file":"x.go"}`, ""),
	})

	msgs := c.MessagesForTest()
	if len(msgs) != before+1 {
		t.Fatalf("expected exactly 1 new chatMsg (the tool), got before=%d after=%d", before, len(msgs))
	}
	if msgs[len(msgs)-1].Role != tui.RoleTool {
		t.Errorf("only chatMsg appended must be RoleTool, got %v", msgs[len(msgs)-1].Role)
	}
}
