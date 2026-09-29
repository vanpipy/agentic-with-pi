package failover_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm/failover"
)

func route(model, provider, method string, available bool) failover.ModelRoute {
	return failover.ModelRoute{
		Model:     model,
		Provider:  provider,
		APIMethod: method,
		Available: available,
	}
}

func TestPickNextFallbackRoute_PrefersSameModelOAuthWhenAPIKeyBroken(t *testing.T) {
	routes := []failover.ModelRoute{
		route("claude-sonnet-4", "Anthropic", "claude-api", true),
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
		route("gpt-5", "OpenAI", "openai-oauth", true),
	}
	pick := failover.PickNextFallbackRoute(routes, "claude-sonnet-4", "Anthropic", "claude-api")
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].APIMethod != "claude-oauth" {
		t.Errorf("got api_method %q, want claude-oauth", routes[pick].APIMethod)
	}
	if routes[pick].Model != "claude-sonnet-4" {
		t.Errorf("got model %q, want claude-sonnet-4", routes[pick].Model)
	}
}

func TestPickNextFallbackRoute_FallsBackToSameProviderSiblingModel(t *testing.T) {
	routes := []failover.ModelRoute{
		route("claude-opus-4", "Anthropic", "claude-oauth", true),
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
	}
	pick := failover.PickNextFallbackRoute(routes, "claude-opus-4", "Anthropic", "claude-oauth")
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].Model != "claude-sonnet-4" {
		t.Errorf("got model %q, want claude-sonnet-4", routes[pick].Model)
	}
	if routes[pick].Provider != "Anthropic" {
		t.Errorf("got provider %q, want Anthropic", routes[pick].Provider)
	}
}

func TestPickNextFallbackRoute_CrossProviderAsLastResort(t *testing.T) {
	routes := []failover.ModelRoute{
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
		route("gpt-5", "OpenAI", "openai-oauth", true),
	}
	pick := failover.PickNextFallbackRoute(routes, "claude-sonnet-4", "Anthropic", "claude-oauth")
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].Provider != "OpenAI" {
		t.Errorf("got provider %q, want OpenAI", routes[pick].Provider)
	}
}

func TestPickNextFallbackRoute_SkipsUnavailableRoutes(t *testing.T) {
	routes := []failover.ModelRoute{
		route("claude-sonnet-4", "Anthropic", "claude-api", true),
		route("claude-sonnet-4", "Anthropic", "claude-oauth", false),
		route("gpt-5", "OpenAI", "openai-oauth", false),
	}
	pick := failover.PickNextFallbackRoute(routes, "claude-sonnet-4", "Anthropic", "claude-api")
	if pick != -1 {
		t.Fatalf("expected -1 (no available fallback), got %d", pick)
	}
}

func TestPickNextFallbackRoute_ReturnsNoneWhenOnlyCurrentRouteExists(t *testing.T) {
	routes := []failover.ModelRoute{
		route("gpt-5", "OpenAI", "openai-oauth", true),
	}
	pick := failover.PickNextFallbackRoute(routes, "gpt-5", "OpenAI", "openai-oauth")
	if pick != -1 {
		t.Fatalf("expected -1 (single route), got %d", pick)
	}
}

func TestPickNextFallbackRoute_CrossProviderPrefersOAuthOverAPIKey(t *testing.T) {
	routes := []failover.ModelRoute{
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
		route("gpt-5", "OpenAI", "openai-api", true),
		route("gpt-5", "OpenAI", "openai-oauth", true),
	}
	pick := failover.PickNextFallbackRoute(routes, "claude-sonnet-4", "Anthropic", "claude-oauth")
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].Provider != "OpenAI" || routes[pick].APIMethod != "openai-oauth" {
		t.Errorf("got %s/%s, want OpenAI/openai-oauth", routes[pick].Provider, routes[pick].APIMethod)
	}
}

func TestPickNextFallbackRoute_UnknownMethodNeverOffersSameModelSameProvider(t *testing.T) {
	routes := []failover.ModelRoute{
		route("gpt-5.5", "OpenAI", "openai-oauth", true),
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
	}
	pick := failover.PickNextFallbackRoute(routes, "gpt-5.5", "OpenAI", "")
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].Provider != "Anthropic" {
		t.Errorf("got provider %q, want Anthropic", routes[pick].Provider)
	}
}

func TestPickNextFallbackRoute_CredentialFailureSkipsSiblingOnSameCredential(t *testing.T) {
	routes := []failover.ModelRoute{
		route("gpt-5.5", "OpenAI", "openai-oauth", true),
		route("gpt-5.4", "OpenAI", "openai-oauth", true),
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
	}
	pick := failover.PickNextFallbackRouteWithOptions(routes, "gpt-5.5", "OpenAI", "openai-oauth",
		failover.FallbackOptions{CredentialFailure: true})
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].Provider != "Anthropic" {
		t.Errorf("got provider %q, want Anthropic (credential failure should skip sibling OpenAI models)", routes[pick].Provider)
	}
}

