package ai

// InputModality describes what kind of input a model accepts.
type InputModality string

const (
	InputText  InputModality = "text"
	InputImage InputModality = "image"
)

// ModelCost describes pricing per million tokens.
type ModelCost struct {
	Input      float64 `json:"input"`      // $/million tokens
	Output     float64 `json:"output"`     // $/million tokens
	CacheRead  float64 `json:"cacheRead"`  // $/million tokens
	CacheWrite float64 `json:"cacheWrite"` // $/million tokens
}

// Model describes a specific LLM endpoint.
type Model struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	API           string            `json:"api"`
	Provider      string            `json:"provider"`
	BaseURL       string            `json:"baseUrl"`
	Reasoning     bool              `json:"reasoning"`
	Input         []InputModality   `json:"input"`
	Cost          ModelCost         `json:"cost"`
	ContextWindow int               `json:"contextWindow"`
	MaxTokens     int               `json:"maxTokens"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// CalculateCost computes costs from token usage and model pricing.
func (m *Model) CalculateCost(u *Usage) {
	u.Cost.Input = (m.Cost.Input / 1_000_000) * float64(u.Input)
	u.Cost.Output = (m.Cost.Output / 1_000_000) * float64(u.Output)
	u.Cost.CacheRead = (m.Cost.CacheRead / 1_000_000) * float64(u.CacheRead)
	u.Cost.CacheWrite = (m.Cost.CacheWrite / 1_000_000) * float64(u.CacheWrite)
	u.Cost.Total = u.Cost.Input + u.Cost.Output + u.Cost.CacheRead + u.Cost.CacheWrite
}
