package llm_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestStreamEventSealedInterface(t *testing.T) {
	var ev llm.StreamEvent
	ev = llm.EventTextDelta{Text: "hi"}
	ev = llm.EventThinkingStart{}
	ev = llm.EventThinkingDelta{Text: "thinking"}
	ev = llm.EventThinkingEnd{}
	ev = llm.EventThinkingSignature{Signature: "sig"}
	ev = llm.EventToolStart{ID: "1", Name: "bash"}
	ev = llm.EventToolDelta{ID: "1", JSON: `{"cmd`}
	ev = llm.EventToolEnd{ID: "1"}
	ev = llm.EventToolUseSignature{ID: "1", Signature: "gsig"}
	ev = llm.EventUsage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 80, CacheCreationTokens: 20}
	ev = llm.EventCompaction{Trigger: "auto", PreTokens: 50000, Native: false}
	ev = llm.EventRetryRollback{Attempt: 2, Max: 5}
	ev = llm.EventFinish{Reason: llm.FinishReasonStop}
	ev = llm.EventErr{Err: nil, RetryAfterSecs: 0}
	ev = llm.EventSessionID{ID: "sess-1"}
	_ = ev
}

func TestEventTextDeltaPayload(t *testing.T) {
	ev := llm.EventTextDelta{Text: "hello world"}
	if ev.Text != "hello world" {
		t.Fatalf("Text = %q, want %q", ev.Text, "hello world")
	}
}

func TestEventThinkingStartPayload(t *testing.T) {
	ev := llm.EventThinkingStart{}
	_ = ev
}

func TestEventThinkingDeltaPayload(t *testing.T) {
	ev := llm.EventThinkingDelta{Text: "deep thought"}
	if ev.Text != "deep thought" {
		t.Fatalf("Text = %q, want %q", ev.Text, "deep thought")
	}
}

func TestEventThinkingEndPayload(t *testing.T) {
	ev := llm.EventThinkingEnd{}
	_ = ev
}

func TestEventThinkingSignaturePayload(t *testing.T) {
	ev := llm.EventThinkingSignature{Signature: "abc"}
	if ev.Signature != "abc" {
		t.Fatalf("Signature = %q, want %q", ev.Signature, "abc")
	}
}

func TestEventToolStartPayload(t *testing.T) {
	ev := llm.EventToolStart{ID: "tool-1", Name: "bash"}
	if ev.ID != "tool-1" {
		t.Fatalf("ID = %q, want %q", ev.ID, "tool-1")
	}
	if ev.Name != "bash" {
		t.Fatalf("Name = %q, want %q", ev.Name, "bash")
	}
}

func TestEventToolDeltaPayload(t *testing.T) {
	ev := llm.EventToolDelta{ID: "tool-1", JSON: `{"cmd":"ls"}`}
	if ev.ID != "tool-1" {
		t.Fatalf("ID = %q, want %q", ev.ID, "tool-1")
	}
	if ev.JSON != `{"cmd":"ls"}` {
		t.Fatalf("JSON = %q, want %q", ev.JSON, `{"cmd":"ls"}`)
	}
}

func TestEventToolEndPayload(t *testing.T) {
	ev := llm.EventToolEnd{ID: "tool-1"}
	if ev.ID != "tool-1" {
		t.Fatalf("ID = %q, want %q", ev.ID, "tool-1")
	}
}

func TestEventToolUseSignaturePayload(t *testing.T) {
	ev := llm.EventToolUseSignature{ID: "tool-1", Signature: "gsig"}
	if ev.ID != "tool-1" {
		t.Fatalf("ID = %q, want %q", ev.ID, "tool-1")
	}
	if ev.Signature != "gsig" {
		t.Fatalf("Signature = %q, want %q", ev.Signature, "gsig")
	}
}

func TestEventUsagePayload(t *testing.T) {
	ev := llm.EventUsage{
		InputTokens:         100,
		OutputTokens:        50,
		CacheReadTokens:     80,
		CacheCreationTokens: 20,
	}
	if ev.InputTokens != 100 {
		t.Fatalf("InputTokens = %d, want 100", ev.InputTokens)
	}
	if ev.OutputTokens != 50 {
		t.Fatalf("OutputTokens = %d, want 50", ev.OutputTokens)
	}
	if ev.CacheReadTokens != 80 {
		t.Fatalf("CacheReadTokens = %d, want 80", ev.CacheReadTokens)
	}
	if ev.CacheCreationTokens != 20 {
		t.Fatalf("CacheCreationTokens = %d, want 20", ev.CacheCreationTokens)
	}
}

func TestEventCompactionPayload(t *testing.T) {
	ev := llm.EventCompaction{Trigger: "auto", PreTokens: 50000, Native: true}
	if ev.Trigger != "auto" {
		t.Fatalf("Trigger = %q, want %q", ev.Trigger, "auto")
	}
	if ev.PreTokens != 50000 {
		t.Fatalf("PreTokens = %d, want 50000", ev.PreTokens)
	}
	if !ev.Native {
		t.Fatalf("Native = false, want true")
	}
}

func TestEventRetryRollbackPayload(t *testing.T) {
	ev := llm.EventRetryRollback{Attempt: 2, Max: 5}
	if ev.Attempt != 2 {
		t.Fatalf("Attempt = %d, want 2", ev.Attempt)
	}
	if ev.Max != 5 {
		t.Fatalf("Max = %d, want 5", ev.Max)
	}
}

func TestEventFinishPayload(t *testing.T) {
	ev := llm.EventFinish{Reason: llm.FinishReasonStop}
	if ev.Reason != llm.FinishReasonStop {
		t.Fatalf("Reason = %v, want %v", ev.Reason, llm.FinishReasonStop)
	}
}

func TestEventErrPayload(t *testing.T) {
	ev := llm.EventErr{Err: nil, RetryAfterSecs: 30}
	if ev.RetryAfterSecs != 30 {
		t.Fatalf("RetryAfterSecs = %d, want 30", ev.RetryAfterSecs)
	}
	if ev.Err != nil {
		t.Fatalf("Err = %v, want nil", ev.Err)
	}
}

func TestEventSessionIDPayload(t *testing.T) {
	ev := llm.EventSessionID{ID: "sess-1"}
	if ev.ID != "sess-1" {
		t.Fatalf("ID = %q, want %q", ev.ID, "sess-1")
	}
}

func TestStreamEventVariantCount(t *testing.T) {
	cases := []llm.StreamEvent{
		llm.EventTextDelta{},
		llm.EventThinkingStart{},
		llm.EventThinkingDelta{},
		llm.EventThinkingEnd{},
		llm.EventThinkingSignature{},
		llm.EventToolStart{},
		llm.EventToolDelta{},
		llm.EventToolEnd{},
		llm.EventToolUseSignature{},
		llm.EventUsage{},
		llm.EventCompaction{},
		llm.EventRetryRollback{},
		llm.EventFinish{},
		llm.EventErr{},
		llm.EventSessionID{},
	}
	if len(cases) != 15 {
		t.Fatalf("variant count = %d, want 15", len(cases))
	}
}
