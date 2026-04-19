package adk

// Thread is the full conversation state sent to the AI model.
type Thread struct {
	SystemPrompt string    `json:"system_prompt,omitempty"`
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
}
