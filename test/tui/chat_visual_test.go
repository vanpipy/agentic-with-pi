package tui_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/tui"
)

func TestAppendReasoningRendersLive(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendReasoningForTest("hello")
	out := c.ContentForTest()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected live reasoning output to include %q, got:\n%s", "hello", out)
	}
}

func TestAppendReasoningEmptyNoFlicker(t *testing.T) {
	c := tui.NewChatModelForTest()
	before := c.RefreshCountForTest()
	c.AppendReasoningForTest("")
	c.AppendReasoningForTest("")
	after := c.RefreshCountForTest()
	if after != before {
		t.Errorf("appendReasoning with empty text incremented refreshCount: before=%d after=%d", before, after)
	}
}

func TestCommitStreamFlushesReasoningToChatMsg(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendReasoningForTest("thought A")
	c.CommitStreamForTest()
	msgs := c.MessagesForTest()
	if len(msgs) == 0 {
		t.Fatalf("expected at least one chatMsg after commitStream, got 0")
	}
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleThinking {
		t.Errorf("last chatMsg role = %v, want roleThinking", last.Role)
	}
	if last.Text != "thought A" {
		t.Errorf("last chatMsg text = %q, want %q", last.Text, "thought A")
	}
	if c.ReasoningLenForTest() != 0 {
		t.Errorf("reasoning buffer non-empty after commit: len=%d", c.ReasoningLenForTest())
	}
}

func TestDiscardStreamClearsReasoning(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendReasoningForTest("thought B")
	if c.ReasoningLenForTest() == 0 {
		t.Fatalf("expected reasoning buffer to be populated before discard")
	}
	before := len(c.MessagesForTest())
	c.DiscardStreamForTest()
	if c.ReasoningLenForTest() != 0 {
		t.Errorf("reasoning buffer non-empty after discard: len=%d", c.ReasoningLenForTest())
	}
	if c.StreamingLenForTest() != 0 {
		t.Errorf("streaming buffer non-empty after discard: len=%d", c.StreamingLenForTest())
	}
	if len(c.MessagesForTest()) != before {
		t.Errorf("discardStream should not add chatMsg; before=%d after=%d", before, len(c.MessagesForTest()))
	}
}

func TestLiveReasoningCollapseWhenLong(t *testing.T) {
	c := tui.NewChatModelForTest()
	long := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11"
	c.AppendReasoningForTest(long)
	out := c.ContentForTest()
	if !strings.Contains(out, "more lines") {
		t.Errorf("expected collapse summary line in output, got:\n%s", out)
	}
	if strings.Contains(out, "line1\nline2\nline3\nline4\nline5\nline6\nline7") {
		t.Errorf("expected oldest lines to be collapsed (hidden), but they appear in output:\n%s", out)
	}
	if !strings.Contains(out, "line9") && !strings.Contains(out, "line10") && !strings.Contains(out, "line11") {
		t.Errorf("expected most-recent lines to remain visible, got:\n%s", out)
	}
}

func TestCommitStreamFullReasoningNotCollapsed(t *testing.T) {
	c := tui.NewChatModelForTest()
	long := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11"
	c.AppendReasoningForTest(long)
	c.CommitStreamForTest()
	msgs := c.MessagesForTest()
	if len(msgs) == 0 {
		t.Fatalf("expected chatMsg after commit, got 0")
	}
	last := msgs[len(msgs)-1]
	if last.Role != tui.RoleThinking {
		t.Fatalf("last chatMsg role = %v, want roleThinking", last.Role)
	}
	if last.Text != long {
		t.Errorf("last chatMsg text length=%d, want full length=%d; got %q", len(last.Text), len(long), last.Text)
	}
}

func TestReasoningAndStreamingCoexist(t *testing.T) {
	c := tui.NewChatModelForTest()
	c.AppendReasoningForTest("thinking now")
	c.AppendStreamForTest("responding now")
	out := c.ContentForTest()
	if !strings.Contains(out, "thinking now") {
		t.Errorf("expected live reasoning text in output, got:\n%s", out)
	}
	if !strings.Contains(out, "responding now") {
		t.Errorf("expected live streaming text in output, got:\n%s", out)
	}
	idxReason := strings.Index(out, "thinking now")
	idxStream := strings.Index(out, "responding now")
	if idxReason < 0 || idxStream < 0 {
		t.Fatalf("missing one of the live buffers in output")
	}
	if idxReason > idxStream {
		t.Errorf("expected reasoning to render before streaming; reasoning idx=%d streaming idx=%d", idxReason, idxStream)
	}
}
