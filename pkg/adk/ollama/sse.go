package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"mnemos/pkg/adk"
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
	PromptEvalCount uint64        `json:"prompt_eval_count,omitempty"`
	EvalCount       uint64        `json:"eval_count,omitempty"`
}

type nativeMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Thinking  string         `json:"thinking,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

// --- Stream parsing ---

func (p *Provider) parseSSE(
	ctx context.Context,
	stream *adk.EventStream,
	body io.Reader,
	model *adk.Model,
	output *adk.AssistantMessage,
) {
	scanner := bufio.NewScanner(body)
	// Increase buffer for potentially large lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentText *adk.Text
	var currentTextIdx int
	var currentThinking *adk.Thinking
	var currentThinkingIdx int

	for scanner.Scan() {
		if ctx.Err() != nil {
			output.StopReason = adk.StopReasonAborted
			output.ErrorMessage = "request was aborted"
			stream.PushError(adk.StopReasonAborted, *output)
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
			output.Usage = adk.Usage{
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
				// Finish text block if one was open
				if currentText != nil {
					stream.Push(adk.TextEndEvent{
						ContentIndex: currentTextIdx,
						Content:      currentText.Text,
						Partial:      output,
					})
					currentText = nil
				}

				// Start a new thinking block
				currentThinking = &adk.Thinking{Thinking: ""}
				output.Content = append(output.Content, *currentThinking)
				currentThinkingIdx = len(output.Content) - 1
				stream.Push(adk.ThinkingStartEvent{
					ContentIndex: currentThinkingIdx,
					Partial:      output,
				})
			}

			currentThinking.Thinking += delta
			// Update in output.Content
			output.Content[currentThinkingIdx] = *currentThinking
			stream.Push(adk.ThinkingDeltaEvent{
				ContentIndex: currentThinkingIdx,
				Delta:        delta,
				Partial:      output,
			})
		}

		// Handle text content delta
		if chunk.Message.Content != "" {
			delta := chunk.Message.Content

			if currentText == nil {
				// Finish thinking block if one was open
				if currentThinking != nil {
					stream.Push(adk.ThinkingEndEvent{
						ContentIndex: currentThinkingIdx,
						Content:      currentThinking.Thinking,
						Partial:      output,
					})
					currentThinking = nil
				}

				// Start a new text block
				currentText = &adk.Text{Text: ""}
				output.Content = append(output.Content, *currentText)
				currentTextIdx = len(output.Content) - 1
				stream.Push(adk.TextStartEvent{
					ContentIndex: currentTextIdx,
					Partial:      output,
				})
			}

			currentText.Text += delta
			// Update in output.Content
			output.Content[currentTextIdx] = *currentText
			stream.Push(adk.TextDeltaEvent{
				ContentIndex: currentTextIdx,
				Delta:        delta,
				Partial:      output,
			})
		}

		// Handle tool calls (Ollama doesn't stream tool arguments in /api/chat natively, it sends the full objects at the end)
		for _, tc := range chunk.Message.ToolCalls {
			// Finish thinking block if one was open
			if currentThinking != nil {
				stream.Push(adk.ThinkingEndEvent{
					ContentIndex: currentThinkingIdx,
					Content:      currentThinking.Thinking,
					Partial:      output,
				})
				currentThinking = nil
			}

			// Finish text block if one was open
			if currentText != nil {
				stream.Push(adk.TextEndEvent{
					ContentIndex: currentTextIdx,
					Content:      currentText.Text,
					Partial:      output,
				})
				currentText = nil
			}

			id := tc.ID
			name := tc.Function.Name

			// parse tool arguments
			argsMap := tc.Function.Arguments

			contentIdx := len(output.Content)
			toolCallContent := adk.ToolCall{ID: id, Name: name, Arguments: argsMap}
			output.Content = append(output.Content, toolCallContent)

			stream.Push(adk.ToolCallStartEvent{
				ContentIndex: contentIdx,
				Partial:      output,
			})

			// Since args are not streamed, we simulate sending delta and then end
			argsJSON, _ := json.Marshal(argsMap)
			stream.Push(adk.ToolCallDeltaEvent{
				ContentIndex: contentIdx,
				Delta:        string(argsJSON),
				Partial:      output,
			})

			stream.Push(adk.ToolCallEndEvent{
				ContentIndex: contentIdx,
				ToolCall:     toolCallContent,
				Partial:      output,
			})
		}

		if chunk.Done {
			output.StopReason = mapFinishReason(chunk.DoneReason)
			break
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		output.StopReason = adk.StopReasonError
		output.ErrorMessage = fmt.Sprintf("ollama: read stream: %v", err)
		stream.PushError(adk.StopReasonError, *output)
		return
	}

	// Finish any remaining open blocks
	if currentThinking != nil {
		stream.Push(adk.ThinkingEndEvent{
			ContentIndex: currentThinkingIdx,
			Content:      currentThinking.Thinking,
			Partial:      output,
		})
	}
	if currentText != nil {
		stream.Push(adk.TextEndEvent{
			ContentIndex: currentTextIdx,
			Content:      currentText.Text,
			Partial:      output,
		})
	}

	// Emit terminal event
	if output.StopReason == adk.StopReasonError || output.StopReason == adk.StopReasonAborted {
		stream.PushError(output.StopReason, *output)
	} else {
		stream.PushDone(output.StopReason, *output)
	}
}

func mapFinishReason(reason string) adk.StopReason {
	switch reason {
	case "stop", "end", "":
		return adk.StopReasonStop
	case "length":
		return adk.StopReasonLength
	case "tool_calls", "function_call":
		return adk.StopReasonToolUse
	default:
		return adk.StopReasonError
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
