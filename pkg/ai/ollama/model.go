package ollama

import "mnemos/pkg/ai"

// ModelOption configures a Model created by NewModel.
type ModelOption func(*ai.Model)

// NewModel creates a Model configured for Ollama.
func NewModel(modelID string, opts ...ModelOption) *ai.Model {
	m := &ai.Model{
		ID:            modelID,
		Name:          modelID,
		API:           apiName,
		Provider:      "ollama",
		BaseURL:       defaultBaseURL,
		Reasoning:     false,
		Input:         []ai.InputModality{ai.InputText},
		Cost:          ai.ModelCost{}, // local models are free
		ContextWindow: 8192,
		MaxTokens:     4096,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// WithBaseURL sets a custom Ollama server URL.
func WithBaseURL(url string) ModelOption {
	return func(m *ai.Model) { m.BaseURL = url }
}

// WithContextWindow sets the context window size.
func WithContextWindow(n int) ModelOption {
	return func(m *ai.Model) { m.ContextWindow = n }
}

// WithMaxTokens sets the maximum output tokens.
func WithMaxTokens(n int) ModelOption {
	return func(m *ai.Model) { m.MaxTokens = n }
}

// WithReasoning marks the model as supporting reasoning/thinking.
func WithReasoning() ModelOption {
	return func(m *ai.Model) { m.Reasoning = true }
}

// WithVision marks the model as supporting image input.
func WithVision() ModelOption {
	return func(m *ai.Model) { m.Input = []ai.InputModality{ai.InputText, ai.InputImage} }
}

// WithName sets a human-friendly display name.
func WithName(name string) ModelOption {
	return func(m *ai.Model) { m.Name = name }
}
