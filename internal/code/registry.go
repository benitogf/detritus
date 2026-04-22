package code

import (
	"fmt"
	"sync"

	"github.com/blevesearch/bleve/v2"
)

// Registry manages lazily-opened Bleve indexes for the known packs.
// It is safe for concurrent use.
type Registry struct {
	mu      sync.Mutex
	handles map[string]bleve.Index
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{handles: map[string]bleve.Index{}}
}

// Open returns the Bleve index for a pack, opening it on first call.
func (r *Registry) Open(name string) (bleve.Index, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx, ok := r.handles[name]; ok {
		return idx, nil
	}
	m, err := LoadManifest(name)
	if err != nil {
		return nil, fmt.Errorf("pack %q not found", name)
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("pack %q was built with schema %d; current is %d — run `detritus --refresh %s`",
			name, m.SchemaVersion, SchemaVersion, name)
	}
	idx, err := bleve.Open(IndexPath(name))
	if err != nil {
		return nil, fmt.Errorf("open index for %q: %w", name, err)
	}
	r.handles[name] = idx
	return idx, nil
}

// Close releases every open index handle.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for _, idx := range r.handles {
		if err := idx.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.handles = map[string]bleve.Index{}
	return firstErr
}

// Forget drops an open handle (used after refresh/unpack so the next Open re-reads).
func (r *Registry) Forget(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if idx, ok := r.handles[name]; ok {
		_ = idx.Close()
		delete(r.handles, name)
	}
}
