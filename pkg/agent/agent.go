package agent

import (
	"context"
	"sync"

	"mnemos/pkg/ai"
)

// eventListener is a wrapper for event listeners.
type eventListener struct {
	fn EventSink
}

// Agent is the main agent type that manages conversation state and executes prompts.
type Agent struct {
	mu            sync.RWMutex
	state         AgentState
	steeringQueue *pendingMessageQueue
	followUpQueue *pendingMessageQueue
	listeners     map[*eventListener]struct{}
	sessionID     string

	// Run context for cancellation
	runCtx    context.Context
	runCancel context.CancelFunc

	// Configuration
	convertToLLM     func([]AgentMessage) ([]ai.Message, error)
	transformContext func([]AgentMessage) ([]AgentMessage, error)
	streamFn         StreamFunction
	getAPIKey        func(provider string) (string, error)
	beforeToolCall   func(BeforeToolCallContext) (*BeforeToolCallResult, error)
	afterToolCall    func(AfterToolCallContext) (*AfterToolCallResult, error)
	toolExecution    ToolExecutionMode
}

// pendingMessageQueue handles queued messages.
type pendingMessageQueue struct {
	messages []AgentMessage
	mode     QueueMode
}

func newPendingMessageQueue(mode QueueMode) *pendingMessageQueue {
	return &pendingMessageQueue{
		messages: []AgentMessage{},
		mode:     mode,
	}
}

func (q *pendingMessageQueue) enqueue(msg AgentMessage) {
	q.messages = append(q.messages, msg)
}

func (q *pendingMessageQueue) hasItems() bool {
	return len(q.messages) > 0
}

func (q *pendingMessageQueue) drain() []AgentMessage {
	if q.mode == QueueModeAll {
		msgs := q.messages
		q.messages = []AgentMessage{}
		return msgs
	}

	if len(q.messages) == 0 {
		return nil
	}

	msgs := []AgentMessage{q.messages[0]}
	q.messages = q.messages[1:]
	return msgs
}

func (q *pendingMessageQueue) clear() {
	q.messages = []AgentMessage{}
}

// AgentOptions configures a new Agent.
type AgentOptions struct {
	InitialState     *AgentState
	ConvertToLLM     func([]AgentMessage) ([]ai.Message, error)
	TransformContext func([]AgentMessage) ([]AgentMessage, error)
	StreamFn         StreamFunction
	GetAPIKey        func(provider string) (string, error)
	BeforeToolCall   func(BeforeToolCallContext) (*BeforeToolCallResult, error)
	AfterToolCall    func(AfterToolCallContext) (*AfterToolCallResult, error)
	SteeringMode     QueueMode
	FollowUpMode     QueueMode
	SessionID        string
	ToolExecution    ToolExecutionMode
}

// New creates a new Agent with the given options.
func New(opts *AgentOptions) *Agent {
	if opts == nil {
		opts = &AgentOptions{}
	}

	// Default state
	state := AgentState{
		SystemPrompt:     "",
		Model:            nil,
		ThinkingLevel:    ThinkingOff,
		Tools:            []AgentTool{},
		Messages:         []AgentMessage{},
		IsStreaming:      false,
		PendingToolCalls: map[string]bool{},
		ErrorMessage:     "",
	}

	if opts.InitialState != nil {
		state.SystemPrompt = opts.InitialState.SystemPrompt
		state.Model = opts.InitialState.Model
		state.ThinkingLevel = opts.InitialState.ThinkingLevel
		state.Tools = opts.InitialState.Tools
		state.Messages = opts.InitialState.Messages
	}

	// Default configuration
	convertToLLM := opts.ConvertToLLM
	if convertToLLM == nil {
		convertToLLM = defaultConvertToLLM
	}

	steeringMode := opts.SteeringMode
	if steeringMode == "" {
		steeringMode = QueueModeOneAtATime
	}

	followUpMode := opts.FollowUpMode
	if followUpMode == "" {
		followUpMode = QueueModeOneAtATime
	}

	toolExecution := opts.ToolExecution
	if toolExecution == "" {
		toolExecution = ToolExecutionParallel
	}

	return &Agent{
		state:            state,
		steeringQueue:    newPendingMessageQueue(steeringMode),
		followUpQueue:    newPendingMessageQueue(followUpMode),
		listeners:        map[*eventListener]struct{}{},
		convertToLLM:     convertToLLM,
		transformContext: opts.TransformContext,
		streamFn:         opts.StreamFn,
		getAPIKey:        opts.GetAPIKey,
		beforeToolCall:   opts.BeforeToolCall,
		afterToolCall:    opts.AfterToolCall,
		toolExecution:    toolExecution,
		sessionID:        opts.SessionID,
	}
}

