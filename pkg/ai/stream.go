package ai

import (
	"context"
	"fmt"
)

// Stream starts a streaming LLM request using the provider identified by model.API.
// Returns an EventStream that emits events as the response is generated.
func Stream(ctx context.Context, model *Model, thread *Thread, opts *StreamOptions) (*EventStream, error) {
	p := GetProvider(model.API)
	if p == nil {
		return nil, fmt.Errorf("ai: no provider registered for api %q", model.API)
	}
	return p.Stream(ctx, model, thread, opts), nil
}

// Complete performs a non-streaming LLM request. It starts a stream, drains all
// events, and returns the final AssistantMessage.
func Complete(ctx context.Context, model *Model, thread *Thread, opts *StreamOptions) (AssistantMessage, error) {
	s, err := Stream(ctx, model, thread, opts)
	if err != nil {
		return AssistantMessage{}, err
	}
	result := s.Result()
	if result.StopReason == StopReasonError || result.StopReason == StopReasonAborted {
		return result, fmt.Errorf("ai: %s: %s", result.StopReason, result.ErrorMessage)
	}
	return result, nil
}
