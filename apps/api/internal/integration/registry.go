package integration

import (
	"sort"
	"sync"
)

// Registry maintains all registered integration adapters.
type Registry struct {
	mu         sync.RWMutex
	adapters   map[string]Adapter
	publishers map[string]PublishingAdapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters:   make(map[string]Adapter),
		publishers: make(map[string]PublishingAdapter),
	}
}

// NewDefaultRegistry returns a registry initialized with all built-in adapters.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&SanityAdapter{})
	r.RegisterPublisher("sanity", SanityPublisherAdapter{})
	r.Register(&MetaAdapter{})
	r.RegisterPublisher("meta", MetaPublisherAdapter{})
	r.Register(&XAdapter{})
	r.RegisterPublisher("x", XPublisherAdapter{})
	r.Register(&YouTubeAdapter{})
	r.RegisterPublisher("youtube", YouTubePublisherAdapter{})
	r.Register(&GenericAPIAdapter{})
	r.Register(&SearchConsoleAdapter{})
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

// RegisterPublisher adds or replaces the publishing implementation for an adapter kind.
func (r *Registry) RegisterPublisher(kind string, publisher PublishingAdapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.publishers[kind] = publisher
}

// Publisher returns the publishing implementation for kind, if it supports writes.
func (r *Registry) Publisher(kind string) (PublishingAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	publisher, ok := r.publishers[kind]
	return publisher, ok
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
