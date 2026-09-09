package llm

import (
	"fmt"
	"sort"
	"sync"
)

const DefaultProviderName = "minimax"

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(id string, p Provider) error {
	if id == "" {
		return fmt.Errorf("registry: provider id must not be empty")
	}
	if p == nil {
		return fmt.Errorf("registry: provider %q is nil", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[id]; exists {
		return fmt.Errorf("registry: provider %q already registered", id)
	}
	r.providers[id] = p
	return nil
}

func (r *Registry) MustRegister(id string, p Provider) {
	if err := r.Register(id, p); err != nil {
		panic(err)
	}
}

func (r *Registry) MustRegisterDefault(p Provider) {
	r.MustRegister(DefaultProviderName, p)
}

func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

func (r *Registry) Default() (Provider, bool) {
	return r.Get(DefaultProviderName)
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *Registry) FindModel(modelID string) (Model, Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		for _, m := range p.Models() {
			if m.ID == modelID {
				return m, p, true
			}
		}
	}
	return Model{}, nil, false
}

func (r *Registry) AllModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Model
	for _, p := range r.providers {
		out = append(out, p.Models()...)
	}
	return out
}
