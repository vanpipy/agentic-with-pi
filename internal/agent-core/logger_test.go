package agentcore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
	"github.com/vanpiyp/awp/internal/llm"
)

type alignedEnvelope struct {
	Kind    string          `json:"kind"`
	Version int             `json:"version"`
	Entry   json.RawMessage `json:"entry"`
}

func newTestAgentWithBuf() (*Agent, *bytes.Buffer) {
	var buf bytes.Buffer
	a := &Agent{
		LogWriter: &buf,
		logBuf:    bufio.NewWriterSize(&buf, 4096),
		logMu:     sync.Mutex{},
	}
	return a, &buf
}

func flushAgentLog(a *Agent) {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logBuf != nil {
		_ = a.logBuf.Flush()
	}
}

func parseEnvelope(t *testing.T, line string) alignedEnvelope {
	t.Helper()
	var env alignedEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\nline: %s", err, line)
	}
	return env
}

func TestWriteMessageEmitsVersion2MessageEnvelope(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	msg := json_rpc.MessageEvent{
		ID:        json_rpc.NewV7(),
		Timestamp: "2026-09-23T14:00:00Z",
		Message: json_rpc.Message{
			Role:    "user",
			Content: []json_rpc.MessageContentPart{{Type: "text", Text: "hello"}},
		},
		StopReason: "end_turn",
	}
	if err := a.WriteMessage(msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d:\n%s", len(lines), buf.String())
	}
	env := parseEnvelope(t, lines[0])
	if env.Kind != "message" {
		t.Errorf("envelope.kind=%q, want message", env.Kind)
	}
	if env.Version != 2 {
		t.Errorf("envelope.version=%d, want 2", env.Version)
	}

	var got json_rpc.MessageEvent
	if err := json.Unmarshal(env.Entry, &got); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if got.Message.Role != "user" {
		t.Errorf("entry.message.role=%q, want user", got.Message.Role)
	}
	if len(got.Message.Content) != 1 || got.Message.Content[0].Text != "hello" {
		t.Errorf("entry.message.content mismatch: %+v", got.Message.Content)
	}
	if got.StopReason != "end_turn" {
		t.Errorf("entry.stopReason=%q, want end_turn", got.StopReason)
	}
}

func TestWriteMessageUpdatesCurrentParentID(t *testing.T) {
	a, _ := newTestAgentWithBuf()

	msg := json_rpc.MessageEvent{ID: "msg-fixed-id-1", Timestamp: "2026-09-23T14:00:00Z"}
	if err := a.WriteMessage(msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	a.logMu.Lock()
	got := a.currentParentID
	a.logMu.Unlock()
	if got != "msg-fixed-id-1" {
		t.Errorf("currentParentID=%q, want msg-fixed-id-1", got)
	}
}

func TestWriteCustomEmitsVersion2CustomEnvelope(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	if err := a.WriteCustom("parent-xyz", "tool_error", json.RawMessage(`{"error":"boom"}`)); err != nil {
		t.Fatalf("WriteCustom: %v", err)
	}
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}
	env := parseEnvelope(t, lines[0])
	if env.Kind != "custom" {
		t.Errorf("envelope.kind=%q, want custom", env.Kind)
	}
	if env.Version != 2 {
		t.Errorf("envelope.version=%d, want 2", env.Version)
	}

	var got json_rpc.CustomEvent
	if err := json.Unmarshal(env.Entry, &got); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if got.CustomType != "tool_error" {
		t.Errorf("entry.customType=%q, want tool_error", got.CustomType)
	}
	if got.ParentID != "parent-xyz" {
		t.Errorf("entry.parentId=%q, want parent-xyz", got.ParentID)
	}
	if string(got.Data) != `{"error":"boom"}` {
		t.Errorf("entry.data=%q, want {\"error\":\"boom\"}", string(got.Data))
	}
	if !uuidV7Re.MatchString(got.ID) {
		t.Errorf("entry.id=%q not UUID v7", got.ID)
	}
}

