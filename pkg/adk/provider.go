package adk

import (
	"context"
	"fmt"
	"sync"
)

// StreamFunc is the function signature for provider streaming implementations.
// It must return an EventStream immediately. All async work (HTTP calls, parsing)
// happens in the background, pushing events into the returned stream.
type StreamFunc func(ctx context.Context, model *Model, thread *Thread, opts *StreamOptions) *EventStream

// Provider is the interface that LLM providers implement.
type Provider interface {
	// API returns the API protocol identifier (e.g. "ollama", "openai-completions").
	API() string

	// Stream starts a streaming request and returns an EventStream.
	Stream(ctx context.Context, model *Model, thread *Thread, opts *StreamOptions) *EventStream
}

// --- Registry ---

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Provider)
)

// RegisterProvider registers a provider for the given API protocol.
// If a provider is already registered for the same API, it is replaced.
func RegisterProvider(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.API()] = p
}

// GetProvider returns the registered provider for the given API, or nil.
func GetProvider(api string) Provider {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[api]
}

// UnregisterProvider removes a provider by API name.
func UnregisterProvider(api string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, api)
}

// MustGetProvider returns the provider or panics if not found.
func MustGetProvider(api string) Provider {
	p := GetProvider(api)
	if p == nil {
		panic(fmt.Sprintf("ai: no provider registered for api %q", api))
	}
	return p
}
