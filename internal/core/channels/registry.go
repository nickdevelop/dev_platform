package channels

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds registered external IM channel adapters keyed by ChannelType.
type Registry struct {
	mu       sync.RWMutex
	adapters map[ChannelType]ChannelAdapter
}

// NewRegistry creates an empty channel adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[ChannelType]ChannelAdapter),
	}
}

// Register adds a channel adapter to the registry.
func (r *Registry) Register(adapter ChannelAdapter) error {
	if adapter == nil {
		return fmt.Errorf("channels: nil adapter")
	}
	desc := adapter.Descriptor()
	if desc.Type == "" {
		return fmt.Errorf("channels: adapter descriptor missing type")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[desc.Type]; exists {
		return fmt.Errorf("channels: duplicate adapter %q", desc.Type)
	}
	r.adapters[desc.Type] = adapter
	return nil
}

// Get returns the adapter for channelType.
func (r *Registry) Get(channelType ChannelType) (ChannelAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[channelType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownChannel, channelType)
	}
	return adapter, nil
}

// Descriptors returns sorted metadata for all registered channel types.
func (r *Registry) Descriptors() []ChannelDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ChannelDescriptor, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a.Descriptor())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Type < out[j].Type
	})
	return out
}
