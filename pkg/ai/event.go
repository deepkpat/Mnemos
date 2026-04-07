package ai

import "sync"

// EventType identifies the kind of streaming event.
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

// Event represents a single streaming event from a provider.
//
// Event protocol:
//
//	start
//	├── text_start → text_delta* → text_end
//	├── thinking_start → thinking_delta* → thinking_end
//	├── toolcall_start → toolcall_delta* → toolcall_end
//	└── (repeat for each content block)
//	done { Message } | error { Error }
type Event struct {
	Type         EventType
	ContentIndex int              // Which content block this event relates to
	Delta        string           // Incremental text for delta events
	Content      string           // Full content for end events
	ToolCall     *ToolCall        // Completed tool call for toolcall_end
	Reason       StopReason       // For done/error events
	Message      AssistantMessage // Final message for done events
	Error        AssistantMessage // Error message for error events
	Partial      AssistantMessage // In-progress message snapshot
}

// EventStream is a channel-based async stream of Events.
// Consumers read from Events(). Producers call Push() and must call Close() when done.
type EventStream struct {
	ch     chan Event
	once   sync.Once
	done   chan struct{}
	result AssistantMessage
	err    error
}

// NewEventStream creates a new EventStream with the given buffer size.
func NewEventStream(bufSize int) *EventStream {
	if bufSize < 1 {
		bufSize = 64
	}
	return &EventStream{
		ch:   make(chan Event, bufSize),
		done: make(chan struct{}),
	}
}

// Push sends an event into the stream. Safe to call from any goroutine.
// Must not be called after Close().
func (s *EventStream) Push(e Event) {
	select {
	case s.ch <- e:
	case <-s.done:
	}

	// Capture terminal events
	if e.Type == EventDone {
		s.result = e.Message
	} else if e.Type == EventError {
		s.result = e.Error
	}
}

// Close signals that no more events will be sent. Must be called exactly once.
func (s *EventStream) Close() {
	s.once.Do(func() {
		close(s.done)
		close(s.ch)
	})
}

// Events returns the read-only channel for consuming events.
//
// Usage:
//
//	for event := range stream.Events() { ... }
func (s *EventStream) Events() <-chan Event {
	return s.ch
}

// Result blocks until the stream completes and returns the final AssistantMessage.
// This is the non-streaming equivalent — it drains all events and returns the result.
func (s *EventStream) Result() AssistantMessage {
	for range s.ch {
		// drain
	}
	return s.result
}

// PushDone is a convenience method that pushes a done event and closes the stream.
func (s *EventStream) PushDone(reason StopReason, msg AssistantMessage) {
	s.Push(Event{
		Type:    EventDone,
		Reason:  reason,
		Message: msg,
	})
	s.Close()
}

// PushError is a convenience method that pushes an error event and closes the stream.
func (s *EventStream) PushError(reason StopReason, msg AssistantMessage) {
	s.Push(Event{
		Type:   EventError,
		Reason: reason,
		Error:  msg,
	})
	s.Close()
}
