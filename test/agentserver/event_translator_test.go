package agentserver_test

import (
	"encoding/json"
	"testing"

	agentcore "github.com/vanpiyp/awp/internal/agent-core"
	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	agentserver "github.com/vanpiyp/awp/internal/agent-server"
	"github.com/vanpiyp/awp/internal/llm"
)

func newStreamTranslator() *agentserver.StreamTranslator {
	return agentserver.NewStreamTranslator()
}

func TestStreamTranslatorEventUserMessage(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{Category: agentcore.EventUserMessage, Content: "hello"})
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
	if emits[0].EventName != json_rpc.EventMessage {
		t.Errorf("eventName = %q, want %q", emits[0].EventName, json_rpc.EventMessage)
	}
	msg, ok := emits[0].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("payload type = %T, want MessageEvent", emits[0].Payload)
	}
	if msg.Message.Role != "user" {
		t.Errorf("role = %q, want user", msg.Message.Role)
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("stopReason = %q, want end_turn", msg.StopReason)
	}
	if len(msg.Message.Content) != 1 || msg.Message.Content[0].Text != "hello" {
		t.Errorf("content = %+v", msg.Message.Content)
	}
	if tr.ParentIDForTest() != msg.ID {
		t.Errorf("parentID not advanced: got %q, msg.ID = %q", tr.ParentIDForTest(), msg.ID)
	}
}

func TestStreamTranslatorEventThoughtStartCreatesBuffer(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0", len(emits))
	}
	if tr.StreamBufForTest() == nil {
		t.Error("streamBuf should be created by ThoughtStart")
	}
}

func TestStreamTranslatorEventThoughtChunkNoEmit(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "reasoning-1"})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (buffered)", len(emits))
	}
}

func TestStreamTranslatorEventThoughtChunkNilBufferNoPanic(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "x"})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil buffer)", len(emits))
	}
}

func TestStreamTranslatorEventThoughtEnd(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtEnd, Usage: &llm.Usage{
		PromptTokens:     11,
		CompletionTokens: 22,
		TotalTokens:      33,
	}})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0", len(emits))
	}
}

func TestStreamTranslatorEventToolNilBufferNoEmit(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "bash",
		ToolArgs: `{}`,
	})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil streamBuf)", len(emits))
	}
}

func TestStreamTranslatorEventToolEmitsAssistantAndToolResult(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{
		Category:   agentcore.EventTool,
		ToolName:   "bash",
		ToolArgs:   `{"command":"date","intent":"Get time"}`,
		ToolIntent: "Get time",
	})
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2 (assistant + toolResult)", len(emits))
	}
	assistant, ok := emits[0].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("emit 0 type = %T, want MessageEvent", emits[0].Payload)
	}
	if assistant.Message.Role != "assistant" {
		t.Errorf("assistant role = %q", assistant.Message.Role)
	}
	if assistant.StopReason != "toolUse" {
		t.Errorf("assistant stopReason = %q", assistant.StopReason)
	}
	toolResult, ok := emits[1].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("emit 1 type = %T, want MessageEvent", emits[1].Payload)
	}
	if toolResult.Message.Role != "toolResult" {
		t.Errorf("toolResult role = %q", toolResult.Message.Role)
	}
	if toolResult.Details == nil || toolResult.Details.ToolName != "bash" {
		t.Errorf("toolResult details missing")
	}
	if toolResult.ParentID == "" {
		t.Errorf("toolResult.ParentID should be set")
	}
	if tr.ParentIDForTest() != toolResult.ID {
		t.Errorf("parentID should advance to toolResult.ID after tool call")
	}
	if tr.StreamBufForTest() == nil {
		t.Error("streamBuf should be re-created after tool emit")
	}
}

func TestStreamTranslatorEventToolEmptyArgs(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "ls",
		ToolArgs: "",
	})
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2 (empty args still emits)", len(emits))
	}
}

func TestStreamTranslatorEventToolIntentFallbackToExtract(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "ls",
		ToolArgs: `{"intent":"fallback"}`,
	})
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2", len(emits))
	}
	assistant, ok := emits[0].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("emit 0 type = %T", emits[0].Payload)
	}
	if len(assistant.Message.Content) == 0 {
		t.Fatal("expected at least one content part")
	}
	if assistant.Message.Content[0].Intent != "fallback" {
		t.Errorf("intent = %q, want fallback", assistant.Message.Content[0].Intent)
	}
}