func TestWriteCustomMessageEmitsVersion2CustomMessageEnvelope(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	if err := a.WriteCustomMessage("parent-xyz", "skill_invoked", "review-pr", json.RawMessage(`{"skill":"review-pr"}`)); err != nil {
		t.Fatalf("WriteCustomMessage: %v", err)
	}
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}
	env := parseEnvelope(t, lines[0])
	if env.Kind != "custom_message" {
		t.Errorf("envelope.kind=%q, want custom_message", env.Kind)
	}
	if env.Version != 2 {
		t.Errorf("envelope.version=%d, want 2", env.Version)
	}

	var got json_rpc.CustomMessageEvent
	if err := json.Unmarshal(env.Entry, &got); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if got.CustomType != "skill_invoked" {
		t.Errorf("entry.customType=%q, want skill_invoked", got.CustomType)
	}
	if got.Content != "review-pr" {
		t.Errorf("entry.content=%q, want review-pr", got.Content)
	}
	if got.ParentID != "parent-xyz" {
		t.Errorf("entry.parentId=%q, want parent-xyz", got.ParentID)
	}
	if string(got.Details) != `{"skill":"review-pr"}` {
		t.Errorf("entry.details=%q, want {\"skill\":\"review-pr\"}", string(got.Details))
	}
}

func TestWriteAlignedEventUserMessageEmitsUserRoleMessage(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	a.writeAlignedEvent(Event{Category: EventUserMessage, Content: "what time is it"})
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d:\n%s", len(lines), buf.String())
	}
	env := parseEnvelope(t, lines[0])
	if env.Kind != "message" {
		t.Fatalf("envelope.kind=%q, want message", env.Kind)
	}
	var msg json_rpc.MessageEvent
	if err := json.Unmarshal(env.Entry, &msg); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if msg.Message.Role != "user" {
		t.Errorf("role=%q, want user", msg.Message.Role)
	}
	if len(msg.Message.Content) != 1 || msg.Message.Content[0].Type != "text" || msg.Message.Content[0].Text != "what time is it" {
		t.Errorf("content mismatch: %+v", msg.Message.Content)
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("stopReason=%q, want end_turn", msg.StopReason)
	}
	if msg.ParentID != "" {
		t.Errorf("user message should have empty parentId, got %q", msg.ParentID)
	}
	if !uuidV7Re.MatchString(msg.ID) {
		t.Errorf("id=%q not UUID v7", msg.ID)
	}
}

