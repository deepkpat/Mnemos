package agent

import (
	"context"
	"errors"
	"time"

	"mnemos/pkg/ai"
)

// StreamFunction is the function signature for the LLM streaming implementation.
// It wraps ai.Stream to provide the agent loop with streaming capabilities.
type StreamFunction func(ctx context.Context, model *ai.Model, conv *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error)

// DefaultStreamFunction is the default streaming function using ai.Stream.
func DefaultStreamFunction(ctx context.Context, model *ai.Model, conv *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
	return ai.Stream(ctx, model, conv, opts)
}

// RunAgentLoop starts an agent loop with new prompt messages.
// The prompts are added to the context and events are emitted for them.
func RunAgentLoop(
	prompts []AgentMessage,
	context_ AgentContext,
	config AgentLoopConfig,
	emit EventSink,
	signal <-chan struct{},
	streamFn StreamFunction,
) ([]AgentMessage, error) {
	if streamFn == nil {
		streamFn = DefaultStreamFunction
	}

	newMessages := make([]AgentMessage, len(prompts))
	copy(newMessages, prompts)

	currentContext := AgentContext{
		SystemPrompt: context_.SystemPrompt,
		Messages:     append(context_.Messages, prompts...),
		Tools:        context_.Tools,
	}

	emit(AgentEvent{Type: EventAgentStart})
	emit(AgentEvent{Type: EventTurnStart})

	for _, msg := range prompts {
		emit(AgentEvent{Type: EventMessageStart, Message: msg})
		emit(AgentEvent{Type: EventMessageEnd, Message: msg})
	}

	err := runLoop(currentContext, newMessages, config, emit, signal, streamFn)
	if err != nil {
		return newMessages, err
	}

	return newMessages, nil
}

// RunAgentLoopContinue continues an agent loop from the current context.
// Used for retries - context already has user message or tool results.
// Returns ErrNoMessages if context has no messages.
// Returns ErrCannotContinueFromAssistant if the last message is an assistant message.
func RunAgentLoopContinue(
	context_ AgentContext,
	config AgentLoopConfig,
	emit EventSink,
	signal <-chan struct{},
	streamFn StreamFunction,
) ([]AgentMessage, error) {
	if streamFn == nil {
		streamFn = DefaultStreamFunction
	}

	if len(context_.Messages) == 0 {
		return nil, errors.New("agent: cannot continue: no messages in context")
	}

	// Last message must be user or toolResult, not assistant
	lastMsg := context_.Messages[len(context_.Messages)-1]
	if _, ok := lastMsg.(*AssistantAgentMessage); ok {
		return nil, errors.New("agent: cannot continue from message role: assistant")
	}

	newMessages := []AgentMessage{}

	emit(AgentEvent{Type: EventAgentStart})
	emit(AgentEvent{Type: EventTurnStart})

	err := runLoop(context_, newMessages, config, emit, signal, streamFn)
	if err != nil {
		return newMessages, err
	}

	return newMessages, nil
}