func TestStreamTranslatorEventObserveNilBufferNoEmit(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{
		Category:   agentcore.EventObserve,
		ToolName:   "ls",
		ToolResult: "out",
	})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil buffer)", len(emits))
	}
}

func TestStreamTranslatorEventObserveEmitsAndResets(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	tr.Translate(agentcore.Event{
		Category: agentcore.EventTool,
		ToolName: "ls",
		ToolArgs: `{"intent":"list"}`,
	})
	emits := tr.Translate(agentcore.Event{
		Category:   agentcore.EventObserve,
		ToolName:   "ls",
		ToolResult: "out",
		ToolIntent: "x",
	})
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
	msg, ok := emits[0].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("payload type = %T, want MessageEvent", emits[0].Payload)
	}
	if msg.Message.Role != "" {
		t.Errorf("role = %q, want empty (fresh buffer after Tool)", msg.Message.Role)
	}
	if msg.StopReason != "toolUse" {
		t.Errorf("stopReason = %q, want toolUse", msg.StopReason)
	}
	if tr.StreamBufForTest() == nil {
		t.Error("streamBuf should be re-created after Observe")
	}
}

func TestStreamTranslatorEventFinalAnswerNilBufferNoEmit(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "ok",
	})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil buffer)", len(emits))
	}
}

func TestStreamTranslatorEventFinalAnswerEmitsAndClears(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{
		Category: agentcore.EventFinalAnswer,
		Content:  "answer",
		Usage:    &llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
	})
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1", len(emits))
	}
	msg, ok := emits[0].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("payload type = %T, want MessageEvent", emits[0].Payload)
	}
	if msg.Message.Role != "assistant" {
		t.Errorf("role = %q", msg.Message.Role)
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("stopReason = %q", msg.StopReason)
	}
	if tr.StreamBufForTest() != nil {
		t.Error("streamBuf should be nil after FinalAnswer")
	}
}

func TestStreamTranslatorEventErrorNilBufferNoEmit(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{
		Category:  agentcore.EventError,
		ToolError: "boom",
	})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 (nil buffer)", len(emits))
	}
}

func TestStreamTranslatorEventErrorEmitsMessageAndCustom(t *testing.T) {
	tr := newStreamTranslator()
	tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart})
	emits := tr.Translate(agentcore.Event{
		Category:  agentcore.EventError,
		ToolError: "boom",
	})
	if len(emits) != 2 {
		t.Fatalf("emits len = %d, want 2 (message + custom)", len(emits))
	}
	if emits[0].EventName != json_rpc.EventMessage {
		t.Errorf("emit 0 eventName = %q", emits[0].EventName)
	}
	if emits[1].EventName != json_rpc.EventCustom {
		t.Errorf("emit 1 eventName = %q", emits[1].EventName)
	}
	custom, ok := emits[1].Payload.(json_rpc.CustomEvent)
	if !ok {
		t.Fatalf("emit 1 type = %T, want CustomEvent", emits[1].Payload)
	}
	if custom.CustomType != "tool_error" {
		t.Errorf("customType = %q, want tool_error", custom.CustomType)
	}
	var data map[string]string
	if err := json.Unmarshal(custom.Data, &data); err != nil {
		t.Fatalf("custom.Data not valid JSON: %v", err)
	}
	if data["error"] != "boom" {
		t.Errorf("data.error = %q, want boom", data["error"])
	}
	if tr.StreamBufForTest() != nil {
		t.Error("streamBuf should be nil after Error")
	}
}

