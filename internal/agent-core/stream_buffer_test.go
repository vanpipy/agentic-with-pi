package agentcore

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/vanpiyp/awp/internal/agent-protocol/json_rpc"
)

var uuidV7Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestStreamBufferThinkingChunksConcat(t *testing.T) {
	b := NewStreamBuffer("")
	b.AppendThinking("Hello", "")
	b.AppendThinking(" world", "")
	b.AppendThinking("!", "")
	msg := b.Finalize()
	if len(msg.Message.Content) != 1 {
		t.Fatalf("expected 1 part, got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0].Type != "thinking" {
		t.Errorf("expected type=thinking, got %q", msg.Message.Content[0].Type)
	}
	if msg.Message.Content[0].Thinking != "Hello world!" {
		t.Errorf("expected concatenated thinking text %q, got %q", "Hello world!", msg.Message.Content[0].Thinking)
	}
}

func TestStreamBufferTextChunksConcat(t *testing.T) {
	b := NewStreamBuffer("")
	b.AppendText("a")
	b.AppendText("b")
	msg := b.Finalize()
	if len(msg.Message.Content) != 1 {
		t.Fatalf("expected 1 part, got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0].Type != "text" {
		t.Errorf("expected type=text, got %q", msg.Message.Content[0].Type)
	}
	if msg.Message.Content[0].Text != "ab" {
		t.Errorf("expected concatenated text %q, got %q", "ab", msg.Message.Content[0].Text)
	}
}

func TestStreamBufferThinkingAndTextAreSeparate(t *testing.T) {
	b := NewStreamBuffer("")
	b.AppendThinking("think", "")
	b.AppendText("text")
	msg := b.Finalize()
	if len(msg.Message.Content) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0].Type != "thinking" {
		t.Errorf("first part should be thinking, got %q", msg.Message.Content[0].Type)
	}
	if msg.Message.Content[1].Type != "text" {
		t.Errorf("second part should be text, got %q", msg.Message.Content[1].Type)
	}
}

func TestStreamBufferToolCallIsSeparatePart(t *testing.T) {
	b := NewStreamBuffer("")
	b.AppendText("I'll use a tool")
	b.AppendToolCall("call-abc", "bash", "run date", json.RawMessage(`{"command":"date"}`))
	msg := b.Finalize()
	if len(msg.Message.Content) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[1].Type != "toolCall" {
		t.Errorf("second part should be toolCall, got %q", msg.Message.Content[1].Type)
	}
	if msg.Message.Content[1].ID != "call-abc" {
		t.Errorf("expected id call-abc, got %q", msg.Message.Content[1].ID)
	}
	if msg.Message.Content[1].Name != "bash" {
		t.Errorf("expected name bash, got %q", msg.Message.Content[1].Name)
	}
	if msg.Message.Content[1].Intent != "run date" {
		t.Errorf("expected intent run date, got %q", msg.Message.Content[1].Intent)
	}
	if string(msg.Message.Content[1].Arguments) != `{"command":"date"}` {
		t.Errorf("expected arguments json, got %q", string(msg.Message.Content[1].Arguments))
	}
}

func TestStreamBufferParentIDPropagates(t *testing.T) {
	b := NewStreamBuffer("parent-uuid-v7")
	msg := b.Finalize()
	if msg.ParentID != "parent-uuid-v7" {
		t.Errorf("expected parent propagation %q, got %q", "parent-uuid-v7", msg.ParentID)
	}
}

func TestStreamBufferParentAccessorReturnsStoredID(t *testing.T) {
	b := NewStreamBuffer("parent-xyz")
	if got := b.ParentID(); got != "parent-xyz" {
		t.Errorf("ParentID() = %q, want parent-xyz", got)
	}
}

func TestStreamBufferFinalizeGeneratesUUIDv7(t *testing.T) {
	b := NewStreamBuffer("")
	msg := b.Finalize()
	if !uuidV7Re.MatchString(msg.ID) {
		t.Errorf("expected UUID v7 format, got %q", msg.ID)
	}
}

func TestStreamBufferFinalizeSetsTimestamp(t *testing.T) {
	b := NewStreamBuffer("")
	msg := b.Finalize()
	if msg.Timestamp == "" {
		t.Fatal("expected non-empty timestamp")
	}
	if !strings.Contains(msg.Timestamp, "T") {
		t.Errorf("expected RFC3339 with T separator, got %q", msg.Timestamp)
	}
	if !strings.HasSuffix(msg.Timestamp, "Z") {
		t.Errorf("expected UTC Z suffix, got %q", msg.Timestamp)
	}
}

func TestStreamBufferRoleIsEmptyByDefault(t *testing.T) {
	b := NewStreamBuffer("")
	msg := b.Finalize()
	if msg.Message.Role != "" {
		t.Errorf("expected empty role by default, got %q", msg.Message.Role)
	}
}

func TestStreamBufferSetRolePropagates(t *testing.T) {
	b := NewStreamBuffer("")
	b.SetRole("assistant")
	msg := b.Finalize()
	if msg.Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %q", msg.Message.Role)
	}
}

