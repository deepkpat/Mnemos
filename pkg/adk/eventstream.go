package adk

import "sync"

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
	if e.EventType() == EventDone {
		s.result = *e.(DoneEvent).Message
	} else if e.EventType() == EventError {
		s.result = *e.(ErrorEvent).Error
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
func (s *EventStream) Events() <-chan Event {
	return s.ch
}

// Result blocks until the stream completes and returns the final AssistantMessage.
func (s *EventStream) Result() AssistantMessage {
	for range s.ch {
		// drain
	}
	return s.result
}

// PushDone is a convenience method that pushes a done event and closes the stream.
func (s *EventStream) PushDone(reason StopReason, msg AssistantMessage) {
	s.Push(DoneEvent{
		Reason:  reason,
		Message: &msg,
	})
	s.Close()
}

// PushError is a convenience method that pushes an error event and closes the stream.
func (s *EventStream) PushError(reason StopReason, msg AssistantMessage) {
	s.Push(ErrorEvent{
		Reason: reason,
		Error:  &msg,
	})
	s.Close()
}
