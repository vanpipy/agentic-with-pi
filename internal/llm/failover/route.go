package failover

import "strings"

// ModelRoute is a single route to access a model: model + provider + api method.
// Mirrors jcode ModelRoute at lib.rs:676-684.
type ModelRoute struct {
	Model     string
	Provider  string
	APIMethod string
	Available bool
	Detail    string
}

// APIMethod is the typed view of ModelRoute.APIMethod, used for the
// routing/credential-failure logic. Mirrors jcode ModelRouteApiMethod at
// lib.rs:856-872. Only the OAuth detection (`IsOAuth`) and the parsing
// stability across aliases are load-bearing for the picker; the rest of
// the variants exist so provider adapters can round-trip.
type APIMethod int

const (
	APIMethodUnknown APIMethod = iota
	APIMethodJcodeSubscription
	APIMethodClaudeOAuth
	APIMethodAnthropicAPIKey
	APIMethodOpenAIOAuth
	APIMethodOpenAIAPIKey
	APIMethodOpenRouter
	APIMethodOpenAICompatible
	APIMethodCopilot
	APIMethodCursor
	APIMethodBedrock
	APIMethodCodeAssistOAuth
	APIMethodRemoteCatalog
	APIMethodOther
)

// IsOAuth reports whether the api_method authenticates via an OAuth /
// subscription login rather than a metered API key. Mirrors
// fallback_pick.rs:17-24.
func (a APIMethod) IsOAuth() bool {
	switch a {
	case APIMethodClaudeOAuth, APIMethodOpenAIOAuth, APIMethodCodeAssistOAuth:
		return true
	default:
		return false
	}
}

// ParseAPIMethod normalizes the raw ModelRoute.APIMethod string to a
// canonical kind. Mirrors ModelRouteApiMethod::parse at lib.rs:887-918.
func ParseAPIMethod(value string) APIMethod {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch lower {
	case "", "current":
		return APIMethodUnknown
	case "jcode-subscription":
		return APIMethodJcodeSubscription
	case "claude-oauth":
		return APIMethodClaudeOAuth
	case "claude-api", "anthropic-api-key", "anthropic-api":
		return APIMethodAnthropicAPIKey
	case "openai-oauth":
		return APIMethodOpenAIOAuth
	case "openai-api", "openai-api-key":
		return APIMethodOpenAIAPIKey
	case "openrouter":
		return APIMethodOpenRouter
	case "openai-compatible", "openai-compatible:":
		return APIMethodOpenAICompatible
	case "copilot":
		return APIMethodCopilot
	case "cursor":
		return APIMethodCursor
	case "bedrock":
		return APIMethodBedrock
	case "code-assist-oauth":
		return APIMethodCodeAssistOAuth
	case "remote-catalog":
		return APIMethodRemoteCatalog
	}
	if strings.HasPrefix(lower, "openai-compatible:") {
		return APIMethodOpenAICompatible
	}
	return APIMethodOther
}

// normalizeProviderLabel lowercases and strips non-alphanumeric characters
// so provider label comparisons tolerate "Claude" / "claude " /
// "CLAUDE_PROVIDER". Mirrors normalize_model_route_provider_label at
// lib.rs:985-990.
func normalizeProviderLabel(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == ' ' || r == '_' || r == '-' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r = r + 32
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ProviderLabelsMatch reports whether the two provider labels refer to the
// same provider. Recognizes the canonical aliases (claude / anthropic,
// openai, gemini / google, antigravity, copilot variants). Mirrors
// model_route_provider_labels_match at lib.rs:992-1010.
func ProviderLabelsMatch(routeProvider, currentProvider string) bool {
	r := normalizeProviderLabel(routeProvider)
	c := normalizeProviderLabel(currentProvider)
	if r == "" || c == "" {
		return false
	}
	if r == c {
		return true
	}
	switch {
	case (c == "claude" || c == "anthropic") && (r == "claude" || r == "anthropic"):
		return true
	case c == "openai" && r == "openai":
		return true
	case (c == "gemini" || c == "google") && (r == "gemini" || r == "google"):
		return true
	case c == "antigravity" && r == "antigravity":
		return true
	case (c == "copilot" || c == "copilotcode" || c == "githubcopilot") &&
		(r == "copilot" || r == "copilotcode" || r == "githubcopilot"):
		return true
	}
	return false
}

// credentialFailureMarkers identify errors whose root cause is a broken
// credential. Mirrors the markers array at fallback_pick.rs:54-72.
var credentialFailureMarkers = []string{
	"token refresh failed",
	"refresh_token_invalidated",
	"re-authenticate",
	"session has ended",
	"authentication_error",
	"invalid_grant",
	"invalid x-api-key",
	"invalid api key",
	"incorrect api key",
	"api key not valid",
	"token expired",
	"unauthorized",
	"oauth token expired",
	"no refresh token",
	"credentials have been revoked",
	"please log in again",
	"run /login",
}

// ErrorLooksLikeCredentialFailure reports whether the error message looks
// like a credential/auth failure. Mirrors
// error_looks_like_credential_failure at fallback_pick.rs:52-74.
func ErrorLooksLikeCredentialFailure(message string) bool {
	if message == "" {
		return false
	}
	lower := strings.ToLower(message)
	for _, marker := range credentialFailureMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
