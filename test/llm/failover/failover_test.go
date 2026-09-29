package failover_test

import (
	"strings"
	"testing"

	"github.com/vanpiyp/awp/internal/llm/failover"
)

func TestPromptRoundtripsFromErrorMessage(t *testing.T) {
	original := failover.ProviderFailoverPrompt{
		FromProvider:         "claude",
		FromLabel:            "Anthropic",
		ToProvider:           "openai",
		ToLabel:              "OpenAI",
		Reason:               "rate limit",
		EstimatedInputChars:  1200,
		EstimatedInputTokens: 300,
	}
	message := original.ErrorMessage()
	parsed := failover.ParseFailoverPromptMessage(message)
	if parsed == nil {
		t.Fatalf("ParseFailoverPromptMessage returned nil for: %q", message)
	}
	if *parsed != original {
		t.Fatalf("parsed = %+v, want %+v", *parsed, original)
	}
}

func TestParseFailoverPromptMessage_RejectsMissingPrefix(t *testing.T) {
	if p := failover.ParseFailoverPromptMessage("just a plain error"); p != nil {
		t.Fatalf("expected nil, got %+v", p)
	}
}

func TestParseFailoverPromptMessage_RejectsEmpty(t *testing.T) {
	if p := failover.ParseFailoverPromptMessage(""); p != nil {
		t.Fatalf("expected nil, got %+v", p)
	}
}

func TestParseFailoverPromptMessage_RejectsInvalidJSON(t *testing.T) {
	if p := failover.ParseFailoverPromptMessage(failover.PromptPrefix + "not-json"); p != nil {
		t.Fatalf("expected nil, got %+v", p)
	}
}

func TestParseFailoverPromptMessage_ToleratesTrailingWhitespace(t *testing.T) {
	msg := "  " + failover.PromptPrefix + `{"from_provider":"a","from_label":"A","to_provider":"b","to_label":"B","reason":"x","estimated_input_chars":0,"estimated_input_tokens":0}` + "\n" + "trailing explanation"
	if p := failover.ParseFailoverPromptMessage(msg); p == nil {
		t.Fatalf("expected non-nil for trimmed prefix")
	}
}

func TestErrorMessage_ContainsHumanReadableLine(t *testing.T) {
	p := failover.ProviderFailoverPrompt{
		FromProvider: "claude", FromLabel: "Anthropic",
		ToProvider: "openai", ToLabel: "OpenAI",
		Reason: "rate limit", EstimatedInputChars: 100, EstimatedInputTokens: 25,
	}
	msg := p.ErrorMessage()
	if !strings.Contains(msg, "Anthropic is unavailable") {
		t.Fatalf("missing human-readable line: %q", msg)
	}
	if !strings.Contains(msg, "switching to OpenAI") {
		t.Fatalf("missing to_label: %q", msg)
	}
	if !strings.Contains(msg, "25 input tokens") || !strings.Contains(msg, "~100 chars") {
		t.Fatalf("missing token counts: %q", msg)
	}
	if !strings.HasPrefix(msg, failover.PromptPrefix) {
		t.Fatalf("first line must start with prefix: %q", msg)
	}
}