// runLoop is the main loop logic shared by RunAgentLoop and RunAgentLoopContinue.
func runLoop(
	currentContext AgentContext,
	newMessages []AgentMessage,
	config AgentLoopConfig,
	emit EventSink,
	signal <-chan struct{},
	streamFn StreamFunction,
) error {
	firstTurn := true

	// Check for steering messages at start
	var pendingMessages []AgentMessage
	if config.GetSteeringMessages != nil {
		msgs, _ := config.GetSteeringMessages()
		pendingMessages = msgs
	}

	// Outer loop: continues when queued follow-up messages arrive after agent would stop
	for {
		hasMoreToolCalls := true

		// Inner loop: process tool calls and steering messages
		for hasMoreToolCalls || len(pendingMessages) > 0 {
			if !firstTurn {
				emit(AgentEvent{Type: EventTurnStart})
			} else {
				firstTurn = false
			}

			// Process pending steering messages
			if len(pendingMessages) > 0 {
				for _, msg := range pendingMessages {
					emit(AgentEvent{Type: EventMessageStart, Message: msg})
					emit(AgentEvent{Type: EventMessageEnd, Message: msg})
					currentContext.Messages = append(currentContext.Messages, msg)
					newMessages = append(newMessages, msg)
				}
				pendingMessages = nil
			}

			// Stream assistant response
			message, err := streamAssistantResponse(currentContext, config, signal, emit, streamFn)
			if err != nil {
				return err
			}

			assistantMsg := NewAssistantMessage(message)
			newMessages = append(newMessages, assistantMsg)

			if message.StopReason == ai.StopReasonError || message.StopReason == ai.StopReasonAborted {
				emit(AgentEvent{Type: EventTurnEnd, Message_: assistantMsg, ToolResults: nil})
				emit(AgentEvent{Type: EventAgentEnd, Messages: newMessages})
				return nil
			}

			// Check for tool calls
			var toolCalls []*ai.ToolCall
			for _, c := range message.Content {
				if tc, ok := c.(*ai.ToolCall); ok {
					toolCalls = append(toolCalls, tc)
				}
			}
			hasMoreToolCalls = len(toolCalls) > 0

			var toolResults []ToolResultAgentMessage
			if hasMoreToolCalls {
				var err error
				toolResults, err = executeToolCalls(currentContext, assistantMsg, toolCalls, config, signal, emit)
				if err != nil {
					return err
				}

				for _, result := range toolResults {
					currentContext.Messages = append(currentContext.Messages, &result)
					newMessages = append(newMessages, &result)
				}
			}

			emit(AgentEvent{Type: EventTurnEnd, Message_: assistantMsg, ToolResults: toolResults})

			// Check for steering messages after turn
			if config.GetSteeringMessages != nil {
				msgs, _ := config.GetSteeringMessages()
				pendingMessages = msgs
			}
		}

		// Agent would stop here. Check for follow-up messages.
		if config.GetFollowUpMessages != nil {
			msgs, _ := config.GetFollowUpMessages()
			if len(msgs) > 0 {
				pendingMessages = msgs
				continue
			}
		}

		// No more messages, exit
		break
	}

	emit(AgentEvent{Type: EventAgentEnd, Messages: newMessages})
	return nil
}

