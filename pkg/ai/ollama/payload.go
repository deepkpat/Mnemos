package ollama

// chatRequest is the top-level request body for Ollama's /api/chat endpoint.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
	Format   any           `json:"format,omitempty"`
	Options  *chatOptions  `json:"options,omitempty"`
	Stream   bool          `json:"stream"`
	Think    bool          `json:"think,omitempty"`
}

// chatOptions carries per-request inference parameters.
type chatOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
	NumCtx      int      `json:"num_ctx,omitempty"`
}

// chatMessage is a single message in the chat history.
type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
	Images    []string       `json:"images,omitempty"`
}

// chatToolCall represents a tool invocation inside a chat message.
type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

// chatFunctionCall holds the name and arguments of a function call.
type chatFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// chatTool describes a tool available to the model.
type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

// chatFunction is the function definition within a tool declaration.
type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}