func TestDecision_String(t *testing.T) {
	cases := []struct {
		d    failover.Decision
		want string
	}{
		{failover.DecisionNone, "none"},
		{failover.DecisionRetryNextProvider, "retry-next-provider"},
		{failover.DecisionRetryAndMarkUnavailable, "retry-and-mark-unavailable"},
	}
	for _, c := range cases {
		if got := c.d.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestDecision_ShouldFailover(t *testing.T) {
	cases := []struct {
		d    failover.Decision
		want bool
	}{
		{failover.DecisionNone, false},
		{failover.DecisionRetryNextProvider, true},
		{failover.DecisionRetryAndMarkUnavailable, true},
	}
	for _, c := range cases {
		if got := c.d.ShouldFailover(); got != c.want {
			t.Errorf("%d.ShouldFailover() = %v, want %v", c.d, got, c.want)
		}
	}
}

func TestDecision_ShouldMarkProviderUnavailable(t *testing.T) {
	if failover.DecisionNone.ShouldMarkProviderUnavailable() {
		t.Errorf("None should not mark unavailable")
	}
	if failover.DecisionRetryNextProvider.ShouldMarkProviderUnavailable() {
		t.Errorf("RetryNextProvider should not mark unavailable")
	}
	if !failover.DecisionRetryAndMarkUnavailable.ShouldMarkProviderUnavailable() {
		t.Errorf("RetryAndMarkUnavailable should mark unavailable")
	}
}

func TestClassifyErrorMessage_RateLimitsMarkUnavailable(t *testing.T) {
	if got := failover.ClassifyErrorMessage("429 Too Many Requests"); got != failover.DecisionRetryAndMarkUnavailable {
		t.Fatalf("got %v, want RetryAndMarkUnavailable", got)
	}
	if got := failover.ClassifyErrorMessage("rate limit exceeded"); got != failover.DecisionRetryAndMarkUnavailable {
		t.Fatalf("got %v, want RetryAndMarkUnavailable", got)
	}
	if got := failover.ClassifyErrorMessage("402 payment required"); got != failover.DecisionRetryAndMarkUnavailable {
		t.Fatalf("got %v, want RetryAndMarkUnavailable", got)
	}
}

func TestClassifyErrorMessage_ContextRetriesWithoutMarkingUnavailable(t *testing.T) {
	if got := failover.ClassifyErrorMessage("context length exceeded"); got != failover.DecisionRetryNextProvider {
		t.Fatalf("got %v, want RetryNextProvider", got)
	}
	if got := failover.ClassifyErrorMessage("413 Payload Too Large"); got != failover.DecisionRetryNextProvider {
		t.Fatalf("got %v, want RetryNextProvider", got)
	}
	if got := failover.ClassifyErrorMessage("prompt is too long"); got != failover.DecisionRetryNextProvider {
		t.Fatalf("got %v, want RetryNextProvider", got)
	}
}

func TestClassifyErrorMessage_AuthMarksUnavailable(t *testing.T) {
	cases := []string{
		"401 Unauthorized",
		"403 Forbidden",
		"authentication failed",
		"provider unavailable",
		"credentials are not configured",
		"access denied",
	}
	for _, msg := range cases {
		if got := failover.ClassifyErrorMessage(msg); got != failover.DecisionRetryAndMarkUnavailable {
			t.Errorf("ClassifyErrorMessage(%q) = %v, want RetryAndMarkUnavailable", msg, got)
		}
	}
}

func TestClassifyErrorMessage_IgnoresEmbeddedStatusDigits(t *testing.T) {
	if got := failover.ClassifyErrorMessage("model version 4130 failed"); got != failover.DecisionNone {
		t.Fatalf("got %v, want None (4130 is not a standalone status)", got)
	}
	if got := failover.ClassifyErrorMessage("request id 14130 timed out"); got != failover.DecisionNone {
		t.Fatalf("got %v, want None (14130 contains 413 but not standalone)", got)
	}
}

func TestClassifyErrorMessage_StandaloneStatusCounts(t *testing.T) {
	if got := failover.ClassifyErrorMessage("HTTP/1.1 413 payload too large"); got != failover.DecisionRetryNextProvider {
		t.Fatalf("got %v, want RetryNextProvider", got)
	}
	if got := failover.ClassifyErrorMessage("status: 429, retry after 5s"); got != failover.DecisionRetryAndMarkUnavailable {
		t.Fatalf("got %v, want RetryAndMarkUnavailable", got)
	}
}

func TestClassifyErrorMessage_ContextBeatsRate(t *testing.T) {
	if got := failover.ClassifyErrorMessage("context length exceeded; 429 retry after 5s"); got != failover.DecisionRetryNextProvider {
		t.Fatalf("got %v, want RetryNextProvider (context wins)", got)
	}
}

func TestClassifyErrorMessage_EmptyReturnsNone(t *testing.T) {
	if got := failover.ClassifyErrorMessage(""); got != failover.DecisionNone {
		t.Fatalf("got %v, want None", got)
	}
}

func TestClassifyErrorMessage_RandomErrorReturnsNone(t *testing.T) {
	if got := failover.ClassifyErrorMessage("the server is on fire"); got != failover.DecisionNone {
		t.Fatalf("got %v, want None", got)
	}
}

func TestClassifyErrorMessage_IsCaseInsensitive(t *testing.T) {
	if got := failover.ClassifyErrorMessage("RATE LIMIT EXCEEDED"); got != failover.DecisionRetryAndMarkUnavailable {
		t.Fatalf("got %v, want RetryAndMarkUnavailable", got)
	}
	if got := failover.ClassifyErrorMessage("CONTEXT LENGTH EXCEEDED"); got != failover.DecisionRetryNextProvider {
		t.Fatalf("got %v, want RetryNextProvider", got)
	}
}