// streamAssistantResponse streams an assistant response from the LLM.
func streamAssistantResponse(
	context_ AgentContext,
	config AgentLoopConfig,
	signal <-chan struct{},
	emit EventSink,
	streamFn StreamFunction,
) (ai.AssistantMessage, error) {
	// Apply context transform if configured
	messages := context_.Messages
	if config.TransformContext != nil {
		var err error
		messages, err = config.TransformContext(context_.Messages)
		if err != nil {
			return ai.AssistantMessage{}, err
		}
	}

	// Convert to LLM-compatible messages
	llmMessages := make([]ai.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = ToLLMMessage(msg)
	}

	// Build LLM context
	llmContext := &ai.Context{
		SystemPrompt: context_.SystemPrompt,
		Messages:     llmMessages,
		Tools:        nil, // Tools are passed separately to stream
	}

	// Convert agent tools to ai tools
	var llmTools []ai.Tool
	if len(context_.Tools) > 0 {
		llmTools = make([]ai.Tool, len(context_.Tools))
		for i, tool := range context_.Tools {
			llmTools[i] = ai.Tool{
				Name:        tool.ToolName(),
				Description: tool.ToolDescription(),
				Parameters:  tool.ToolParameters(),
			}
		}
	}
	if len(llmTools) > 0 {
		llmContext.Tools = llmTools
	}

	// Build stream options
	opts := &ai.StreamOptions{}
	if config.Reasoning != "" && config.Reasoning != ThinkingOff {
		opts.Reasoning = ai.ThinkingLevel(config.Reasoning)
	}
	if config.GetAPIKey != nil {
		key, _ := config.GetAPIKey(config.Model.Provider)
		opts.APIKey = key
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle abort signal
	if signal != nil {
		go func() {
			select {
			case <-signal:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	// Stream response
	response, err := streamFn(ctx, config.Model, llmContext, opts)
	if err != nil {
		return ai.AssistantMessage{}, err
	}

	// Process events
	var partialMessage *ai.AssistantMessage
	addedPartial := false

	for event := range response.Events() {
		switch event.Type {
		case ai.EventStart:
			partialMessage = &event.Partial
			context_.Messages = append(context_.Messages, NewAssistantMessage(event.Partial))
			addedPartial = true
			emit(AgentEvent{Type: EventMessageStart, Message: NewAssistantMessage(event.Partial)})

		case ai.EventTextStart, ai.EventTextDelta, ai.EventTextEnd,
			ai.EventThinkingStart, ai.EventThinkingDelta, ai.EventThinkingEnd,
			ai.EventToolCallStart, ai.EventToolCallDelta, ai.EventToolCallEnd:
			if partialMessage != nil {
				partialMessage = &event.Partial
				if len(context_.Messages) > 0 {
					context_.Messages[len(context_.Messages)-1] = NewAssistantMessage(event.Partial)
				}
				emit(AgentEvent{
					Type:                  EventMessageUpdate,
					Message:               NewAssistantMessage(event.Partial),
					AssistantMessageEvent: &event,
				})
			}

		case ai.EventDone, ai.EventError:
			finalMessage := response.Result()
			if addedPartial {
				context_.Messages[len(context_.Messages)-1] = NewAssistantMessage(finalMessage)
			} else {
				context_.Messages = append(context_.Messages, NewAssistantMessage(finalMessage))
			}
			if !addedPartial {
				emit(AgentEvent{Type: EventMessageStart, Message: NewAssistantMessage(finalMessage)})
			}
			emit(AgentEvent{Type: EventMessageEnd, Message: NewAssistantMessage(finalMessage)})
			return finalMessage, nil
		}
	}

	// Fallback: get result
	finalMessage := response.Result()
	if addedPartial {
		if len(context_.Messages) > 0 {
			context_.Messages[len(context_.Messages)-1] = NewAssistantMessage(finalMessage)
		}
	} else {
		context_.Messages = append(context_.Messages, NewAssistantMessage(finalMessage))
		emit(AgentEvent{Type: EventMessageStart, Message: NewAssistantMessage(finalMessage)})
	}
	emit(AgentEvent{Type: EventMessageEnd, Message: NewAssistantMessage(finalMessage)})
	return finalMessage, nil
}

// executeToolCalls executes tool calls from an assistant message.
func executeToolCalls(
	context_ AgentContext,
	assistantMsg *AssistantAgentMessage,
	toolCalls []*ai.ToolCall,
	config AgentLoopConfig,
	signal <-chan struct{},
	emit EventSink,
) ([]ToolResultAgentMessage, error) {
	if config.ToolExecution == ToolExecutionSequential {
		return executeToolCallsSequential(context_, assistantMsg, toolCalls, config, signal, emit)
	}
	return executeToolCallsParallel(context_, assistantMsg, toolCalls, config, signal, emit)
}

func executeToolCallsSequential(
	context_ AgentContext,
	assistantMsg *AssistantAgentMessage,
	toolCalls []*ai.ToolCall,
	config AgentLoopConfig,
	signal <-chan struct{},
	emit EventSink,
) ([]ToolResultAgentMessage, error) {
	results := make([]ToolResultAgentMessage, 0, len(toolCalls))

	for _, tc := range toolCalls {
		emit(AgentEvent{
			Type:       EventToolExecutionStart,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Args:       tc.Arguments,
		})

		result, isError := executeSingleToolCall(context_, assistantMsg, tc, config, signal, emit)
		results = append(results, result)

		emit(AgentEvent{
			Type:       EventToolExecutionEnd,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Result:     result.Content,
			IsError:    isError,
		})
	}

	return results, nil
}

func executeToolCallsParallel(
	context_ AgentContext,
	assistantMsg *AssistantAgentMessage,
	toolCalls []*ai.ToolCall,
	config AgentLoopConfig,
	signal <-chan struct{},
	emit EventSink,
) ([]ToolResultAgentMessage, error) {
	// Prepare all tool calls first
	type preparedCall struct {
		toolCall *ai.ToolCall
		tool     AgentTool
		args     map[string]any
	}

	prepared := make([]preparedCall, 0, len(toolCalls))
	results := make([]ToolResultAgentMessage, 0, len(toolCalls))

	for _, tc := range toolCalls {
		emit(AgentEvent{
			Type:       EventToolExecutionStart,
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Args:       tc.Arguments,
		})

		// Find tool
		var tool AgentTool
		for _, t := range context_.Tools {
			if t.ToolName() == tc.Name {
				tool = t
				break
			}
		}

		if tool == nil {
			// Tool not found - create error result immediately
			result := newToolErrorResult(tc.ID, tc.Name, "tool not found")
			results = append(results, result)
			emit(AgentEvent{
				Type:       EventToolExecutionEnd,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Result:     result.Content,
				IsError:    true,
			})
			continue
		}

		prepared = append(prepared, preparedCall{
			toolCall: tc,
			tool:     tool,
			args:     tc.Arguments,
		})
	}

	// Execute in parallel
	type resultPair struct {
		index int
		msg   ToolResultAgentMessage
	}
	resultChan := make(chan resultPair, len(prepared))

	for i, p := range prepared {
		go func(idx int, pc preparedCall) {
			result, _ := executeToolDirect(context_, assistantMsg, pc.toolCall, pc.tool, pc.args, config, signal, emit)
			resultChan <- resultPair{index: idx, msg: result}
		}(i, p)
	}

	// Collect results in order
	parallelResults := make([]ToolResultAgentMessage, len(prepared))
	for range prepared {
		r := <-resultChan
		parallelResults[r.index] = r.msg

		emit(AgentEvent{
			Type:       EventToolExecutionEnd,
			ToolCallID: r.msg.ToolCallID,
			ToolName:   r.msg.ToolName,
			Result:     r.msg.Content,
			IsError:    r.msg.IsError,
		})
	}

	results = append(results, parallelResults...)
	return results, nil
}

func executeSingleToolCall(
	context_ AgentContext,
	assistantMsg *AssistantAgentMessage,
	toolCall *ai.ToolCall,
	config AgentLoopConfig,
	signal <-chan struct{},
	emit EventSink,
) (ToolResultAgentMessage, bool) {
	// Find tool
	var tool AgentTool
	for _, t := range context_.Tools {
		if t.ToolName() == toolCall.Name {
			tool = t
			break
		}
	}

	if tool == nil {
		return newToolErrorResult(toolCall.ID, toolCall.Name, "tool not found"), true
	}

	return executeToolDirect(context_, assistantMsg, toolCall, tool, toolCall.Arguments, config, signal, emit)
}

func executeToolDirect(
	context_ AgentContext,
	assistantMsg *AssistantAgentMessage,
	toolCall *ai.ToolCall,
	tool AgentTool,
	args map[string]any,
	config AgentLoopConfig,
	signal <-chan struct{},
	emit EventSink,
) (ToolResultAgentMessage, bool) {
	// Call beforeToolCall hook if configured
	if config.BeforeToolCall != nil {
		result, err := config.BeforeToolCall(BeforeToolCallContext{
			AssistantMessage: assistantMsg,
			ToolCall:         toolCall,
			Args:             args,
			Context:          &context_,
		})
		if err == nil && result != nil && result.Block {
			return newToolErrorResult(toolCall.ID, toolCall.Name, result.Reason), true
		}
	}

	// Execute tool
	var onUpdate func(*AgentToolResult[any])
	if config.BeforeToolCall != nil {
		onUpdate = func(partial *AgentToolResult[any]) {
			emit(AgentEvent{
				Type:          EventToolExecutionUpdate,
				ToolCallID:    toolCall.ID,
				ToolName:      toolCall.Name,
				Args:          args,
				PartialResult: partial,
			})
		}
	}

	execResult, err := tool.Execute(toolCall.ID, args, signal, onUpdate)
	isError := err != nil
	if err != nil {
		execResult = &AgentToolResult[any]{
			Content: []ai.Content{ai.TextContent{Text: err.Error()}},
			Details: nil,
		}
		isError = true
	}

	// Call afterToolCall hook if configured
	if config.AfterToolCall != nil {
		result, err := config.AfterToolCall(AfterToolCallContext{
			AssistantMessage: assistantMsg,
			ToolCall:         toolCall,
			Args:             args,
			Result:           execResult,
			IsError:          isError,
			Context:          &context_,
		})
		if err == nil && result != nil {
			if result.Content != nil {
				execResult.Content = *result.Content
			}
			if result.IsError != nil {
				isError = *result.IsError
			}
		}
	}

	return ToolResultAgentMessage{
		ToolCallID: toolCall.ID,
		ToolName:   toolCall.Name,
		Content:    execResult.Content,
		IsError:    isError,
		Timestamp:  time.Now(),
	}, isError
}

func newToolErrorResult(toolCallID, toolName, errMsg string) ToolResultAgentMessage {
	return ToolResultAgentMessage{
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Content:    []ai.Content{ai.TextContent{Text: errMsg}},
		IsError:    true,
		Timestamp:  time.Now(),
	}
}
