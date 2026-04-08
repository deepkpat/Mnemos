package ai

// Context is the full conversation state sent to the LLM.
type Context struct {
	SystemPrompt string    `json:"systemPrompt,omitempty"`
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
}

// StreamOptions configures a streaming request.
type StreamOptions struct {
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      int               `json:"maxTokens,omitempty"`
	APIKey         string            `json:"apiKey,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Reasoning      ThinkingLevel     `json:"reasoning,omitempty"`
	ThinkingBudget int               `json:"thinkingBudget,omitempty"`
}