func TestStreamBufferUsageAndStopReasonAndDetails(t *testing.T) {
	b := NewStreamBuffer("")
	b.SetUsage(json_rpc.UsageStats{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})
	b.SetStopReason("end_turn")
	b.SetDetails(json_rpc.MessageDetails{ToolName: "bash"})
	msg := b.Finalize()
	if msg.Usage == nil {
		t.Fatal("expected Usage to be set")
	}
	if msg.Usage.TotalTokens != 15 {
		t.Errorf("expected TotalTokens 15, got %d", msg.Usage.TotalTokens)
	}
	if msg.StopReason != "end_turn" {
		t.Errorf("expected stopReason end_turn, got %q", msg.StopReason)
	}
	if msg.Details == nil {
		t.Fatal("expected Details to be set")
	}
	if msg.Details.ToolName != "bash" {
		t.Errorf("expected details.toolName bash, got %q", msg.Details.ToolName)
	}
}

func TestStreamBufferThinkingSignaturePropagates(t *testing.T) {
	b := NewStreamBuffer("")
	b.AppendThinking("reasoning", "")
	b.AppendThinking(" more", "sig-xyz")
	msg := b.Finalize()
	if len(msg.Message.Content) != 1 {
		t.Fatalf("expected 1 part, got %d", len(msg.Message.Content))
	}
	if msg.Message.Content[0].Thinking != "reasoning more" {
		t.Errorf("expected concatenated thinking, got %q", msg.Message.Content[0].Thinking)
	}
	if msg.Message.Content[0].ThinkingSignature != "sig-xyz" {
		t.Errorf("expected signature sig-xyz, got %q", msg.Message.Content[0].ThinkingSignature)
	}
}

func TestStreamBufferEmptyAppendIsNoop(t *testing.T) {
	b := NewStreamBuffer("")
	b.AppendThinking("", "")
	b.AppendText("")
	msg := b.Finalize()
	if len(msg.Message.Content) != 0 {
		t.Errorf("expected no content parts, got %d", len(msg.Message.Content))
	}
}

func TestStreamBufferMonotonicIDsAcrossBuffers(t *testing.T) {
	const N = 100
	ids := make([]string, N)
	for i := range ids {
		ids[i] = NewStreamBuffer("").Finalize().ID
	}
	seen := make(map[string]bool, N)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("UUID collision on %s", id)
		}
		seen[id] = true
	}
}

func TestStreamBufferConcurrentAppendIsRaceFree(t *testing.T) {
	b := NewStreamBuffer("")
	const goroutines = 8
	const perGoroutine = 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				b.AppendText("x")
			}
		}()
	}
	wg.Wait()
	msg := b.Finalize()
	if len(msg.Message.Content) != 1 {
		t.Fatalf("expected 1 part, got %d", len(msg.Message.Content))
	}
	if got, want := msg.Message.Content[0].Text, strings.Repeat("x", goroutines*perGoroutine); got != want {
		t.Errorf("expected %d chars, got %d", len(want), len(got))
	}
}

func TestStreamBufferFinalizeIsIdempotent(t *testing.T) {
	b := NewStreamBuffer("parent-1")
	b.AppendText("hello")
	first := b.Finalize()
	if first.ID == "" {
		t.Fatal("first Finalize produced empty id")
	}
	second := b.Finalize()
	if second.ID != first.ID {
		t.Errorf("expected idempotent id, got %q then %q", first.ID, second.ID)
	}
	if second.ParentID != first.ParentID {
		t.Errorf("expected idempotent parentId, got %q then %q", first.ParentID, second.ParentID)
	}
}
