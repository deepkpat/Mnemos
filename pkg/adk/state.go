package adk

// AgentState holds the agent's runtime state
// !PENDING
type AgentState struct {
	SessionID    string `json:"session_id"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	Model        *Model `json:"model"`
}