func TestWriteAlignedEventFullChainProducesFourMessages(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	events := []Event{
		{Category: EventUserMessage, Content: "what time is it"},
		{Category: EventThoughtStart},
		{Category: EventThoughtChunk, Reasoning: "The user "},
		{Category: EventThoughtChunk, Reasoning: "wants time"},
		{Category: EventThoughtEnd},
		{Category: EventTool, ToolName: "bash", ToolArgs: `{"command":"date","intent":"Get current time"}`, ToolIntent: "Get current time"},
		{Category: EventObserve, ToolName: "bash", ToolResult: "Wed Sep 23 14:00:00 UTC", ToolIntent: "Get current time"},
		{Category: EventFinalAnswer, Content: "It's 2 PM"},
	}
	for _, ev := range events {
		a.writeAlignedEvent(ev)
	}
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 JSONL lines, got %d:\n%s", len(lines), buf.String())
	}

	var msgs [4]json_rpc.MessageEvent
	for i, line := range lines {
		env := parseEnvelope(t, line)
		if env.Kind != "message" {
			t.Errorf("line %d: envelope.kind=%q, want message", i, env.Kind)
		}
		if env.Version != 2 {
			t.Errorf("line %d: envelope.version=%d, want 2", i, env.Version)
		}
		if err := json.Unmarshal(env.Entry, &msgs[i]); err != nil {
			t.Fatalf("line %d: unmarshal entry: %v", i, err)
		}
	}

	if msgs[0].Message.Role != "user" {
		t.Errorf("msg0.role=%q, want user", msgs[0].Message.Role)
	}
	if msgs[1].Message.Role != "assistant" {
		t.Errorf("msg1.role=%q, want assistant", msgs[1].Message.Role)
	}
	if msgs[2].Message.Role != "toolResult" {
		t.Errorf("msg2.role=%q, want toolResult", msgs[2].Message.Role)
	}
	if msgs[3].Message.Role != "assistant" {
		t.Errorf("msg3.role=%q, want assistant", msgs[3].Message.Role)
	}

	if msgs[0].ParentID != "" {
		t.Errorf("msg0.parentId=%q, want empty", msgs[0].ParentID)
	}
	if msgs[1].ParentID != msgs[0].ID {
		t.Errorf("msg1.parentId=%q, want msg0.id=%q", msgs[1].ParentID, msgs[0].ID)
	}
	if msgs[2].ParentID != msgs[1].Message.Content[1].ID {
		t.Errorf("msg2.parentId=%q, want toolCall id=%q", msgs[2].ParentID, msgs[1].Message.Content[1].ID)
	}
	if msgs[3].ParentID != msgs[2].ID {
		t.Errorf("msg3.parentId=%q, want msg2.id=%q", msgs[3].ParentID, msgs[2].ID)
	}

	for i, m := range msgs {
		if !uuidV7Re.MatchString(m.ID) {
			t.Errorf("msg%d.id=%q not UUID v7", i, m.ID)
		}
	}

	if len(msgs[1].Message.Content) != 2 {
		t.Fatalf("msg1.content parts=%d, want 2", len(msgs[1].Message.Content))
	}
	if msgs[1].Message.Content[0].Type != "thinking" {
		t.Errorf("msg1.content[0].type=%q, want thinking", msgs[1].Message.Content[0].Type)
	}
	if msgs[1].Message.Content[0].Thinking != "The user wants time" {
		t.Errorf("msg1.content[0].thinking=%q, want %q", msgs[1].Message.Content[0].Thinking, "The user wants time")
	}
	if msgs[1].Message.Content[1].Type != "toolCall" {
		t.Errorf("msg1.content[1].type=%q, want toolCall", msgs[1].Message.Content[1].Type)
	}
	if msgs[1].Message.Content[1].Name != "bash" {
		t.Errorf("msg1.content[1].name=%q, want bash", msgs[1].Message.Content[1].Name)
	}
	if msgs[1].Message.Content[1].Intent != "Get current time" {
		t.Errorf("msg1.content[1].intent=%q, want Get current time", msgs[1].Message.Content[1].Intent)
	}
	if msgs[1].StopReason != "toolUse" {
		t.Errorf("msg1.stopReason=%q, want toolUse", msgs[1].StopReason)
	}

	if msgs[2].Details == nil {
		t.Fatal("msg2.details should be non-nil")
	}
	if msgs[2].Details.ToolName != "bash" {
		t.Errorf("msg2.details.toolName=%q, want bash", msgs[2].Details.ToolName)
	}
	if msgs[2].Details.Error != "" {
		t.Errorf("msg2.details.error=%q, want empty", msgs[2].Details.Error)
	}
	if msgs[2].StopReason != "toolUse" {
		t.Errorf("msg2.stopReason=%q, want toolUse", msgs[2].StopReason)
	}
	if len(msgs[2].Message.Content) != 1 || msgs[2].Message.Content[0].Text != "Wed Sep 23 14:00:00 UTC" {
		t.Errorf("msg2.content mismatch: %+v", msgs[2].Message.Content)
	}

	if msgs[3].StopReason != "end_turn" {
		t.Errorf("msg3.stopReason=%q, want end_turn", msgs[3].StopReason)
	}
	if len(msgs[3].Message.Content) != 1 || msgs[3].Message.Content[0].Text != "It's 2 PM" {
		t.Errorf("msg3.content mismatch: %+v", msgs[3].Message.Content)
	}
}

