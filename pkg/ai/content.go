package ai

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
