package agent

import (
	"context"
	"errors"
	"sync"

	"mnemos/pkg/ai"
)

// errNoMessages is returned when there are no messages to continue from.
var errNoMessages = errors.New("agent: no messages to continue from")

// errCannotContinueFromAssistant is returned when continuing from an assistant message.
var errCannotContinueFromAssistant = errors.New("agent: cannot continue from message role: assistant")

// ErrAgentBusy is returned when the agent is already processing a prompt.
var ErrAgentBusy = errors.New("agent is already processing a prompt")

// Prompt starts a new prompt with the given input.
func (a *Agent) Prompt(input string) error {
	return a.PromptWithImages(input, nil)
}

// PromptWithImages starts a new prompt with text and optional images.
func (a *Agent) PromptWithImages(input string, images []ai.Content) error {
	return a.PromptMessages([]AgentMessage{NewUserMessage(input)})
}

// PromptMessage starts a new prompt with an agent message.
func (a *Agent) PromptMessage(msg AgentMessage) error {
	return a.PromptMessages([]AgentMessage{msg})
}

// PromptMessages starts a new prompt with multiple agent messages.
func (a *Agent) PromptMessages(messages []AgentMessage) error {
	a.mu.Lock()
	if a.runCtx != nil {
		a.mu.Unlock()
		return ErrAgentBusy
	}

	// Create context with cancel for this run
	ctx, cancel := context.WithCancel(context.Background())
	a.runCtx = ctx
	a.runCancel = cancel

	a.mu.Unlock()

	// Run in background
	go func() {
		a.runPrompt(messages, ctx)
		a.mu.Lock()
		a.runCtx = nil
		a.runCancel = nil
		a.state.IsStreaming = false
		a.mu.Unlock()
	}()

	return nil
}

// Continue continues from the current transcript.
func (a *Agent) Continue() error {
	a.mu.Lock()
	if a.runCtx != nil {
		a.mu.Unlock()
		return ErrAgentBusy
	}

	if len(a.state.Messages) == 0 {
		a.mu.Unlock()
		return errNoMessages
	}

	lastMsg := a.state.Messages[len(a.state.Messages)-1]
	if _, ok := lastMsg.(*AssistantAgentMessage); ok {
		a.mu.Unlock()
		return errCannotContinueFromAssistant
	}

	// Create context with cancel for this run
	ctx, cancel := context.WithCancel(context.Background())
	a.runCtx = ctx
	a.runCancel = cancel

	a.mu.Unlock()

	// Run in background
	go func() {
		a.runContinuation(ctx)
		a.mu.Lock()
		a.runCtx = nil
		a.runCancel = nil
		a.state.IsStreaming = false
		a.mu.Unlock()
	}()

	return nil
}

// Wait blocks until the current run completes.
func (a *Agent) Wait() {
	a.mu.RLock()
	ctx := a.runCtx
	a.mu.RUnlock()

	if ctx != nil {
		<-ctx.Done()
	}
}

// runPrompt runs a new prompt.
func (a *Agent) runPrompt(messages []AgentMessage, runCtx context.Context) error {
	config := a.createLoopConfig()
	ctx := a.createContextSnapshot()

	// Add messages to state
	a.mu.Lock()
	a.state.Messages = append(a.state.Messages, messages...)
	a.mu.Unlock()

	// Emit message events
	for _, msg := range messages {
		a.emitAndProcess(AgentEvent{Type: EventMessageStart, Message: msg})
		a.emitAndProcess(AgentEvent{Type: EventMessageEnd, Message: msg})
	}

	// Run the loop
	_, err := RunAgentLoop(messages, ctx, *config, a.emitAndProcess, runCtx, a.streamFn)
	return err
}

// runContinuation continues from the current context.
func (a *Agent) runContinuation(runCtx context.Context) error {
	config := a.createLoopConfig()
	ctx := a.createContextSnapshot()

	// Run the continue loop
	_, err := RunAgentLoopContinue(ctx, *config, a.emitAndProcess, runCtx, a.streamFn)
	return err
}

// SimpleAgent is a simpler synchronous version of the Agent.
// It provides a straightforward API for single prompts.
type SimpleAgent struct {
	mu    sync.Mutex
	agent *Agent
}

// SimpleAgentConfig configures a SimpleAgent.
type SimpleAgentConfig struct {
	Model        *ai.Model
	SystemPrompt string
	Tools        []AgentTool
	StreamFn     StreamFunction
	GetAPIKey    func(provider string) (string, error)
}

// NewSimpleAgent creates a new SimpleAgent.
func NewSimpleAgent(config *SimpleAgentConfig) *SimpleAgent {
	agentConfig := &AgentOptions{
		InitialState: &AgentState{
			Model:        config.Model,
			SystemPrompt: config.SystemPrompt,
			Tools:        config.Tools,
		},
		StreamFn:  config.StreamFn,
		GetAPIKey: config.GetAPIKey,
	}

	return &SimpleAgent{
		agent: New(agentConfig),
	}
}

// Prompt sends a prompt and waits for the response.
// Returns the final messages or an error.
func (sa *SimpleAgent) Prompt(ctx context.Context, prompt string) ([]AgentMessage, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	// Create a new conversation
	msg := NewUserMessage(prompt)
	err := sa.agent.PromptMessage(msg)
	if err != nil {
		return nil, err
	}

	// Wait for completion
	sa.agent.Wait()

	// Return messages
	return sa.agent.Messages(), nil
}

// PromptWithFunc sends a prompt with a custom streaming function.
func (sa *SimpleAgent) PromptWithFunc(
	ctx context.Context,
	prompt string,
	onEvent func(AgentEvent),
) ([]AgentMessage, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if onEvent != nil {
		sa.agent.Subscribe(onEvent)
	}

	msg := NewUserMessage(prompt)
	err := sa.agent.PromptMessage(msg)
	if err != nil {
		return nil, err
	}

	// Wait for completion
	sa.agent.Wait()

	return sa.agent.Messages(), nil
}

// Continue continues the conversation with a new prompt.
func (sa *SimpleAgent) Continue(ctx context.Context) ([]AgentMessage, error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	err := sa.agent.Continue()
	if err != nil {
		return nil, err
	}

	// Wait for completion
	sa.agent.Wait()

	return sa.agent.Messages(), nil
}

// Messages returns all messages in the conversation.
func (sa *SimpleAgent) Messages() []AgentMessage {
	return sa.agent.Messages()
}

// State returns the current agent state.
func (sa *SimpleAgent) State() AgentState {
	return sa.agent.State()
}

// Abort aborts the current run.
func (sa *SimpleAgent) Abort() {
	sa.agent.Abort()
}

// Reset clears the conversation.
func (sa *SimpleAgent) Reset() {
	sa.agent.Reset()
}

// Subscribe subscribes to agent events.
func (sa *SimpleAgent) Subscribe(listener EventSink) func() {
	return sa.agent.Subscribe(listener)
}
