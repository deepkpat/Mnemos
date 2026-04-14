package adk

// Usage tracks token consumption and associated costs
type Usage struct {
	Input       uint64    `json:"input"`
	Output      uint64    `json:"output"`
	CacheRead   uint64    `json:"cache_read"`
	CacheWrite  uint64    `json:"cache_write"`
	TotalTokens uint64    `json:"total_tokens"`
	Cost        UsageCost `json:"cost"`
}

// UsageCost tracks the dollar cost breakdown
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Total      float64 `json:"total"`
}

// ZeroUsage returns a Usage with all fields zeroed
func ZeroUsage() Usage {
	return Usage{}
}
