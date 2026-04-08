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

// --- Native JSON response types ---

// Ollama streams native JSON responses (newline-delimited):
//  {"model":"...","message":{"role":"assistant","content":"..."},"done":false}
//  {"model":"...","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop",...}

type nativeChunk struct {
	Model           string        `json:"model"`
	Message         nativeMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

type nativeMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Thinking  string         `json:"thinking,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

// chatToolCall is already defined in convert.go, but we need it here for parsing
// Unmarshaling into it should work if it's identical or we can just redefine it.
// Actually since they are in the same package, we can use the one from convert.go.
// Wait, convert.go defined it as chatToolCall. Let's use it.

// --- Stream parsing ---

func (p *Provider) parseSSE(
	ctx context.Context,
	stream *ai.EventStream,
	body io.Reader,
	model *ai.Model,
	output *ai.AssistantMessage,
) {
	scanner := bufio.NewScanner(body)
	// Increase buffer for potentially large lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentText *ai.TextContent
	var currentTextIdx int
	var currentThinking *ai.ThinkingContent
	var currentThinkingIdx int

	for scanner.Scan() {
		if ctx.Err() != nil {
			output.StopReason = ai.StopReasonAborted
			output.ErrorMessage = "request was aborted"
			stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonAborted, Error: *output})
			return
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var chunk nativeChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		// Update usage if provided (usually in the last chunk)
		if chunk.PromptEvalCount > 0 || chunk.EvalCount > 0 {
			output.Usage = ai.Usage{
				Input:       chunk.PromptEvalCount,
				Output:      chunk.EvalCount,
				TotalTokens: chunk.PromptEvalCount + chunk.EvalCount,
			}
			model.CalculateCost(&output.Usage)
		}

		// Handle thinking content delta
		if chunk.Message.Thinking != "" {
			delta := chunk.Message.Thinking

			if currentThinking == nil {
				// Start a new thinking block
				currentThinking = &ai.ThinkingContent{Thinking: ""}
				output.Content = append(output.Content, *currentThinking)
				currentThinkingIdx = len(output.Content) - 1
				stream.Push(ai.Event{
					Type:         ai.EventThinkingStart,
					ContentIndex: currentThinkingIdx,
					Partial:      *output,
				})
			}

			currentThinking.Thinking += delta
			// Update in output.Content
			output.Content[currentThinkingIdx] = *currentThinking
			stream.Push(ai.Event{
				Type:         ai.EventThinkingDelta,
				ContentIndex: currentThinkingIdx,
				Delta:        delta,
				Partial:      *output,
			})
		}

		// Handle text content delta
		if chunk.Message.Content != "" {
			delta := chunk.Message.Content

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

		// Handle tool calls (Ollama doesn't stream tool arguments in /api/chat natively, it sends the full objects at the end)
		for _, tc := range chunk.Message.ToolCalls {
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

			id := tc.ID
			name := tc.Function.Name
			
			// parse tool arguments
			argsMap := tc.Function.Arguments

			contentIdx := len(output.Content)
			toolCallContent := ai.ToolCall{ID: id, Name: name, Arguments: argsMap}
			output.Content = append(output.Content, toolCallContent)

			stream.Push(ai.Event{
				Type:         ai.EventToolCallStart,
				ContentIndex: contentIdx,
				Partial:      *output,
			})
			
			// Since args are not streamed, we simulate sending delta and then end
			argsJSON, _ := json.Marshal(argsMap)
			stream.Push(ai.Event{
				Type:         ai.EventToolCallDelta,
				ContentIndex: contentIdx,
				Delta:        string(argsJSON),
				Partial:      *output,
			})

			stream.Push(ai.Event{
				Type:         ai.EventToolCallEnd,
				ContentIndex: contentIdx,
				ToolCall:     &toolCallContent,
				Partial:      *output,
			})
		}

		if chunk.Done {
			output.StopReason = mapFinishReason(chunk.DoneReason)
			break
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		output.StopReason = ai.StopReasonError
		output.ErrorMessage = fmt.Sprintf("ollama: read stream: %v", err)
		stream.Push(ai.Event{Type: ai.EventError, Reason: ai.StopReasonError, Error: *output})
		return
	}

	// Finish any remaining open blocks
	if currentThinking != nil {
		stream.Push(ai.Event{
			Type:         ai.EventThinkingEnd,
			ContentIndex: currentThinkingIdx,
			Content:      currentThinking.Thinking,
			Partial:      *output,
		})
	}
	if currentText != nil {
		stream.Push(ai.Event{
			Type:         ai.EventTextEnd,
			ContentIndex: currentTextIdx,
			Content:      currentText.Text,
			Partial:      *output,
		})
	}

	// Emit terminal event
	if output.StopReason == ai.StopReasonError || output.StopReason == ai.StopReasonAborted {
		stream.Push(ai.Event{Type: ai.EventError, Reason: output.StopReason, Error: *output})
	} else {
		stream.Push(ai.Event{Type: ai.EventDone, Reason: output.StopReason, Message: *output})
	}
}

func mapFinishReason(reason string) ai.StopReason {
	switch reason {
	case "stop", "end", "":
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
