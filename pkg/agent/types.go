package agent

import (
	"fmt"
	"time"

	"mnemos/pkg/ai"
)

// ThinkingLevel controls the model's reasoning effort.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
)

// ToolExecutionMode controls how multiple tool calls in a single assistant message are executed.
type ToolExecutionMode string

const (
	ToolExecutionSequential ToolExecutionMode = "sequential"
	ToolExecutionParallel   ToolExecutionMode = "parallel"
)

// QueueMode controls how queued messages are drained.
type QueueMode string

const (
	QueueModeAll        QueueMode = "all"
	QueueModeOneAtATime QueueMode = "one-at-a-time"
)

// AgentMessage is a message in the agent conversation.
// It can be a standard LLM message (User, Assistant, ToolResult) or a custom message.
type AgentMessage interface {
	isAgentMessage()
}

// UserAgentMessage represents input from the user.
type UserAgentMessage struct {
	Content   []ai.Content `json:"content"`
	Timestamp time.Time    `json:"timestamp"`
}

func (m *UserAgentMessage) isAgentMessage() {}

// AssistantAgentMessage represents a response from the model.
type AssistantAgentMessage struct {
	Content      []ai.Content  `json:"content"`
	API          string        `json:"api"`
	Provider     string        `json:"provider"`
	Model        string        `json:"model"`
	ResponseID   string        `json:"responseId,omitempty"`
	Usage        ai.Usage      `json:"usage"`
	StopReason   ai.StopReason `json:"stopReason"`
	ErrorMessage string        `json:"errorMessage,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
}

func (m *AssistantAgentMessage) isAgentMessage() {}

// ToolResultAgentMessage carries a tool execution result back to the model.
type ToolResultAgentMessage struct {
	ToolCallID string       `json:"toolCallId"`
	ToolName   string       `json:"toolName"`
	Content    []ai.Content `json:"content"`
	IsError    bool         `json:"isError"`
	Timestamp  time.Time    `json:"timestamp"`
}

func (m *ToolResultAgentMessage) isAgentMessage() {}

// NewUserMessage creates a UserAgentMessage from a plain string.
func NewUserMessage(text string) *UserAgentMessage {
	return &UserAgentMessage{
		Content:   []ai.Content{ai.TextContent{Text: text}},
		Timestamp: time.Now(),
	}
}

// NewAssistantMessage creates an AssistantAgentMessage from ai.AssistantMessage.
func NewAssistantMessage(msg ai.AssistantMessage) *AssistantAgentMessage {
	return &AssistantAgentMessage{
		Content:      msg.Content,
		API:          msg.API,
		Provider:     msg.Provider,
		Model:        msg.Model,
		ResponseID:   msg.ResponseID,
		Usage:        msg.Usage,
		StopReason:   msg.StopReason,
		ErrorMessage: msg.ErrorMessage,
		Timestamp:    msg.Timestamp,
	}
}

// NewToolResultMessage creates a ToolResultAgentMessage.
func NewToolResultMessage(toolCallID, toolName string, content []ai.Content, isError bool) *ToolResultAgentMessage {
	return &ToolResultAgentMessage{
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Content:    content,
		IsError:    isError,
		Timestamp:  time.Now(),
	}
}

// ToLLMMessage converts an AgentMessage to the equivalent ai.Message for LLM calls.
func ToLLMMessage(msg AgentMessage) ai.Message {
	switch m := msg.(type) {
	case *UserAgentMessage:
		return ai.UserMessage{Content: m.Content, Timestamp: m.Timestamp}
	case *AssistantAgentMessage:
		return ai.AssistantMessage{
			Content:      m.Content,
			API:          m.API,
			Provider:     m.Provider,
			Model:        m.Model,
			ResponseID:   m.ResponseID,
			Usage:        m.Usage,
			StopReason:   m.StopReason,
			ErrorMessage: m.ErrorMessage,
			Timestamp:    m.Timestamp,
		}
	case *ToolResultAgentMessage:
		return ai.ToolResultMessage{
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
			Content:    m.Content,
			IsError:    m.IsError,
			Timestamp:  m.Timestamp,
		}
	default:
		return nil
	}
}

// AgentState holds the agent's runtime state.
type AgentState struct {
	SystemPrompt     string          `json:"systemPrompt,omitempty"`
	Model            *ai.Model       `json:"model"`
	ThinkingLevel    ThinkingLevel   `json:"thinkingLevel"`
	Tools            []AgentTool     `json:"tools"`
	Messages         []AgentMessage  `json:"messages"`
	IsStreaming      bool            `json:"isStreaming"`
	PendingToolCalls map[string]bool `json:"pendingToolCalls"`
	ErrorMessage     string          `json:"errorMessage,omitempty"`
}

// AgentToolResult represents the result of a tool execution.
type AgentToolResult[T any] struct {
	Content []ai.Content `json:"content"`
	Details T            `json:"details"`
}

// ToolUpdateFunc is the callback type for streaming tool execution updates.
type ToolUpdateFunc[T any] func(partialResult *AgentToolResult[T])

// AgentTool is the interface implemented by all agent tools.
type AgentTool interface {
	ToolName() string
	ToolDescription() string
	ToolLabel() string
	ToolParameters() map[string]any
	Execute(toolCallID string, args map[string]any, signal <-chan struct{}, onUpdate func(*AgentToolResult[any])) (*AgentToolResult[any], error)
}

// SimpleTool is a basic tool implementation that uses any for parameters and results.
// It implements AgentTool interface.
type SimpleTool struct {
	Name        string
	Description string
	Label       string
	Parameters  map[string]any // JSON Schema
	Run         func(toolCallID string, args map[string]any, signal <-chan struct{}, onUpdate func(*AgentToolResult[any])) (*AgentToolResult[any], error)
}

func (t *SimpleTool) ToolName() string               { return t.Name }
func (t *SimpleTool) ToolDescription() string        { return t.Description }
func (t *SimpleTool) ToolLabel() string              { return t.Label }
func (t *SimpleTool) ToolParameters() map[string]any { return t.Parameters }
func (t *SimpleTool) Execute(toolCallID string, args map[string]any, signal <-chan struct{}, onUpdate func(*AgentToolResult[any])) (*AgentToolResult[any], error) {
	return t.Run(toolCallID, args, signal, onUpdate)
}

// AgentContext is a snapshot of the agent context at a point in time.
type AgentContext struct {
	SystemPrompt string         `json:"systemPrompt,omitempty"`
	Messages     []AgentMessage `json:"messages"`
	Tools        []AgentTool    `json:"tools,omitempty"`
}

// BeforeToolCallContext is passed to the beforeToolCall hook.
type BeforeToolCallContext struct {
	AssistantMessage *AssistantAgentMessage
	ToolCall         *ai.ToolCall
	Args             map[string]any
	Context          *AgentContext
}

// BeforeToolCallResult is returned from the beforeToolCall hook.
type BeforeToolCallResult struct {
	Block  bool
	Reason string
}

// AfterToolCallContext is passed to the afterToolCall hook.
type AfterToolCallContext struct {
	AssistantMessage *AssistantAgentMessage
	ToolCall         *ai.ToolCall
	Args             map[string]any
	Result           *AgentToolResult[any]
	IsError          bool
	Context          *AgentContext
}

// AfterToolCallResult can override parts of the executed tool result.
type AfterToolCallResult struct {
	Content *[]ai.Content
	Details any
	IsError *bool
}

// ContextDistiller transforms a message list before sending to the LLM.
// It can be used to compress, filter, or otherwise process messages.
type ContextDistiller interface {
	Distill(messages []AgentMessage) []AgentMessage
}

// IdentityContextDistiller is a no-op distiller that returns messages unchanged.
type IdentityContextDistiller struct{}

func (IdentityContextDistiller) Distill(messages []AgentMessage) []AgentMessage {
	return messages
}

// SimpleDistiller removes read/write/edit tool calls and replaces them with
// the actual file contents.
type SimpleDistiller struct {
	// FileReader is an optional function to read file contents.
	// If not set, the distiller will just collect paths without reading.
	FileReader func(path string) (string, error)
}

// toolNames to filter
var simpleDistillerFilterToolNames = map[string]bool{
	"read":  true,
	"write": true,
	"edit":  true,
}

func (d SimpleDistiller) Distill(messages []AgentMessage) []AgentMessage {
	// First pass: filter tool result messages
	result := make([]AgentMessage, 0, len(messages))
	modifiedFiles := make(map[string]bool) // path -> true

	for _, msg := range messages {
		switch m := msg.(type) {
		case *ToolResultAgentMessage:
			// Skip read, write, edit tool results
			if simpleDistillerFilterToolNames[m.ToolName] {
				continue
			}
			result = append(result, m)
		case *AssistantAgentMessage:
			// Extract file paths from tool calls and filter them out
			filteredContent := make([]ai.Content, 0, len(m.Content))
			for _, c := range m.Content {
				if tc, ok := c.(ai.ToolCall); ok {
					if simpleDistillerFilterToolNames[tc.Name] {
						if path, ok := tc.Arguments["path"].(string); ok {
							modifiedFiles[path] = true
						}
						continue // Skip this tool call
					}
					filteredContent = append(filteredContent, c)
				} else {
					filteredContent = append(filteredContent, c)
				}
			}
			// Add assistant message without the filtered tool calls
			m.Content = filteredContent
			result = append(result, m)
		default:
			result = append(result, m)
		}
	}

	// If we have a file reader and modified files, read them and add as tool result messages
	if d.FileReader != nil && len(modifiedFiles) > 0 {
		for path := range modifiedFiles {
			content, err := d.FileReader(path)
			var fileContent []ai.Content
			if err != nil {
				fileContent = []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error reading file: %v", err)}}
			} else {
				fileContent = []ai.Content{ai.TextContent{Text: content}}
			}
			// Add as a tool result message
			result = append(result, &ToolResultAgentMessage{
				ToolCallID: "distilled-read-" + path,
				ToolName: "read",
				Content:  fileContent,
				IsError:  err != nil,
				Timestamp: time.Now(),
			})
		}
	}

	return result
}

// AgentLoopConfig is the configuration for the agent loop.
type AgentLoopConfig struct {
	Model               *ai.Model
	Reasoning           ThinkingLevel
	SessionID           string
	ToolExecution       ToolExecutionMode
	ConvertToLLM        func(messages []AgentMessage) ([]ai.Message, error)
	TransformContext    func(messages []AgentMessage) ([]AgentMessage, error)
	ContextDistiller    ContextDistiller
	GetAPIKey           func(provider string) (string, error)
	GetSteeringMessages func() ([]AgentMessage, error)
	GetFollowUpMessages func() ([]AgentMessage, error)
	BeforeToolCall      func(ctx BeforeToolCallContext) (*BeforeToolCallResult, error)
	AfterToolCall       func(ctx AfterToolCallContext) (*AfterToolCallResult, error)
}
