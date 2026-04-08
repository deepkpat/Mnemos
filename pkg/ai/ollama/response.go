package ollama

// Ollama streams native JSON responses (newline-delimited):
//
//	{"model":"...","message":{"role":"assistant","content":"..."},"done":false}
//	{"model":"...","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop",...}

// nativeChunk is a single chunk in Ollama's streaming response.
type nativeChunk struct {
	Model           string        `json:"model"`
	Message         nativeMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

// nativeMessage is the message payload within a streaming chunk.
type nativeMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Thinking  string         `json:"thinking,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}
