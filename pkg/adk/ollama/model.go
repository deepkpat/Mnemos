package ollama

import (
	"mnemos/pkg/adk"
)

// NewModel creates a Model configured for Ollama.
func NewModel(modelID string, opts ...ModelOption) *adk.Model {
	m := &adk.Model{
		ID:              modelID,
		Name:            modelID,
		API:             apiName,
		Provider:        "ollama",
		BaseURL:         defaultBaseURL,
		Reasoning:       false,
		InputModalities: []adk.Modality{adk.ModalityText},
		Cost:            adk.ModelCost{}, // local models are free
		ContextWindow:   8192,
		MaxTokens:       4096,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ModelOption configures a Model created by NewModel.
type ModelOption func(*adk.Model)

// WithBaseURL sets a custom Ollama server URL.
func WithBaseURL(url string) ModelOption {
	return func(m *adk.Model) { m.BaseURL = url }
}

// WithContextWindow sets the context window size.
func WithContextWindow(n uint64) ModelOption {
	return func(m *adk.Model) { m.ContextWindow = n }
}

// WithMaxTokens sets the maximum output tokens.
func WithMaxTokens(n uint64) ModelOption {
	return func(m *adk.Model) { m.MaxTokens = n }
}

// WithReasoning marks the model as supporting reasoning/thinking.
func WithReasoning() ModelOption {
	return func(m *adk.Model) { m.Reasoning = true }
}

// WithVision marks the model as supporting image input.
func WithVision() ModelOption {
	return func(m *adk.Model) { m.InputModalities = []adk.Modality{adk.ModalityText, adk.ModalityImage} }
}

// WithName sets a human-friendly display name.
func WithName(name string) ModelOption {
	return func(m *adk.Model) { m.Name = name }
}
