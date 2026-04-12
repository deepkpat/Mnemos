package ai

// StreamEventType describes the type of StreamEvent
type StreamEventType string

const (
	StreamEventTypeStart         StreamEventType = "start"
	StreamEventTypeTextStart     StreamEventType = "text_start"
	StreamEventTypeTextDelta     StreamEventType = "text_delta"
	StreamEventTypeTextEnd       StreamEventType = "text_end"
	StreamEventTypeThinkingStart StreamEventType = "thinking_start"
	StreamEventTypeThinkingDelta StreamEventType = "thinking_delta"
	StreamEventTypeThinkingEnd   StreamEventType = "thinking_end"
	StreamEventTypeToolCallStart StreamEventType = "tool_call_start"
	StreamEventTypeToolCallDelta StreamEventType = "tool_call_delta"
	StreamEventTypeToolCallEnd   StreamEventType = "tool_call_end"
	StreamEventTypeDone          StreamEventType = "done"
	StreamEventTypeError         StreamEventType = "error"
)

type StreamEvent struct {
	Type StreamEventType
}

type Stream interface {
	Stream() <-chan StreamEvent
	Result() (AssistantMessage, error)
}
