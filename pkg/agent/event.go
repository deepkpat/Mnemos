package agent

import "mnemos/pkg/ai"

// AgentEventType identifies the kind of agent event.
type AgentEventType string

const (
	// Agent lifecycle
	EventAgentStart AgentEventType = "agent_start"
	EventAgentEnd   AgentEventType = "agent_end"

	// Turn lifecycle - one assistant response + any tool calls/results
	EventTurnStart AgentEventType = "turn_start"
	EventTurnEnd   AgentEventType = "turn_end"

	// Message lifecycle - emitted for user, assistant, and toolResult messages
	EventMessageStart  AgentEventType = "message_start"
	EventMessageUpdate AgentEventType = "message_update"
	EventMessageEnd    AgentEventType = "message_end"

	// Tool execution lifecycle
	EventToolExecutionStart  AgentEventType = "tool_execution_start"
	EventToolExecutionUpdate AgentEventType = "tool_execution_update"
	EventToolExecutionEnd    AgentEventType = "tool_execution_end"
)

// AgentEvent represents an event emitted by the agent loop.
type AgentEvent struct {
	Type AgentEventType

	// For message events
	Message AgentMessage

	// For message_update events
	AssistantMessageEvent *ai.Event

	// For turn_end events
	Message_    AgentMessage
	ToolResults []ToolResultAgentMessage

	// For tool_execution events
	ToolCallID string
	ToolName   string
	Args       map[string]any

	// For tool_execution_update events
	PartialResult any

	// For tool_execution_end events
	Result  any // Can be *AgentToolResult[any] or []ai.Content
	IsError bool

	// For agent_end events
	Messages []AgentMessage
}

// EventSink is the function signature for handling agent events.
type EventSink func(event AgentEvent)

// String returns a string representation of the event type.
func (e AgentEventType) String() string {
	return string(e)
}
