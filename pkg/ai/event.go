package ai

// EventType describe the type of event in a stream
type EventType string

const (
	EventStart         EventType = "start"
	EventTextStart     EventType = "text_start"
	EventTextDelta     EventType = "text_delta"
	EventTextEnd       EventType = "text_end"
	EventThinkingStart EventType = "thinking_start"
	EventThinkingDelta EventType = "thinking_delta"
	EventThinkingEnd   EventType = "thinking_end"
	EventToolCallStart EventType = "toolcall_start"
	EventToolCallDelta EventType = "toolcall_delta"
	EventToolCallEnd   EventType = "toolcall_end"
	EventDone          EventType = "done"
	EventError         EventType = "error"
)

// StartEvent is the first event emitted when streaming begins
type StartEvent struct {
	Partial *AssistantMessage `json:"partial"`
}

func (e StartEvent) EventType() EventType { return EventStart }

// TextStartEvent emitted when a text content block begins
type TextStartEvent struct {
	ContentIndex int               `json:"content_index"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e TextStartEvent) EventType() EventType { return EventTextStart }

// TextDeltaEvent emitted as text content streams in (can be many)
type TextDeltaEvent struct {
	ContentIndex int               `json:"content_index"`
	Delta        string            `json:"delta"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e TextDeltaEvent) EventType() EventType { return EventTextDelta }

// TextEndEvent emitted when a text content block completes
type TextEndEvent struct {
	ContentIndex int               `json:"content_index"`
	Content      string            `json:"content"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e TextEndEvent) EventType() EventType { return EventTextEnd }

// ThinkingStartEvent emitted when a thinking content block begins
type ThinkingStartEvent struct {
	ContentIndex int               `json:"content_index"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e ThinkingStartEvent) EventType() EventType { return EventThinkingStart }

// ThinkingDeltaEvent emitted as thinking content streams in (can be many)
type ThinkingDeltaEvent struct {
	ContentIndex int               `json:"content_index"`
	Delta        string            `json:"delta"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e ThinkingDeltaEvent) EventType() EventType { return EventThinkingDelta }

// ThinkingEndEvent emitted when a thinking content block completes
type ThinkingEndEvent struct {
	ContentIndex int               `json:"content_index"`
	Content      string            `json:"content"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e ThinkingEndEvent) EventType() EventType { return EventThinkingEnd }

// ToolCallStartEvent emitted when a tool call block begins
// key: name is available in first shot, arguments come in deltas
type ToolCallStartEvent struct {
	ContentIndex int               `json:"content_index"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e ToolCallStartEvent) EventType() EventType { return EventToolCallStart }

// ToolCallDeltaEvent emitted as tool call arguments stream in (JSON chunks)
type ToolCallDeltaEvent struct {
	ContentIndex int               `json:"content_index"`
	Delta        string            `json:"delta"` // Raw JSON text chunk
	Partial      *AssistantMessage `json:"partial"`
}

func (e ToolCallDeltaEvent) EventType() EventType { return EventToolCallDelta }

// ToolCallEndEvent emitted when a tool call completes
type ToolCallEndEvent struct {
	ContentIndex int               `json:"content_index"`
	ToolCall     ToolCall          `json:"tool_call"`
	Partial      *AssistantMessage `json:"partial"`
}

func (e ToolCallEndEvent) EventType() EventType { return EventToolCallEnd }

// DoneEvent emitted when streaming completes successfully
type DoneEvent struct {
	Reason  StopReason        `json:"reason"`
	Message *AssistantMessage `json:"message"`
}

func (e DoneEvent) EventType() EventType { return EventDone }

// ErrorEvent emitted when an error occurs
type ErrorEvent struct {
	Reason StopReason        `json:"reason"`
	Error  *AssistantMessage `json:"error"`
}

func (e ErrorEvent) EventType() EventType { return EventError }

// Event interface
type Event interface {
	EventType() EventType
}
