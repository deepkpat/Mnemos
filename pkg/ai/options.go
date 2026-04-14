package ai

// ThinkingLevel controls how much reasoning effort the model should use
type ThinkingLevel string

const (
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
)

// StreamOptions configures a streaming request
type StreamOptions struct {
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      uint64            `json:"max_tokens,omitempty"`
	APIKey         string            `json:"api_key,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Reasoning      ThinkingLevel     `json:"reasoning,omitempty"`
	ThinkingBudget int               `json:"thinking_budget,omitempty"`
}
