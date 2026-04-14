package adk

// Tool defines a function the model can call
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"` // json schema
}
