package adk

// ContentType describes the type of message content
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeThinking ContentType = "thinking"
	ContentTypeImage    ContentType = "image"
	ContentTypeToolCall ContentType = "tool_call"
)

// Text represents plain text output from the model
type Text struct {
	Text          string `json:"text"`
	TextSignature string `json:"text_signature,omitempty"`
}

func (c Text) ContentType() ContentType { return ContentTypeText }

// Thinking represents chain of thought reasoning
type Thinking struct {
	Thinking          string `json:"thinking"`
	ThinkingSignature string `json:"thinking_signature,omitempty"`
	Redacted          bool   `json:"redacted,omitempty"`
}

func (c Thinking) ContentType() ContentType { return ContentTypeThinking }

// Image represents base64 encoded image data
type Image struct {
	Data     string `json:"data"`      // base64 encoded
	MimeType string `json:"mime_type"` // e.g. "image/png"
}

func (c Image) ContentType() ContentType { return ContentTypeImage }

// ToolCall represents the model requesting a tool execution.
type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	ThoughtSignature string         `json:"thought_signature,omitempty"`
}

func (c ToolCall) ContentType() ContentType { return ContentTypeToolCall }

// Content is the interface implemented by all content block types
type Content interface {
	ContentType() ContentType
}
