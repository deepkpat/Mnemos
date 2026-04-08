package ai

import "time"

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopReasonStop    StopReason = "stop"    // Natural completion
	StopReasonLength  StopReason = "length"  // Hit max token limit
	StopReasonToolUse StopReason = "toolUse" // Model wants to call tool(s)
	StopReasonError   StopReason = "error"   // Provider/network error
	StopReasonAborted StopReason = "aborted" // Cancelled by caller
)

// ThinkingLevel controls how much reasoning effort the model should use.
type ThinkingLevel string

const (
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
)

// InputModality describes what kind of input a model accepts.
type InputModality string

const (
	InputText  InputModality = "text"
	InputImage InputModality = "image"
)

// --- Content Blocks ---

// ContentType is a discriminator for content blocks.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeThinking ContentType = "thinking"
	ContentTypeImage    ContentType = "image"
	ContentTypeToolCall ContentType = "toolCall"
)

// TextContent represents plain text output from the model.
type TextContent struct {
	Text          string `json:"text"`
	TextSignature string `json:"textSignature,omitempty"`
}

func (c TextContent) ContentType() ContentType { return ContentTypeText }

// ThinkingContent represents chain-of-thought reasoning.
type ThinkingContent struct {
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinkingSignature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

func (c ThinkingContent) ContentType() ContentType { return ContentTypeThinking }

// ImageContent represents base64-encoded image data.
type ImageContent struct {
	Data     string `json:"data"`     // base64 encoded
	MimeType string `json:"mimeType"` // e.g. "image/png"
}

func (c ImageContent) ContentType() ContentType { return ContentTypeImage }

// ToolCall represents the model requesting a tool execution.
type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thoughtSignature,omitempty"`
}

func (c ToolCall) ContentType() ContentType { return ContentTypeToolCall }

// Content is the interface implemented by all content block types.
type Content interface {
	ContentType() ContentType
}

// Role is the role of a message in a conversation.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "toolResult"
)

// --- Messages ---

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

// --- Usage ---

// Usage tracks token consumption and associated costs.
type Usage struct {
	Input       int       `json:"input"`
	Output      int       `json:"output"`
	CacheRead   int       `json:"cacheRead"`
	CacheWrite  int       `json:"cacheWrite"`
	TotalTokens int       `json:"totalTokens"`
	Cost        UsageCost `json:"cost"`
}

// UsageCost tracks the dollar cost breakdown.
type UsageCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
	Total      float64 `json:"total"`
}

// ZeroUsage returns a Usage with all fields zeroed.
func ZeroUsage() Usage {
	return Usage{}
}

// --- Model ---

// ModelCost describes pricing per million tokens.
type ModelCost struct {
	Input      float64 `json:"input"`      // $/million tokens
	Output     float64 `json:"output"`     // $/million tokens
	CacheRead  float64 `json:"cacheRead"`  // $/million tokens
	CacheWrite float64 `json:"cacheWrite"` // $/million tokens
}

// Model describes a specific LLM endpoint.
type Model struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	API           string            `json:"api"`
	Provider      string            `json:"provider"`
	BaseURL       string            `json:"baseUrl"`
	Reasoning     bool              `json:"reasoning"`
	Input         []InputModality   `json:"input"`
	Cost          ModelCost         `json:"cost"`
	ContextWindow int               `json:"contextWindow"`
	MaxTokens     int               `json:"maxTokens"`
	Headers       map[string]string `json:"headers,omitempty"`
}

// CalculateCost computes costs from token usage and model pricing.
func (m *Model) CalculateCost(u *Usage) {
	u.Cost.Input = (m.Cost.Input / 1_000_000) * float64(u.Input)
	u.Cost.Output = (m.Cost.Output / 1_000_000) * float64(u.Output)
	u.Cost.CacheRead = (m.Cost.CacheRead / 1_000_000) * float64(u.CacheRead)
	u.Cost.CacheWrite = (m.Cost.CacheWrite / 1_000_000) * float64(u.CacheWrite)
	u.Cost.Total = u.Cost.Input + u.Cost.Output + u.Cost.CacheRead + u.Cost.CacheWrite
}

// --- Tool ---

// Tool defines a function the model can call.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// --- Context ---

// Context is the full conversation state sent to the LLM.
type Context struct {
	SystemPrompt string    `json:"systemPrompt,omitempty"`
	Messages     []Message `json:"messages"`
	Tools        []Tool    `json:"tools,omitempty"`
}

// --- Stream Options ---

// StreamOptions configures a streaming request.
type StreamOptions struct {
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      int               `json:"maxTokens,omitempty"`
	APIKey         string            `json:"apiKey,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Reasoning      ThinkingLevel     `json:"reasoning,omitempty"`
	ThinkingBudget int               `json:"thinkingBudget,omitempty"`
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
