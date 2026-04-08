package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mnemos/pkg/ai"
)

func (p *Provider) doStream(
	ctx context.Context,
	stream *ai.EventStream,
	model *ai.Model,
	conv *ai.Context,
	opts *ai.StreamOptions,
) {
	output := ai.AssistantMessage{
		Content:    nil,
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage:      ai.ZeroUsage(),
		StopReason: ai.StopReasonStop,
		Timestamp:  time.Now(),
	}

	req, err := p.buildRequest(ctx, model, conv, opts)
	if err != nil {
		output.StopReason = ai.StopReasonError
		output.ErrorMessage = err.Error()
		stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonError, Error: output})
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			output.StopReason = ai.StopReasonAborted
			output.ErrorMessage = "request was aborted"
			stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonAborted, Error: output})
			return
		}
		output.StopReason = ai.StopReasonError
		output.ErrorMessage = err.Error()
		stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonError, Error: output})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		output.StopReason = ai.StopReasonError
		output.ErrorMessage = fmt.Sprintf("ollama: HTTP %d: %s", resp.StatusCode, string(body))
		stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonError, Error: output})
		return
	}

	stream.Push(ai.Event{Type: ai.EventStart, Partial: output})

	p.parseSSE(ctx, stream, resp.Body, model, &output)
}

func (p *Provider) buildRequest(
	ctx context.Context,
	model *ai.Model,
	conv *ai.Context,
	opts *ai.StreamOptions,
) (*http.Request, error) {
	payload := buildPayload(model, conv, opts)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal payload: %w", err)
	}

	url := strings.TrimRight(model.BaseURL, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if opts != nil && opts.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	}
	if opts != nil {
		for k, v := range opts.Headers {
			req.Header.Set(k, v)
		}
	}
	for k, v := range model.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}
