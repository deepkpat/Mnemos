package adk

import "time"

// Role is the role of a message in a conversation
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
)

// UserMessage is a message from the user
type UserMessage struct {
	Content   []Content `json:"content"` // [](Text|Image)
	Timestamp time.Time `json:"timestamp"`
}

func (m UserMessage) Role() Role { return RoleUser }

// StopReason indicates why the model stopped generating
type StopReason string

const (
	StopReasonStop    StopReason = "stop"     // natural completion
	StopReasonLength  StopReason = "length"   // hit max token limit
	StopReasonToolUse StopReason = "tool_use" // model wants to call tool(s)
	StopReasonError   StopReason = "error"    // provider/network error
	StopReasonAborted StopReason = "aborted"  // cancelled by caller
)

// AssistantMessage is a response from the model
type AssistantMessage struct {
	Content      []Content  `json:"content"` // [](Text|Thinking|ToolCall)
	API          string     `json:"api"`
	Provider     string     `json:"provider"`
	Model        string     `json:"model"`
	ResponseID   string     `json:"response_id,omitempty"`
	Usage        Usage      `json:"usage"`
	StopReason   StopReason `json:"stop_reason"`
	ErrorMessage string     `json:"error_message,omitempty"`
	Timestamp    time.Time  `json:"timestamp"`
}

func (m AssistantMessage) Role() Role { return RoleAssistant }

// ToolResultMessage carries the result of a tool execution back to the model
type ToolResultMessage struct {
	ToolCallID string    `json:"tool_call_id"`
	ToolName   string    `json:"tool_name"`
	Content    []Content `json:"content"` // [](Text|Image)
	IsError    bool      `json:"is_error"`
	Timestamp  time.Time `json:"timestamp"`
}

func (m ToolResultMessage) Role() Role { return RoleToolResult }

// Message is the interface implemented by all message types
type Message interface {
	Role() Role
}

// NewUserMessage creates a UserMessage from a plain string.
func NewUserMessage(text string) UserMessage {
	return UserMessage{
		Content:   []Content{Text{Text: text}},
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
