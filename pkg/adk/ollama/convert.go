package ollama

import (
	"strings"

	"mnemos/pkg/adk"
)

// --- Request payload types ---

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []chatTool    `json:"tools,omitempty"`
	Format   any           `json:"format,omitempty"`
	Options  *chatOptions  `json:"options,omitempty"`
	Stream   bool          `json:"stream"`
	Think    bool          `json:"think,omitempty"`
}

type chatOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	NumPredict  uint64   `json:"num_predict,omitempty"`
	NumCtx      uint64   `json:"num_ctx,omitempty"`
}

type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
	Images    []string       `json:"images,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// --- Message conversion ---

// buildPayload converts the adk.Thread into an Ollama chat completion request.
func buildPayload(model *adk.Model, thread *adk.Thread, opts *adk.StreamOptions) chatRequest {
	req := chatRequest{
		Model:  model.ID,
		Stream: true,
	}

	if model.Reasoning {
		req.Think = true
	}

	var options *chatOptions
	if opts != nil {
		if opts.Temperature != nil {
			options = &chatOptions{Temperature: opts.Temperature}
		}
		if opts.MaxTokens > 0 {
			if options == nil {
				options = &chatOptions{}
			}
			options.NumPredict = opts.MaxTokens
		}
	}
	if options == nil && (model.MaxTokens > 0 || model.ContextWindow > 0) {
		options = &chatOptions{}
	}
	if options != nil {
		if options.NumPredict == 0 && model.MaxTokens > 0 {
			options.NumPredict = model.MaxTokens
		}
		if options.NumCtx == 0 && model.ContextWindow > 0 {
			options.NumCtx = model.ContextWindow
		}
	}
	req.Options = options

	// System prompt
	if thread.SystemPrompt != "" {
		req.Messages = append(req.Messages, chatMessage{
			Role:    "system",
			Content: thread.SystemPrompt,
		})
	}

	// Conversation messages
	for _, msg := range thread.Messages {
		switch m := msg.(type) {
		case adk.UserMessage:
			req.Messages = append(req.Messages, convertUserMessage(m))
		case *adk.UserMessage:
			req.Messages = append(req.Messages, convertUserMessage(*m))
		case adk.AssistantMessage:
			req.Messages = append(req.Messages, convertAssistantMessage(m))
		case *adk.AssistantMessage:
			req.Messages = append(req.Messages, convertAssistantMessage(*m))
		case adk.ToolResultMessage:
			req.Messages = append(req.Messages, convertToolResultMessage(m))
		case *adk.ToolResultMessage:
			req.Messages = append(req.Messages, convertToolResultMessage(*m))
		}
	}

	// Tools
	for _, tool := range thread.Tools {
		req.Tools = append(req.Tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}

	return req
}

func convertUserMessage(m adk.UserMessage) chatMessage {
	cm := chatMessage{Role: "user"}
	var texts []string
	for _, c := range m.Content {
		switch block := c.(type) {
		case adk.Text:
			texts = append(texts, block.Text)
		case adk.Image:
			cm.Images = append(cm.Images, block.Data)
		}
	}
	cm.Content = strings.Join(texts, "\n")
	return cm
}

func convertAssistantMessage(m adk.AssistantMessage) chatMessage {
	cm := chatMessage{Role: "assistant"}
	var texts []string
	for _, c := range m.Content {
		switch block := c.(type) {
		case adk.Text:
			texts = append(texts, block.Text)
		case adk.Thinking:
			// Include thinking as text for replay
			if block.Thinking != "" {
				texts = append(texts, block.Thinking)
			}
		case adk.ToolCall:
			cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
				ID:   block.ID,
				Type: "function",
				Function: chatFunctionCall{
					Name:      block.Name,
					Arguments: block.Arguments,
				},
			})
		}
	}
	cm.Content = strings.Join(texts, "\n")
	return cm
}

func convertToolResultMessage(m adk.ToolResultMessage) chatMessage {
	var texts []string
	for _, c := range m.Content {
		if tc, ok := c.(adk.Text); ok {
			texts = append(texts, tc.Text)
		}
	}
	return chatMessage{
		Role:    "tool",
		Content: strings.Join(texts, "\n"),
	}
}
