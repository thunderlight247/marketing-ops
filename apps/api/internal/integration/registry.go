package integration

import (
	"sort"
	"sync"
)

// Registry maintains all registered integration adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// NewDefaultRegistry returns a registry initialized with all built-in adapters.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&SanityAdapter{})
	r.Register(&GenericAPIAdapter{})
	r.Register(&ManualExportAdapter{})
	return r
}

// Register adds or replaces an adapter by its Kind().
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Kind()] = a
}

// Get returns the adapter for kind, or false if not registered.
func (r *Registry) Get(kind string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[kind]
	return a, ok
}

// ListDescriptors returns sorted metadata for all registered adapters.
func (r *Registry) ListDescriptors() []AdapterDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AdapterDescriptor, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, AdapterDescriptor{
			Kind:        a.Kind(),
			DisplayName: a.DisplayName(),
			Description: a.Description(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Kind < out[j].Kind
	})
	return out
}
