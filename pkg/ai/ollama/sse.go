package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"mnemos/pkg/ai"
)

// --- SSE response types ---

// Ollama streams OpenAI-compatible SSE responses:
//
//	data: {"id":"...","choices":[{"delta":{"content":"..."}}],...}
//	data: [DONE]

type sseChunk struct {
	ID      string      `json:"id"`
	Choices []sseChoice `json:"choices"`
	Usage   *sseUsage   `json:"usage,omitempty"`
}

type sseChoice struct {
	Index        int      `json:"index"`
	Delta        sseDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason"`
}

type sseDelta struct {
	Content   *string       `json:"content"`
	ToolCalls []sseToolCall `json:"tool_calls,omitempty"`
}

type sseToolCall struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function *sseToolCallFn `json:"function,omitempty"`
}

type sseToolCallFn struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type sseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// activeToolCall tracks a tool call being streamed.
type activeToolCall struct {
	id         string
	name       string
	argsBuffer strings.Builder
	index      int // content index in output.Content
}

// --- SSE parsing ---

func (p *Provider) parseSSE(
	ctx context.Context,
	stream *ai.EventStream,
	body io.Reader,
	model *ai.Model,
	output *ai.AssistantMessage,
) {
	scanner := bufio.NewScanner(body)
	// Increase buffer for potentially large SSE lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentText *ai.TextContent
	var currentTextIdx int
	activeTools := make(map[int]*activeToolCall) // keyed by SSE tool_call index

	for scanner.Scan() {
		if ctx.Err() != nil {
			output.StopReason = ai.StopReasonAborted
			output.ErrorMessage = "request was aborted"
			stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonAborted, Error: *output})
			return
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if output.ResponseID == "" && chunk.ID != "" {
			output.ResponseID = chunk.ID
		}

		if chunk.Usage != nil {
			output.Usage = ai.Usage{
				Input:       chunk.Usage.PromptTokens,
				Output:      chunk.Usage.CompletionTokens,
				TotalTokens: chunk.Usage.TotalTokens,
			}
			model.CalculateCost(&output.Usage)
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		// Handle finish reason
		if choice.FinishReason != nil {
			output.StopReason = mapFinishReason(*choice.FinishReason)

			// Finish any open text block
			if currentText != nil {
				stream.Push(ai.Event{
					Type:         ai.EventTextEnd,
					ContentIndex: currentTextIdx,
					Content:      currentText.Text,
					Partial:      *output,
				})
				currentText = nil
			}
			// Finish any open tool calls
			for sseIdx, tc := range activeTools {
				finishToolCall(stream, output, tc)
				delete(activeTools, sseIdx)
			}
		}

		// Handle text content delta
		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			delta := *choice.Delta.Content

			if currentText == nil {
				// Start a new text block
				currentText = &ai.TextContent{Text: ""}
				output.Content = append(output.Content, *currentText)
				currentTextIdx = len(output.Content) - 1
				stream.Push(ai.Event{
					Type:         ai.EventTextStart,
					ContentIndex: currentTextIdx,
					Partial:      *output,
				})
			}

			currentText.Text += delta
			// Update in output.Content
			output.Content[currentTextIdx] = *currentText
			stream.Push(ai.Event{
				Type:         ai.EventTextDelta,
				ContentIndex: currentTextIdx,
				Delta:        delta,
				Partial:      *output,
			})
		}

		// Handle tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			existing, ok := activeTools[tc.Index]
			if !ok {
				// Finish text block if one was open
				if currentText != nil {
					stream.Push(ai.Event{
						Type:         ai.EventTextEnd,
						ContentIndex: currentTextIdx,
						Content:      currentText.Text,
						Partial:      *output,
					})
					currentText = nil
				}

				// New tool call
				id := tc.ID
				name := ""
				if tc.Function != nil {
					name = tc.Function.Name
				}
				contentIdx := len(output.Content)
				toolCallContent := ai.ToolCall{ID: id, Name: name, Arguments: map[string]any{}}
				output.Content = append(output.Content, toolCallContent)

				existing = &activeToolCall{id: id, name: name, index: contentIdx}
				activeTools[tc.Index] = existing

				stream.Push(ai.Event{
					Type:         ai.EventToolCallStart,
					ContentIndex: contentIdx,
					Partial:      *output,
				})
			}

			if tc.Function != nil {
				if tc.Function.Name != "" {
					existing.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					existing.argsBuffer.WriteString(tc.Function.Arguments)
					stream.Push(ai.Event{
						Type:         ai.EventToolCallDelta,
						ContentIndex: existing.index,
						Delta:        tc.Function.Arguments,
						Partial:      *output,
					})
				}
			}
			if tc.ID != "" {
				existing.id = tc.ID
			}
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		output.StopReason = ai.StopReasonError
		output.ErrorMessage = fmt.Sprintf("ollama: read stream: %v", err)
		stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonError, Error: *output})
		return
	}

	// Finish any remaining open blocks
	if currentText != nil {
		stream.Push(ai.Event{
			Type:         ai.EventTextEnd,
			ContentIndex: currentTextIdx,
			Content:      currentText.Text,
			Partial:      *output,
		})
	}
	for sseIdx, tc := range activeTools {
		finishToolCall(stream, output, tc)
		delete(activeTools, sseIdx)
	}

	// Emit terminal event
	if output.StopReason == ai.StopReasonError || output.StopReason == ai.StopReasonAborted {
		stream.Push(ai.Event{Type: ai.EventError, Reason: output.StopReason, Error: *output})
	} else {
		stream.Push(ai.Event{Type: ai.EventDone, Reason: output.StopReason, Message: *output})
	}
}

func finishToolCall(stream *ai.EventStream, output *ai.AssistantMessage, tc *activeToolCall) {
	args := parsePartialJSON(tc.argsBuffer.String())
	toolCall := ai.ToolCall{
		ID:        tc.id,
		Name:      tc.name,
		Arguments: args,
	}
	// Update the content block in output
	if tc.index < len(output.Content) {
		output.Content[tc.index] = toolCall
	}
	stream.Push(ai.Event{
		Type:         ai.EventToolCallEnd,
		ContentIndex: tc.index,
		ToolCall:     &toolCall,
		Partial:      *output,
	})
}

func mapFinishReason(reason string) ai.StopReason {
	switch reason {
	case "stop", "end":
		return ai.StopReasonStop
	case "length":
		return ai.StopReasonLength
	case "tool_calls", "function_call":
		return ai.StopReasonToolUse
	default:
		return ai.StopReasonError
	}
}

// parsePartialJSON attempts to parse JSON; returns empty map on failure.
func parsePartialJSON(s string) map[string]any {
	s = strings.TrimSpace(s)
	if s == "" {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		return map[string]any{}
	}
	return result
}
