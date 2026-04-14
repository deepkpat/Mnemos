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

	"mnemos/pkg/adk"
)

func (p *Provider) doStream(
	ctx context.Context,
	stream *adk.EventStream,
	model *adk.Model,
	thread *adk.Thread,
	opts *adk.StreamOptions,
) {
	output := adk.AssistantMessage{
		Content:    nil,
		API:        model.API,
		Provider:   model.Provider,
		Model:      model.ID,
		Usage:      adk.ZeroUsage(),
		StopReason: adk.StopReasonStop,
		Timestamp:  time.Now(),
	}

	req, err := p.buildRequest(ctx, model, thread, opts)
	if err != nil {
		output.StopReason = adk.StopReasonError
		output.ErrorMessage = err.Error()
		stream.PushError(adk.StopReasonError, output)
		return
	}

	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			output.StopReason = adk.StopReasonAborted
			output.ErrorMessage = "request was aborted"
			stream.PushError(adk.StopReasonAborted, output)
			return
		}
		output.StopReason = adk.StopReasonError
		output.ErrorMessage = err.Error()
		stream.PushError(adk.StopReasonError, output)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		output.StopReason = adk.StopReasonError
		output.ErrorMessage = fmt.Sprintf("ollama: HTTP %d: %s", resp.StatusCode, string(body))
		stream.PushError(adk.StopReasonError, output)
		return
	}

	stream.Push(adk.StartEvent{Partial: &output})

	p.parseSSE(ctx, stream, resp.Body, model, &output)
}

func (p *Provider) buildRequest(
	ctx context.Context,
	model *adk.Model,
	thread *adk.Thread,
	opts *adk.StreamOptions,
) (*http.Request, error) {
	payload := buildPayload(model, thread, opts)

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

	return req, nil
}
