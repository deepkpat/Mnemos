package ollama

import (
	"encoding/json"
	"strings"

	"mnemos/pkg/ai"
)

// --- Request payload types ---

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Tools       []chatTool    `json:"tools,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Images     []string       `json:"images,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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

// buildPayload converts the unified ai.Context into an Ollama chat completion request.
func buildPayload(model *ai.Model, conv *ai.Context, opts *ai.StreamOptions) chatRequest {
	req := chatRequest{
		Model:  model.ID,
		Stream: true,
	}

	if opts != nil {
		req.Temperature = opts.Temperature
		if opts.MaxTokens > 0 {
			req.MaxTokens = opts.MaxTokens
		}
	}
	if req.MaxTokens == 0 && model.MaxTokens > 0 {
		req.MaxTokens = model.MaxTokens
	}

	// System prompt
	if conv.SystemPrompt != "" {
		req.Messages = append(req.Messages, chatMessage{
			Role:    "system",
			Content: conv.SystemPrompt,
		})
	}

	// Conversation messages
	for _, msg := range conv.Messages {
		switch m := msg.(type) {
		case ai.UserMessage:
			req.Messages = append(req.Messages, convertUserMessage(m))
		case ai.AssistantMessage:
			req.Messages = append(req.Messages, convertAssistantMessage(m))
		case ai.ToolResultMessage:
			req.Messages = append(req.Messages, convertToolResultMessage(m))
		}
	}

	// Tools
	for _, tool := range conv.Tools {
		req.Tools = append(req.Tools, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}

	return req
}

func convertUserMessage(m ai.UserMessage) chatMessage {
	cm := chatMessage{Role: "user"}
	var texts []string
	for _, c := range m.Content {
		switch block := c.(type) {
		case ai.TextContent:
			texts = append(texts, block.Text)
		case ai.ImageContent:
			cm.Images = append(cm.Images, block.Data)
		}
	}
	cm.Content = strings.Join(texts, "\n")
	return cm
}

func convertAssistantMessage(m ai.AssistantMessage) chatMessage {
	cm := chatMessage{Role: "assistant"}
	var texts []string
	for _, c := range m.Content {
		switch block := c.(type) {
		case ai.TextContent:
			texts = append(texts, block.Text)
		case ai.ThinkingContent:
			// Include thinking as text for replay
			if block.Thinking != "" {
				texts = append(texts, block.Thinking)
			}
		case ai.ToolCall:
			args, _ := json.Marshal(block.Arguments)
			cm.ToolCalls = append(cm.ToolCalls, chatToolCall{
				ID:   block.ID,
				Type: "function",
				Function: chatFunctionCall{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}
	cm.Content = strings.Join(texts, "\n")
	return cm
}

func convertToolResultMessage(m ai.ToolResultMessage) chatMessage {
	var texts []string
	for _, c := range m.Content {
		if tc, ok := c.(ai.TextContent); ok {
			texts = append(texts, tc.Text)
		}
	}
	return chatMessage{
		Role:       "tool",
		Content:    strings.Join(texts, "\n"),
		ToolCallID: m.ToolCallID,
	}
}
