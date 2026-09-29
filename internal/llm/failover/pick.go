package failover

import "strings"

// FallbackOptions carries extra context about the failure that triggered the
// fallback search. Mirrors FallbackPickOptions at fallback_pick.rs:34-45.
type FallbackOptions struct {
	// CredentialFailure means the failure was a credential/auth failure
	// (expired OAuth session, invalid API key, failed token refresh).
	// Every route that uses the same credential is equally broken, so:
	//   - when the failed APIMethod is known, all same-provider routes
	//     with that method are excluded (not just the same model), and
	//   - when the failed APIMethod is unknown, all same-provider routes
	//     are excluded because any of them could share the broken
	//     credential.
	CredentialFailure bool
}

// PickNextFallbackRoute returns the index of the best alternative route to
// fall back to after the currently-selected route failed. Returns -1 when no
// available route other than the current one exists. Mirrors
// pick_next_fallback_route at fallback_pick.rs:92-105.
func PickNextFallbackRoute(
	routes []ModelRoute,
	currentModel, currentProvider, currentAPIMethod string,
) int {
	return PickNextFallbackRouteWithOptions(routes, currentModel, currentProvider, currentAPIMethod,
		FallbackOptions{})
}

// PickNextFallbackRouteWithOptions is PickNextFallbackRoute with
// failure-classification options. Mirrors
// pick_next_fallback_route_with_options at fallback_pick.rs:108-162.
//
// Ranking (lower tuple wins; ties broken to keep the result stable and to
// prefer subscription logins, then original catalog order):
//
//  1. Same model, different auth method - e.g. the active `claude-api`
//     route's key is broken but a `claude-oauth` login for the same model
//     is available.
//  2. Same provider, different model.
//  3. Different provider (last resort cross-provider hop).
func PickNextFallbackRouteWithOptions(
	routes []ModelRoute,
	currentModel, currentProvider, currentAPIMethod string,
	options FallbackOptions,
) int {
	unknownMethod := strings.TrimSpace(currentAPIMethod) == ""

	best := -1
	var bestKey [3]int

	for index, route := range routes {
		if !route.Available {
			continue
		}

		sameModel := modelsMatch(route.Model, currentModel)
		sameMethod := apiMethodsMatch(route.APIMethod, currentAPIMethod)
		sameProvider := ProviderLabelsMatch(route.Provider, currentProvider) ||
			modelsMatch(route.Provider, currentProvider)

		// Never offer the exact route that just failed. When the caller
		// does not know which auth method the failed route used (empty
		// currentAPIMethod), any same-model route on the same provider
		// could be that exact failed route, so skip those too instead of
		// offering the user a "fallback" that is guaranteed to fail
		// identically.
		if sameModel && sameProvider && (sameMethod || unknownMethod) {
			continue
		}

		// A credential failure breaks every route that authenticates with
		// the same credential, not just the failed model.
		if options.CredentialFailure && sameProvider && (sameMethod || unknownMethod) {
			continue
		}

		var tier int
		switch {
		case sameModel && !sameMethod:
			tier = 0
		case sameProvider:
			tier = 1
		default:
			tier = 2
		}

		// Within a tier, prefer subscription (OAuth) logins, then
		// preserve catalog ordering.
		prefersOAuth := 1
		if ParseAPIMethod(route.APIMethod).IsOAuth() {
			prefersOAuth = 0
		}

		key := [3]int{tier, prefersOAuth, index}
		if best == -1 || compareKeys(key, bestKey) < 0 {
			best = index
			bestKey = key
		}
	}

	return best
}

// compareKeys returns -1 / 0 / +1 for a < b / a == b / a > b, using
// lexicographic order on the integer triple.
func compareKeys(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func modelsMatch(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func apiMethodsMatch(a, b string) bool {
	return ParseAPIMethod(a) == ParseAPIMethod(b)
}
