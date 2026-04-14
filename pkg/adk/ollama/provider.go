package ollama

import (
	"context"
	"net/http"
	"time"

	"mnemos/pkg/adk"
)

const (
	apiName        = "ollama"
	defaultBaseURL = "http://localhost:11434"
)

// Provider implements adk.Provider for Ollama's OpenAI-compatible API.
type Provider struct {
	client *http.Client
}

// New creates a new Ollama provider with the default HTTP client.
func New() *Provider {
	return &Provider{
		client: &http.Client{Timeout: 10 * time.Minute},
	}
}

// NewWithClient creates a new Ollama provider with a custom HTTP client.
func NewWithClient(client *http.Client) *Provider {
	return &Provider{client: client}
}

func (p *Provider) API() string { return apiName }

// Register registers this provider in the global ai registry.
func Register() *Provider {
	p := New()
	adk.RegisterProvider(p)
	return p
}

// Stream starts a streaming chat completion request to Ollama.
func (p *Provider) Stream(ctx context.Context, model *adk.Model, thread *adk.Thread, opts *adk.StreamOptions) *adk.EventStream {
	stream := adk.NewEventStream(128)

	go func() {
		defer stream.Close()
		p.doStream(ctx, stream, model, thread, opts)
	}()

	return stream
}