// defaultConvertToLLM converts AgentMessages to LLM messages (filters custom messages).
func defaultConvertToLLM(messages []AgentMessage) ([]ai.Message, error) {
	result := make([]ai.Message, 0, len(messages))
	for _, msg := range messages {
		switch m := msg.(type) {
		case *UserAgentMessage:
			result = append(result, ai.UserMessage{Content: m.Content, Timestamp: m.Timestamp})
		case *AssistantAgentMessage:
			result = append(result, ai.AssistantMessage{
				Content:      m.Content,
				API:          m.API,
				Provider:     m.Provider,
				Model:        m.Model,
				ResponseID:   m.ResponseID,
				Usage:        m.Usage,
				StopReason:   m.StopReason,
				ErrorMessage: m.ErrorMessage,
				Timestamp:    m.Timestamp,
			})
		case *ToolResultAgentMessage:
			result = append(result, ai.ToolResultMessage{
				ToolCallID: m.ToolCallID,
				ToolName:   m.ToolName,
				Content:    m.Content,
				IsError:    m.IsError,
				Timestamp:  m.Timestamp,
			})
		}
	}
	return result, nil
}

// Subscribe registers a listener for agent events.
// Returns an unsubscribe function.
func (a *Agent) Subscribe(listener EventSink) (unsubscribe func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	el := &eventListener{fn: listener}
	a.listeners[el] = struct{}{}
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.listeners, el)
	}
}

// State returns the current agent state.
func (a *Agent) State() AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// Model returns the current model.
func (a *Agent) Model() *ai.Model {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state.Model
}

// SetModel sets the current model.
func (a *Agent) SetModel(model *ai.Model) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Model = model
}

// Tools returns the current tools.
func (a *Agent) Tools() []AgentTool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	tools := make([]AgentTool, len(a.state.Tools))
	copy(tools, a.state.Tools)
	return tools
}

// SetTools sets the available tools.
func (a *Agent) SetTools(tools []AgentTool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Tools = tools
}

// Messages returns the conversation messages.
func (a *Agent) Messages() []AgentMessage {
	a.mu.RLock()
	defer a.mu.RUnlock()
	msgs := make([]AgentMessage, len(a.state.Messages))
	copy(msgs, a.state.Messages)
	return msgs
}

// SetMessages sets the conversation messages.
func (a *Agent) SetMessages(messages []AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Messages = messages
}

// IsStreaming returns whether the agent is currently processing.
func (a *Agent) IsStreaming() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state.IsStreaming
}

// PendingToolCalls returns the IDs of currently executing tool calls.
func (a *Agent) PendingToolCalls() map[string]bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make(map[string]bool)
	for k, v := range a.state.PendingToolCalls {
		result[k] = v
	}
	return result
}

// ErrorMessage returns the error message from the last failed run.
func (a *Agent) ErrorMessage() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state.ErrorMessage
}

// SteeringMode gets the steering queue mode.
func (a *Agent) SteeringMode() QueueMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.steeringQueue.mode
}

// SetSteeringMode sets the steering queue mode.
func (a *Agent) SetSteeringMode(mode QueueMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steeringQueue.mode = mode
}

// FollowUpMode gets the follow-up queue mode.
func (a *Agent) FollowUpMode() QueueMode {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.followUpQueue.mode
}

// SetFollowUpMode sets the follow-up queue mode.
func (a *Agent) SetFollowUpMode(mode QueueMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.followUpQueue.mode = mode
}

// Steer queues a message to be injected after the current turn finishes.
func (a *Agent) Steer(message AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steeringQueue.enqueue(message)
}

