// Package failover (failover_core.go): FailoverCore orchestrates multiple
// Core instances, retrying within each via RetryCore and switching to the
// next when the retryable-but-failover-worthy error type surfaces. Mirrors
// the cross-provider failover pattern from jcode failover.rs /
// fallback_pick.rs and is the bridge between the pure classifiers and the
// retry-aware StreamChat surface.
package llm

import (
	"context"
	"log/slog"

	"github.com/vanpiyp/awp/internal/llm/failover"
)

// FailoverRoute describes one provider entry in a failover chain.
type FailoverRoute struct {
	Core  Core
	Route failover.ModelRoute
}

// FailoverConfig configures cross-provider failover behaviour.
type FailoverConfig struct {
	// Retry is applied within a single route via the standard RetryCore.
	Retry RetryConfig
	// MaxRoutes caps how many routes the failover loop will try before
	// giving up. Zero or negative means "all available routes".
	MaxRoutes int
	// OnDecision is an optional hook fired on every classified
	// failover-worthy error before switching routes. The hook may
	// e.g. surface a ProviderFailoverPrompt to the UI. nil disables.
	OnDecision func(fromIdx int, toIdx int, decision failover.Decision, lastErr error)
}

// FailoverCore tries Core[0], retries within it via RetryCore, and on a
// classified failover-worthy error advances to the next available Core via
// failover.PickNextFallbackRouteWithOptions.
type FailoverCore struct {
	routes []FailoverRoute
	config FailoverConfig
}

// NewFailoverCore returns a FailoverCore that walks the routes in picker
// order (the caller's current route first, then the fallback chain),
// retrying within each one via the supplied RetryConfig.
func NewFailoverCore(routes []FailoverRoute, config FailoverConfig) *FailoverCore {
	if config.Retry.MaxRetries < 0 {
		config.Retry.MaxRetries = 0
	}
	if config.MaxRoutes <= 0 || config.MaxRoutes > len(routes) {
		config.MaxRoutes = len(routes)
	}
	return &FailoverCore{routes: routes, config: config}
}

// StreamChat satisfies Core. The channel emits events from the active
// route; if the active route fails with a classified failover-worthy
// error, the next route is tried with the same request.
func (f *FailoverCore) StreamChat(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	out := make(chan StreamEvent, 32)
	go f.run(ctx, req, out)
	return out, nil
}

func (f *FailoverCore) run(ctx context.Context, req *ChatRequest, out chan<- StreamEvent) {
	defer close(out)
	if len(f.routes) == 0 {
		return
	}
	order := f.buildOrder(0)
	for i, idx := range order {
		if err := ctx.Err(); err != nil {
			return
		}
		if i >= f.config.MaxRoutes {
			return
		}
		if i > 0 {
			if !f.notifySwitch(ctx, order[i-1], idx, out) {
				return
			}
		}
		ch, ok := f.tryRoute(ctx, f.routes[idx].Core, req)
		if !ok {
			continue
		}
		for ev := range ch {
			if !f.send(ctx, out, ev) {
				return
			}
		}
		return
	}
}

func (f *FailoverCore) tryRoute(ctx context.Context, core Core, req *ChatRequest) (<-chan StreamEvent, bool) {
	rc := NewRetryCore(core, f.config.Retry)
	ch, err := rc.StreamChat(ctx, req)
	if err != nil {
		return nil, false
	}
	drained := make(chan StreamEvent, 32)
	var lastErr error
	for ev := range ch {
		if e, ok := ev.(EventErr); ok && e.Err != nil {
			lastErr = e.Err
		}
		drained <- ev
	}
	close(drained)
	if lastErr != nil {
		slog.Debug("llm/failover: route gave up", "err", lastErr)
		_ = lastErr
		return nil, false
	}
	return drained, true
}

// buildOrder returns the index sequence to walk through the route table:
// caller-supplied current route, then the picker-ranked fallback set.
func (f *FailoverCore) buildOrder(currentIdx int) []int {
	if currentIdx < 0 || currentIdx >= len(f.routes) {
		return nil
	}
	order := []int{currentIdx}
	for seen := 1; seen < len(f.routes); seen++ {
		next := f.pickNext(currentIdx, order)
		if next < 0 {
			break
		}
		order = append(order, next)
	}
	return order
}

func (f *FailoverCore) pickNext(currentIdx int, alreadyTried []int) int {
	routes := make([]failover.ModelRoute, len(f.routes))
	for i, r := range f.routes {
		routes[i] = r.Route
	}
	current := f.routes[currentIdx].Route
	candidate := failover.PickNextFallbackRoute(
		routes,
		current.Model,
		current.Provider,
		current.APIMethod,
	)
	if candidate < 0 {
		return -1
	}
	for _, tried := range alreadyTried {
		if candidate == tried {
			return -1
		}
	}
	return candidate
}

// notifySwitch emits a synthetic EventErr carrying a ProviderFailoverPrompt
// before the next route is tried. Returns false if the context is done.
func (f *FailoverCore) notifySwitch(
	ctx context.Context,
	fromIdx, toIdx int,
	out chan<- StreamEvent,
) bool {
	from := f.routes[fromIdx].Route
	to := f.routes[toIdx].Route
	prompt := failover.ProviderFailoverPrompt{
		FromProvider: from.Provider,
		FromLabel:    from.Provider,
		ToProvider:   to.Provider,
		ToLabel:      to.Provider,
		Reason:       "cross-provider failover",
	}
	if f.config.OnDecision != nil {
		f.config.OnDecision(fromIdx, toIdx, failover.DecisionRetryAndMarkUnavailable, nil)
	}
	return f.send(ctx, out, EventErr{Err: &failoverFailoverError{prompt: prompt}})
}

// failoverFailoverError is a synthetic error whose Error() embeds the
// ProviderFailoverPrompt payload, ready for a UI handler to parse.
type failoverFailoverError struct {
	prompt failover.ProviderFailoverPrompt
}

func (e *failoverFailoverError) Error() string {
	return e.prompt.ErrorMessage()
}

func (f *FailoverCore) send(ctx context.Context, out chan<- StreamEvent, ev StreamEvent) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
