package failover

import (
	"encoding/json"
	"strconv"
	"strings"
)

// PromptPrefix marks an error message that contains a serialized
// ProviderFailoverPrompt. The prefix plus a JSON payload on the first line
// is parsed by ParseFailoverPromptMessage. Mirrors
// PROVIDER_FAILOVER_PROMPT_PREFIX from jcode failover.rs:3.
const PromptPrefix = "[jcode-provider-failover]"

// ProviderFailoverPrompt is the structured payload sent in an error message
// when the retry loop wants the UI to offer a one-key switch to a different
// provider. Mirrors jcode failover.rs:6-14.
type ProviderFailoverPrompt struct {
	FromProvider         string `json:"from_provider"`
	FromLabel            string `json:"from_label"`
	ToProvider           string `json:"to_provider"`
	ToLabel              string `json:"to_label"`
	Reason               string `json:"reason"`
	EstimatedInputChars  int    `json:"estimated_input_chars"`
	EstimatedInputTokens int    `json:"estimated_input_tokens"`
}

// ErrorMessage formats the prompt for embedding in an error. The first line
// carries the JSON payload prefixed with PromptPrefix; subsequent lines carry
// the human-readable explanation. Mirrors
// ProviderFailoverPrompt::to_error_message at failover.rs:17-23.
func (p ProviderFailoverPrompt) ErrorMessage() string {
	payload, err := json.Marshal(p)
	if err != nil {
		payload = []byte("{}")
	}
	return PromptPrefix + string(payload) + "\n" +
		p.FromLabel + " is unavailable; switching to " + p.ToLabel +
		" would resend about " + strconv.Itoa(p.EstimatedInputTokens) + " input tokens" +
		" (~" + strconv.Itoa(p.EstimatedInputChars) + " chars)."
}

// ParseFailoverPromptMessage extracts a ProviderFailoverPrompt from an error
// message emitted by ErrorMessage. Returns nil if the message does not start
// with PromptPrefix or the payload does not parse. Mirrors
// parse_failover_prompt_message at failover.rs:26-30.
func ParseFailoverPromptMessage(message string) *ProviderFailoverPrompt {
	if message == "" {
		return nil
	}
	firstLine := message
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		firstLine = message[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)
	if !strings.HasPrefix(firstLine, PromptPrefix) {
		return nil
	}
	payload := firstLine[len(PromptPrefix):]
	var p ProviderFailoverPrompt
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return nil
	}
	return &p
}

// Decision is what the retry loop should do after classifying an error.
// Mirrors FailoverDecision at failover.rs:33-37.
type Decision int

const (
	// DecisionNone means the error is not failover-worthy; retry on the
	// same provider or give up.
	DecisionNone Decision = iota
	// DecisionRetryNextProvider means the error is content-related
	// (context-length, 413); try the next provider with the same input.
	DecisionRetryNextProvider
	// DecisionRetryAndMarkUnavailable means the error is
	// auth/quota/rate-related; mark the current route unavailable and
	// retry with the next provider.
	DecisionRetryAndMarkUnavailable
)

// String returns a stable identifier for logging and persistence. Mirrors
// FailoverDecision::as_str at failover.rs:48-54.
func (d Decision) String() string {
	switch d {
	case DecisionRetryNextProvider:
		return "retry-next-provider"
	case DecisionRetryAndMarkUnavailable:
		return "retry-and-mark-unavailable"
	default:
		return "none"
	}
}

// ShouldFailover reports whether the decision implies moving to a different
// provider. Mirrors FailoverDecision::should_failover at failover.rs:40-42.
func (d Decision) ShouldFailover() bool {
	return d != DecisionNone
}

// ShouldMarkProviderUnavailable reports whether the active route should be
// flagged as temporarily unusable. Mirrors
// FailoverDecision::should_mark_provider_unavailable at failover.rs:44-46.
func (d Decision) ShouldMarkProviderUnavailable() bool {
	return d == DecisionRetryAndMarkUnavailable
}

// contextTooLongMarkers are the substrings whose presence in the lowercased
// error message identifies a request-size / context-window error. Mirrors the
// request_size_or_context list at failover.rs:72-84.
var contextTooLongMarkers = []string{
	"context length",
	"context_length",
	"context window",
	"maximum context",
	"prompt is too long",
	"input is too long",
	"too many tokens",
	"max tokens",
	"token limit",
	"token_limit",
	"413 payload too large",
	"413 request entity too large",
}

// rateOrQuotaMarkers identify rate-limit / quota / billing errors. Mirrors the
// rate_or_quota list at failover.rs:93-104.
var rateOrQuotaMarkers = []string{
	"rate limit",
	"rate-limited",
	"too many requests",
	"quota",
	"credit balance",
	"credits have run out",
	"insufficient credit",
	"billing",
	"payment required",
	"usage tier",
}

// authOrAccessMarkers identify auth / provider-unavailable errors. Mirrors
// the auth_or_access list at failover.rs:113-127.
var authOrAccessMarkers = []string{
	"access denied",
	"not accessible by integration",
	"provider unavailable",
	"provider not available",
	"provider is unavailable",
	"provider currently unavailable",
	"provider not configured",
	"credentials are not configured",
	"no credentials",
	"token exchange failed",
	"authentication failed",
	"unauthorized",
	"forbidden",
}

// ClassifyErrorMessage inspects a lowercased error message and returns the
// Decision the retry loop should apply. Order of checks matters: context
// errors take precedence over rate/auth markers. Mirrors
// classify_failover_error_message at failover.rs:69-137.
func ClassifyErrorMessage(message string) Decision {
	if message == "" {
		return DecisionNone
	}
	lower := strings.ToLower(message)

	if containsAny(lower, contextTooLongMarkers) || containsIndependentStatusCode(lower, "413") {
		return DecisionRetryNextProvider
	}
	if containsAny(lower, rateOrQuotaMarkers) ||
		containsIndependentStatusCode(lower, "429") ||
		containsIndependentStatusCode(lower, "402") {
		return DecisionRetryAndMarkUnavailable
	}
	if containsAny(lower, authOrAccessMarkers) ||
		containsIndependentStatusCode(lower, "401") ||
		containsIndependentStatusCode(lower, "403") {
		return DecisionRetryAndMarkUnavailable
	}
	return DecisionNone
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// containsIndependentStatusCode reports whether code appears in haystack as a
// standalone token (not embedded in a longer digit run). Mirrors
// contains_independent_status_code at failover.rs:57-67.
func containsIndependentStatusCode(haystack, code string) bool {
	if code == "" {
		return false
	}
	for i := 0; i <= len(haystack)-len(code); i++ {
		if haystack[i:i+len(code)] != code {
			continue
		}
		beforeOK := i == 0 || !isDigit(haystack[i-1])
		end := i + len(code)
		afterOK := end == len(haystack) || !isDigit(haystack[end])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