func TestStreamTranslatorEventCompactionEmitsCustomEvent(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{
		Category:        agentcore.EventCompaction,
		Summary:         "compacted summary",
		TokensBefore:    1000,
		TokensAfter:     500,
		FirstKeptSeq:    5,
		CompactionModel: "minimax-M3",
	})
	if len(emits) != 1 {
		t.Fatalf("emits len = %d, want 1 (custom compaction event)", len(emits))
	}
	if emits[0].EventName != json_rpc.EventCustom {
		t.Errorf("eventName = %q, want %q", emits[0].EventName, json_rpc.EventCustom)
	}
	custom, ok := emits[0].Payload.(json_rpc.CustomEvent)
	if !ok {
		t.Fatalf("payload type = %T, want CustomEvent", emits[0].Payload)
	}
	if custom.CustomType != "compaction" {
		t.Errorf("customType = %q, want compaction", custom.CustomType)
	}
	if custom.ID == "" {
		t.Error("custom.ID should be set (UUIDv7)")
	}
	if custom.Timestamp == "" {
		t.Error("custom.Timestamp should be set (RFC3339)")
	}
	var data map[string]any
	if err := json.Unmarshal(custom.Data, &data); err != nil {
		t.Fatalf("custom.Data not valid JSON: %v", err)
	}
	if data["summary"] != "compacted summary" {
		t.Errorf("data.summary = %v, want compacted summary", data["summary"])
	}
	if v, ok := data["tokens_before"].(float64); !ok || int(v) != 1000 {
		t.Errorf("data.tokens_before = %v, want 1000", data["tokens_before"])
	}
	if v, ok := data["tokens_after"].(float64); !ok || int(v) != 500 {
		t.Errorf("data.tokens_after = %v, want 500", data["tokens_after"])
	}
	if v, ok := data["first_kept_seq"].(float64); !ok || int(v) != 5 {
		t.Errorf("data.first_kept_seq = %v, want 5", data["first_kept_seq"])
	}
	if data["model"] != "minimax-M3" {
		t.Errorf("data.model = %v, want minimax-M3", data["model"])
	}
}

func TestStreamTranslatorEventInvalidNoEmit(t *testing.T) {
	tr := newStreamTranslator()
	emits := tr.Translate(agentcore.Event{Category: agentcore.EventInvalid})
	if len(emits) != 0 {
		t.Errorf("emits len = %d, want 0 for EventInvalid", len(emits))
	}
}

func TestStreamTranslatorStateAcrossEvents(t *testing.T) {
	tr := newStreamTranslator()

	userEmits := tr.Translate(agentcore.Event{Category: agentcore.EventUserMessage, Content: "Q?"})
	if len(userEmits) != 1 {
		t.Fatalf("user emits = %d, want 1", len(userEmits))
	}
	userMsg, ok := userEmits[0].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("user payload type = %T", userEmits[0].Payload)
	}
	if userMsg.ParentID != "" {
		t.Errorf("first user message should have no ParentID, got %q", userMsg.ParentID)
	}
	if tr.ParentIDForTest() != userMsg.ID {
		t.Fatalf("parentID after user = %q, want %q", tr.ParentIDForTest(), userMsg.ID)
	}

	if emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtStart}); len(emits) != 0 {
		t.Fatalf("thoughtStart emits = %d", len(emits))
	}
	if tr.StreamBufForTest() == nil {
		t.Fatal("streamBuf should be created after ThoughtStart")
	}

	for i := 0; i < 3; i++ {
		if emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtChunk, Reasoning: "x"}); len(emits) != 0 {
			t.Errorf("thoughtChunk[%d] emits = %d", i, len(emits))
		}
	}

	if emits := tr.Translate(agentcore.Event{Category: agentcore.EventThoughtEnd, Usage: &llm.Usage{
		PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3,
	}}); len(emits) != 0 {
		t.Errorf("thoughtEnd emits = %d, want 0", len(emits))
	}

	toolEmits := tr.Translate(agentcore.Event{
		Category:   agentcore.EventTool,
		ToolName:   "bash",
		ToolArgs:   `{"intent":"run something"}`,
		ToolIntent: "run something",
	})
	if len(toolEmits) != 2 {
		t.Fatalf("tool emits = %d, want 2", len(toolEmits))
	}
	toolResult, ok := toolEmits[1].Payload.(json_rpc.MessageEvent)
	if !ok {
		t.Fatalf("toolResult payload type = %T", toolEmits[1].Payload)
	}
	if tr.ParentIDForTest() != toolResult.ID {
		t.Errorf("parentID after tool = %q, want toolResult.ID = %q", tr.ParentIDForTest(), toolResult.ID)
	}
	if toolResult.ParentID == "" {
		t.Errorf("toolResult.ParentID should be non-empty (tool call ID)")
	}
	if toolResult.ParentID == toolEmits[0].Payload.(json_rpc.MessageEvent).ID {
		t.Errorf("toolResult.ParentID should NOT equal assistant.ID; it must be the tool call ID")
	}
}
