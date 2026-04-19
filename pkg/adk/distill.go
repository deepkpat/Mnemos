package adk

import "sync"

// ThreadDistiller transforms a message list before sending to the LLM
// It can be used to compress, filter, or otherwise process messages
type ThreadDistiller interface {
	Distill(messages []Message) []Message
}

// IdentityDistiller is a no-op distiller
type IdentityThreadDistiller struct{}

func (IdentityThreadDistiller) Distill(messages []Message) []Message {
	return messages
}

// DistillerRegistry manages the available distillation strategies
type DistillerRegistry struct {
	mu         sync.RWMutex
	distillers map[string]ThreadDistiller
}

// NewDistillerRegistry initializes a new registry with an IdentityDistiller by default
func NewDistillerRegistry() *DistillerRegistry {
	r := &DistillerRegistry{
		distillers: make(map[string]ThreadDistiller),
	}
	r.Register("identity", IdentityThreadDistiller{})
	return r
}

// Register adds a new distiller to the registry
func (r *DistillerRegistry) Register(name string, distiller ThreadDistiller) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.distillers[name] = distiller
}

// Get retrieves a distiller by name. Returns the identity distiller if not found
func (r *DistillerRegistry) Get(name string) ThreadDistiller {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if d, ok := r.distillers[name]; ok {
		return d
	}

	// Fallback to identity to avoid nil pointer panics in the pipeline
	return r.distillers["identity"]
}

// Remove deletes a distiller from the registry
func (r *DistillerRegistry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.distillers, name)
}
