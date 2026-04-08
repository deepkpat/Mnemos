package ai

import "time"

// Role is the role of a message in a conversation.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "toolResult"
)

// UserMessage is a message from the user.
type UserMessage struct {
	// Content is either a plain string (stored as a single TextContent)
	// or a slice of Content blocks (TextContent | ImageContent).
	Content   []Content `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

func (m UserMessage) Role() Role { return RoleUser }

// AssistantMessage is a response from the model.
type AssistantMessage struct {
	Content      []Content  `json:"content"` // TextContent | ThinkingContent | ToolCall
	API          string     `json:"api"`
	Provider     string     `json:"provider"`
	Model        string     `json:"model"`
	ResponseID   string     `json:"responseId,omitempty"`
	Usage        Usage      `json:"usage"`
	StopReason   StopReason `json:"stopReason"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	Timestamp    time.Time  `json:"timestamp"`
}

func (m AssistantMessage) Role() Role { return RoleAssistant }

// ToolResultMessage carries the result of a tool execution back to the model.
type ToolResultMessage struct {
	ToolCallID string    `json:"toolCallId"`
	ToolName   string    `json:"toolName"`
	Content    []Content `json:"content"` // TextContent | ImageContent
	IsError    bool      `json:"isError"`
	Timestamp  time.Time `json:"timestamp"`
}

func (m ToolResultMessage) Role() Role { return RoleToolResult }

// Message is the interface implemented by all message types.
type Message interface {
	Role() Role
}

// NewUserMessage creates a UserMessage from a plain string.
func NewUserMessage(text string) UserMessage {
	return UserMessage{
		Content:   []Content{TextContent{Text: text}},
		Timestamp: time.Now(),
	}
}

// NewAssistantError creates an AssistantMessage representing an error.
func NewAssistantError(model *Model, err error) AssistantMessage {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return AssistantMessage{
		Content:      nil,
		API:          model.API,
		Provider:     model.Provider,
		Model:        model.ID,
		Usage:        ZeroUsage(),
		StopReason:   StopReasonError,
		ErrorMessage: errMsg,
		Timestamp:    time.Now(),
	}
}