// FollowUp queues a message to run only after the agent would otherwise stop.
func (a *Agent) FollowUp(message AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.followUpQueue.enqueue(message)
}

// ClearSteeringQueue removes all queued steering messages.
func (a *Agent) ClearSteeringQueue() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steeringQueue.clear()
}

// ClearFollowUpQueue removes all queued follow-up messages.
func (a *Agent) ClearFollowUpQueue() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.followUpQueue.clear()
}

// ClearAllQueues removes all queued messages.
func (a *Agent) ClearAllQueues() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steeringQueue.clear()
	a.followUpQueue.clear()
}

// HasQueuedMessages returns true when either queue has pending messages.
func (a *Agent) HasQueuedMessages() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.steeringQueue.hasItems() || a.followUpQueue.hasItems()
}

// Abort signals abort to the current run, if any.
func (a *Agent) Abort() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.runCancel != nil {
		a.runCancel()
	}
}

// Reset clears the agent state and queues.
func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state.Messages = []AgentMessage{}
	a.state.IsStreaming = false
	a.state.PendingToolCalls = map[string]bool{}
	a.state.ErrorMessage = ""
	a.steeringQueue.clear()
	a.followUpQueue.clear()
}

// emit dispatches an event to all listeners.
func (a *Agent) emit(event AgentEvent) {
	a.mu.RLock()
	listeners := make([]*eventListener, 0, len(a.listeners))
	for listener := range a.listeners {
		listeners = append(listeners, listener)
	}
	a.mu.RUnlock()

	for _, listener := range listeners {
		listener.fn(event)
	}
}

// emitAndProcess processes an event and updates internal state.
func (a *Agent) emitAndProcess(event AgentEvent) {
	// Update internal state based on event
	switch event.Type {
	case EventMessageStart:
		a.mu.Lock()
		a.state.Messages = append(a.state.Messages, event.Message)
		a.mu.Unlock()

	case EventTurnEnd:
		if msg, ok := event.Message_.(*AssistantAgentMessage); ok && msg.ErrorMessage != "" {
			a.mu.Lock()
			a.state.ErrorMessage = msg.ErrorMessage
			a.mu.Unlock()
		}

	case EventToolExecutionStart:
		a.mu.Lock()
		a.state.PendingToolCalls[event.ToolCallID] = true
		a.mu.Unlock()

	case EventToolExecutionEnd:
		a.mu.Lock()
		delete(a.state.PendingToolCalls, event.ToolCallID)
		a.mu.Unlock()

	case EventAgentStart:
		a.mu.Lock()
		a.state.IsStreaming = true
		a.state.ErrorMessage = ""
		a.mu.Unlock()

	case EventAgentEnd:
		a.mu.Lock()
		a.state.IsStreaming = false
		a.mu.Unlock()
	}

	// Emit to listeners
	a.emit(event)
}

// createLoopConfig creates the AgentLoopConfig for the agent loop.
func (a *Agent) createLoopConfig() *AgentLoopConfig {
	return &AgentLoopConfig{
		Model:            a.state.Model,
		Reasoning:        a.state.ThinkingLevel,
		SessionID:        a.sessionID,
		ToolExecution:    a.toolExecution,
		ConvertToLLM:     a.convertToLLM,
		TransformContext: a.transformContext,
		GetAPIKey:        a.getAPIKey,
		GetSteeringMessages: func() ([]AgentMessage, error) {
			return a.steeringQueue.drain(), nil
		},
		GetFollowUpMessages: func() ([]AgentMessage, error) {
			return a.followUpQueue.drain(), nil
		},
		BeforeToolCall: a.beforeToolCall,
		AfterToolCall:  a.afterToolCall,
	}
}

// createContextSnapshot creates an AgentContext snapshot.
func (a *Agent) createContextSnapshot() AgentContext {
	a.mu.RLock()
	defer a.mu.RUnlock()

	tools := make([]AgentTool, len(a.state.Tools))
	for i, t := range a.state.Tools {
		tools[i] = t
	}

	return AgentContext{
		SystemPrompt: a.state.SystemPrompt,
		Messages:     a.state.Messages,
		Tools:        tools,
	}
}