func TestPickNextFallbackRoute_CredentialFailureStillOffersOtherMethodSameProvider(t *testing.T) {
	routes := []failover.ModelRoute{
		route("gpt-5.5", "OpenAI", "openai-oauth", true),
		route("gpt-5.5", "OpenAI", "openai-api", true),
	}
	pick := failover.PickNextFallbackRouteWithOptions(routes, "gpt-5.5", "OpenAI", "openai-oauth",
		failover.FallbackOptions{CredentialFailure: true})
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].APIMethod != "openai-api" {
		t.Errorf("got api_method %q, want openai-api", routes[pick].APIMethod)
	}
}

func TestPickNextFallbackRoute_CredentialFailureUnknownMethodSkipsWholeProvider(t *testing.T) {
	routes := []failover.ModelRoute{
		route("gpt-5.5", "OpenAI", "openai-oauth", true),
		route("gpt-5.5", "OpenAI", "openai-api", true),
		route("gpt-5.4", "OpenAI", "openai-oauth", true),
		route("claude-sonnet-4", "Anthropic", "claude-oauth", true),
	}
	pick := failover.PickNextFallbackRouteWithOptions(routes, "gpt-5.5", "OpenAI", "",
		failover.FallbackOptions{CredentialFailure: true})
	if pick < 0 {
		t.Fatalf("expected a fallback, got -1")
	}
	if routes[pick].Provider != "Anthropic" {
		t.Errorf("got provider %q, want Anthropic", routes[pick].Provider)
	}
}

func TestErrorLooksLikeCredentialFailure_ClassifiesAuthFailures(t *testing.T) {
	cases := []string{
		"OpenAI token refresh failed; run /login to re-authenticate: refresh_token_invalidated",
		"Your session has ended. Please log in again.",
		"Anthropic API error (401 Unauthorized): authentication_error invalid x-api-key",
		"task failed: Anthropic API error (401 Unauthorized)",
		"invalid_grant: refresh token invalid",
		"provider returned Unauthorized",
	}
	for _, msg := range cases {
		if !failover.ErrorLooksLikeCredentialFailure(msg) {
			t.Errorf("ErrorLooksLikeCredentialFailure(%q) = false, want true", msg)
		}
	}
}

func TestErrorLooksLikeCredentialFailure_RejectsNonAuthErrors(t *testing.T) {
	cases := []string{
		"429 rate limit exceeded, retry after 30s",
		"500 internal server error",
		"",
		"context length exceeded",
	}
	for _, msg := range cases {
		if failover.ErrorLooksLikeCredentialFailure(msg) {
			t.Errorf("ErrorLooksLikeCredentialFailure(%q) = true, want false", msg)
		}
	}
}

func TestAPIMethod_IsOAuth(t *testing.T) {
	cases := []struct {
		method string
		want   bool
	}{
		{"claude-oauth", true},
		{"openai-oauth", true},
		{"code-assist-oauth", true},
		{"claude-api", false},
		{"openai-api", false},
		{"openai-api-key", false},
		{"anthropic-api-key", false},
		{"openrouter", false},
		{"copilot", false},
		{"bedrock", false},
		{"", false},
		{"custom-thing", false},
	}
	for _, c := range cases {
		if got := failover.ParseAPIMethod(c.method).IsOAuth(); got != c.want {
			t.Errorf("ParseAPIMethod(%q).IsOAuth() = %v, want %v", c.method, got, c.want)
		}
	}
}

func TestAPIMethod_ParseAliases(t *testing.T) {
	if failover.ParseAPIMethod("CLAUDE-OAUTH") != failover.APIMethodClaudeOAuth {
		t.Errorf("CLAUDE-OAUTH should normalize to ClaudeOAuth")
	}
	if failover.ParseAPIMethod("anthropic-api-key") != failover.APIMethodAnthropicAPIKey {
		t.Errorf("anthropic-api-key should normalize to AnthropicAPIKey")
	}
	if failover.ParseAPIMethod("openai-api-key") != failover.APIMethodOpenAIAPIKey {
		t.Errorf("openai-api-key should normalize to OpenAIAPIKey")
	}
	if failover.ParseAPIMethod("openai-compatible:cerebras") != failover.APIMethodOpenAICompatible {
		t.Errorf("openai-compatible:cerebras should normalize to OpenAICompatible")
	}
}

func TestProviderLabelsMatch_Aliases(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"claude", "Anthropic", true},
		{"Anthropic", "claude", true},
		{"openai", "OpenAI", true},
		{"gemini", "google", true},
		{"google", "gemini", true},
		{"copilot", "GitHubCopilot", true},
		{"openai", "anthropic", false},
		{"claude", "openai", false},
		{"", "claude", false},
		{"claude", "", false},
		{"random", "claude", false},
	}
	for _, c := range cases {
		if got := failover.ProviderLabelsMatch(c.a, c.b); got != c.want {
			t.Errorf("ProviderLabelsMatch(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