func TestWriteAlignedEventErrorEmitsMessageThenCustomToolError(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	a.writeAlignedEvent(Event{Category: EventUserMessage, Content: "hi"})
	a.writeAlignedEvent(Event{Category: EventThoughtStart})
	a.writeAlignedEvent(Event{Category: EventThoughtChunk, Reasoning: "thinking..."})
	a.writeAlignedEvent(Event{Category: EventFinalAnswer, Content: "answer"})
	a.writeAlignedEvent(Event{Category: EventError, ToolError: "boom"})
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 JSONL lines (user/assistant/custom), got %d:\n%s", len(lines), buf.String())
	}

	var msgEnv alignedEnvelope
	if err := json.Unmarshal([]byte(lines[0]), &msgEnv); err != nil {
		t.Fatalf("line 0 unmarshal: %v", err)
	}
	if msgEnv.Kind != "message" {
		t.Errorf("line 0 kind=%q, want message", msgEnv.Kind)
	}
	var userMsg json_rpc.MessageEvent
	json.Unmarshal(msgEnv.Entry, &userMsg)

	if err := json.Unmarshal([]byte(lines[1]), &msgEnv); err != nil {
		t.Fatalf("line 1 unmarshal: %v", err)
	}
	if msgEnv.Kind != "message" {
		t.Errorf("line 1 kind=%q, want message", msgEnv.Kind)
	}
	var assistantMsg json_rpc.MessageEvent
	json.Unmarshal(msgEnv.Entry, &assistantMsg)
	if assistantMsg.Message.Role != "assistant" {
		t.Errorf("line 1 role=%q, want assistant", assistantMsg.Message.Role)
	}
	if assistantMsg.StopReason != "end_turn" {
		t.Errorf("line 1 stopReason=%q, want end_turn", assistantMsg.StopReason)
	}

	if err := json.Unmarshal([]byte(lines[2]), &msgEnv); err != nil {
		t.Fatalf("line 2 unmarshal: %v", err)
	}
	if msgEnv.Kind != "custom" {
		t.Errorf("line 2 kind=%q, want custom", msgEnv.Kind)
	}
	var customEnv json_rpc.CustomEvent
	json.Unmarshal(msgEnv.Entry, &customEnv)
	if customEnv.CustomType != "tool_error" {
		t.Errorf("line 2 customType=%q, want tool_error", customEnv.CustomType)
	}
	if customEnv.ParentID != assistantMsg.ID {
		t.Errorf("line 2 parentId=%q, want assistant.id=%q", customEnv.ParentID, assistantMsg.ID)
	}
}

func TestWriteAlignedEventObserveErrorPopulatesToolErrorDetails(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	events := []Event{
		{Category: EventUserMessage, Content: "try"},
		{Category: EventThoughtStart},
		{Category: EventThoughtEnd},
		{Category: EventTool, ToolName: "bash", ToolArgs: `{"command":"false","intent":"fail"}`, ToolIntent: "fail"},
		{Category: EventObserve, ToolName: "bash", ToolError: "exit 1", ToolIntent: "fail"},
	}
	for _, ev := range events {
		a.writeAlignedEvent(ev)
	}
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 JSONL lines (user, assistant, toolResult), got %d:\n%s", len(lines), buf.String())
	}

	var toolResult json_rpc.MessageEvent
	env := parseEnvelope(t, lines[2])
	if err := json.Unmarshal(env.Entry, &toolResult); err != nil {
		t.Fatalf("line 2 unmarshal: %v", err)
	}
	if toolResult.Message.Role != "toolResult" {
		t.Errorf("line 2 role=%q, want toolResult", toolResult.Message.Role)
	}
	if toolResult.Details == nil {
		t.Fatal("line 2 details should be non-nil")
	}
	if toolResult.Details.Error != "exit 1" {
		t.Errorf("line 2 details.error=%q, want exit 1", toolResult.Details.Error)
	}
	if toolResult.Details.ToolName != "bash" {
		t.Errorf("line 2 details.toolName=%q, want bash", toolResult.Details.ToolName)
	}
	if len(toolResult.Message.Content) != 1 || toolResult.Message.Content[0].Text != "exit 1" {
		t.Errorf("line 2 content mismatch: %+v", toolResult.Message.Content)
	}
}

