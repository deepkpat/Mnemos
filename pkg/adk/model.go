package adk

// InputModality describes the type of input model accepts
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
)

// ModelCost describes pricing per million tokens
type ModelCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// Model describes a specific ai model
type Model struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	API             string     `json:"api"`
	Provider        string     `json:"provider"`
	BaseURL         string     `json:"base_url"`
	InputModalities []Modality `json:"input_modalities"`
	Reasoning       bool       `json:"reasoning"`
	Cost            ModelCost  `json:"cost"`
	ContextWindow   uint64     `json:"context_window"`
	MaxTokens       uint64     `json:"max_tokens"`
}

// CalculateCost computes costs from token usage and model pricing
func (m *Model) CalculateCost(u *Usage) {
	u.Cost.Input = (m.Cost.Input / 1_000_000) * float64(u.Input)
	u.Cost.Output = (m.Cost.Output / 1_000_000) * float64(u.Output)
	u.Cost.CacheRead = (m.Cost.CacheRead / 1_000_000) * float64(u.CacheRead)
	u.Cost.CacheWrite = (m.Cost.CacheWrite / 1_000_000) * float64(u.CacheWrite)
	u.Cost.Total = u.Cost.Input + u.Cost.Output + u.Cost.CacheRead + u.Cost.CacheWrite
}
