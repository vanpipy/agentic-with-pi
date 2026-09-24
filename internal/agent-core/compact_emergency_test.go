package agentcore

import (
	"errors"
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestEmergencyTruncateToolResultsTableDriven(t *testing.T) {
	const maxChars = 100
	cases := []struct {
		name        string
		msgs        []llm.Message
		wantTrunc   bool
		wantMarker  string
		wantKeptLen int
	}{
		{
			name: "short_tool_passthrough",
			msgs: []llm.Message{
				{Role: "tool", ToolCallID: "c1", Content: "short result"},
			},
			wantTrunc:   false,
			wantKeptLen: len("short result"),
		},
		{
			name: "long_tool_truncated_with_marker",
			msgs: []llm.Message{
				{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("a", 5000)},
			},
			wantTrunc:   true,
			wantMarker:  "emergency truncated",
			wantKeptLen: maxChars,
		},
		{
			name: "non_tool_passthrough",
			msgs: []llm.Message{
				{Role: "user", Content: strings.Repeat("u", 5000)},
			},
			wantTrunc:   false,
			wantKeptLen: 5000,
		},
		{
			name: "exactly_max_passthrough",
			msgs: []llm.Message{
				{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("a", 100)},
			},
			wantTrunc:   false,
			wantKeptLen: 100,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := emergencyTruncateToolResults(tc.msgs, maxChars)
			if len(out) != len(tc.msgs) {
				t.Fatalf("returned %d msgs, want %d", len(out), len(tc.msgs))
			}
			got := out[0].Content
			if tc.wantTrunc {
				if !strings.Contains(got, tc.wantMarker) {
					t.Errorf("missing marker %q in %q", tc.wantMarker, got)
				}
				prefix := strings.Repeat("a", maxChars)
				if !strings.HasPrefix(got, prefix) {
					t.Errorf("missing kept prefix; got prefix=%q", got[:30])
				}
			} else {
				if tc.wantMarker != "" && strings.Contains(got, tc.wantMarker) {
					t.Errorf("unexpected marker %q in %q", tc.wantMarker, got)
				}
			}
		})
	}
}

func TestEmergencyTruncateDoesNotMutateInput(t *testing.T) {
	msgs := []llm.Message{
		{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("a", 5000)},
	}
	originalContent := msgs[0].Content
	_ = emergencyTruncateToolResults(msgs, 100)
	if msgs[0].Content != originalContent {
		t.Errorf("input slice mutated: original len %d, now %d", len(originalContent), len(msgs[0].Content))
	}
}

func TestEmergencyTruncatedToolResultFields(t *testing.T) {
	r := emergencyTruncatedToolResult{
		OriginalLen: 5000,
		KeptLen:     100,
		Marker:      "emergency truncated",
	}
	if r.OriginalLen != 5000 {
		t.Errorf("OriginalLen = %d, want 5000", r.OriginalLen)
	}
	if r.KeptLen != 100 {
		t.Errorf("KeptLen = %d, want 100", r.KeptLen)
	}
	if r.Marker == "" {
		t.Error("Marker empty")
	}
}

func TestIsRequestPayloadTooLargeErrorTableDriven(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil_error", nil, false},
		{"plain_error", errors.New("something else"), false},
		{"explicit_payload_too_large", errors.New("Error: request payload too large"), true},
		{"413_status_code", errors.New("HTTP 413 Request Entity Too Large"), true},
		{"context_length_exceeded", errors.New("openai: context_length_exceeded: 8192 > 4096"), true},
		{"lowercase_payload_too_large", errors.New("request payload too large; please reduce content"), true},
		{"unrelated_500", errors.New("server error: 500"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isRequestPayloadTooLargeError(tc.err)
			if got != tc.want {
				t.Errorf("got %v, want %v for %q", got, tc.want, tc.err)
			}
		})
	}
}

func TestStripOversizedToolResultsInMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("a", 5000)},
		{Role: "assistant", Content: "small"},
		{Role: "tool", ToolCallID: "c2", Content: strings.Repeat("b", 5000)},
		{Role: "user", Content: "follow up"},
	}

	out := stripOversizedToolResultsInMessages(msgs, 1000)
	if len(out) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(out), out)
	}
	if out[0].Role != "user" || out[0].Content != "hi" {
		t.Errorf("out[0] = %+v, want user/hi", out[0])
	}
	if out[1].Role != "assistant" || out[1].Content != "small" {
		t.Errorf("out[1] = %+v, want assistant/small", out[1])
	}
	if out[2].Role != "user" || out[2].Content != "follow up" {
		t.Errorf("out[2] = %+v, want user/follow up", out[2])
	}
}

func TestStripOversizedToolResultsKeepsSmall(t *testing.T) {
	msgs := []llm.Message{
		{Role: "tool", ToolCallID: "c1", Content: "small"},
		{Role: "user", Content: "u"},
	}
	got := stripOversizedToolResultsInMessages(msgs, 1000)
	if len(got) != 2 {
		t.Errorf("got %d, want 2 (none should be stripped)", len(got))
	}
}

func TestEmergencyHelpersDoesNotPanicOnEmpty(t *testing.T) {
	out := emergencyTruncateToolResults(nil, 100)
	if len(out) != 0 {
		t.Errorf("expected empty result for nil input, got %d msgs", len(out))
	}
	out = stripOversizedToolResultsInMessages(nil, 100)
	if len(out) != 0 {
		t.Errorf("strip nil: got %d, want 0", len(out))
	}
	if isRequestPayloadTooLargeError(nil) {
		t.Error("nil error should not be payload too large")
	}
}

func TestEmergencyTruncateProducesResultType(t *testing.T) {
	msgs := []llm.Message{
		{Role: "tool", ToolCallID: "c1", Content: strings.Repeat("x", 5000)},
	}
	out := emergencyTruncateToolResults(msgs, 100)
	if out[0].Role != "tool" {
		t.Errorf("role changed: %q", out[0].Role)
	}
	if out[0].ToolCallID != "c1" {
		t.Errorf("ToolCallID changed: %q", out[0].ToolCallID)
	}
	if out[0].Content == msgs[0].Content {
		t.Error("content was not truncated")
	}
	if !strings.Contains(out[0].Content, "emergency") {
		t.Errorf("missing emergency marker: %q", out[0].Content[:120])
	}
	if len(out) != 1 {
		t.Errorf("len mismatch: %d", len(out))
	}
	_ = llm.Message{}
}