func TestLegacyWriteEventStillEmitsKindEvent(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	a.writeEvent(42, Event{Category: EventFinalAnswer, Content: "legacy answer"})
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}

	var legacy struct {
		Kind     string `json:"kind"`
		Seq      int    `json:"seq"`
		Category string `json:"category"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &legacy); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if legacy.Kind != "event" {
		t.Errorf("kind=%q, want event", legacy.Kind)
	}
	if legacy.Seq != 42 {
		t.Errorf("seq=%d, want 42", legacy.Seq)
	}
	if legacy.Category != "final_answer" {
		t.Errorf("category=%q, want final_answer", legacy.Category)
	}
	if legacy.Content != "legacy answer" {
		t.Errorf("content=%q, want legacy answer", legacy.Content)
	}
}

func TestWriteAlignedEventThoughtChunkWithoutStartIsNoop(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	a.writeAlignedEvent(Event{Category: EventThoughtChunk, Reasoning: "orphan chunk"})
	flushAgentLog(a)

	if got := buf.String(); got != "" {
		t.Errorf("expected empty output for orphan chunk, got %q", got)
	}
}

func TestWriteMessageSetsTimestampWhenMissing(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	msg := json_rpc.MessageEvent{
		ID:      json_rpc.NewV7(),
		Message: json_rpc.Message{Role: "user", Content: []json_rpc.MessageContentPart{{Type: "text", Text: "hi"}}},
	}
	if err := a.WriteMessage(msg); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	flushAgentLog(a)

	var env alignedEnvelope
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var got json_rpc.MessageEvent
	if err := json.Unmarshal(env.Entry, &got); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if got.Timestamp == "" {
		t.Error("WriteMessage should populate timestamp when caller left it empty")
	}
}

func TestWriteAlignedEventPropagatesUsageStatsToAssistantMessage(t *testing.T) {
	a, buf := newTestAgentWithBuf()

	usage := &llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}
	a.writeAlignedEvent(Event{Category: EventUserMessage, Content: "q"})
	a.writeAlignedEvent(Event{Category: EventThoughtStart})
	a.writeAlignedEvent(Event{Category: EventThoughtChunk, Reasoning: "think"})
	a.writeAlignedEvent(Event{Category: EventThoughtEnd, Usage: usage})
	a.writeAlignedEvent(Event{Category: EventFinalAnswer, Content: "a", Usage: usage})
	flushAgentLog(a)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", len(lines))
	}

	var assistantMsg json_rpc.MessageEvent
	env := parseEnvelope(t, lines[1])
	if err := json.Unmarshal(env.Entry, &assistantMsg); err != nil {
		t.Fatalf("unmarshal assistant entry: %v", err)
	}
	if assistantMsg.Usage == nil {
		t.Fatal("assistant.usage should be non-nil")
	}
	if assistantMsg.Usage.TotalTokens != 150 {
		t.Errorf("assistant.usage.totalTokens=%d, want 150", assistantMsg.Usage.TotalTokens)
	}
	if assistantMsg.Usage.PromptTokens != 100 {
		t.Errorf("assistant.usage.promptTokens=%d, want 100", assistantMsg.Usage.PromptTokens)
	}
}

var _ = regexp.MustCompile
