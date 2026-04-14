package main

import (
	"context"
	"fmt"
	"os"

	"mnemos/pkg/adk"
	"mnemos/pkg/adk/ollama"
)

func main() {
	// Register the Ollama provider globally
	ollama.Register()

	// Pick any model you've pulled locally via: ollama pull <model>
	// For Qwen: ollama pull qwen2.5:0.5b
	// For DeepSeek reasoning: ollama pull deepseek-r1:8b
	// For basic chat: ollama pull llama3.2
	model := ollama.NewModel("qwen3.5:0.8b",
		// Don't use WithReasoning() for small models - they go on thinking rampages
		ollama.WithReasoning(),
		ollama.WithContextWindow(32_000),
		ollama.WithMaxTokens(4096),
	)

	// Build a conversation thread
	thread := &adk.Thread{
		SystemPrompt: "you are an expert programmer who writes clean and intuitive code, that anyone can explain just by looking at it. you first formalize the problem and break it in small parts before you code. you do not use emojis.",
		Messages: []adk.Message{
			adk.NewUserMessage("implement hash set from scratch in python, you can only use list/array"),
		},
	}

	fmt.Printf("Model: %s (%s)\n\n", model.Name, model.ID)

	// --- Streaming example with thinking budget ---
	// ThinkingBudget hard-limits thinking tokens (prevents rampage)
	opts := &adk.StreamOptions{
		ThinkingBudget: 100, // Max 100 tokens for thinking
	}
	stream, err := adk.Stream(context.Background(), model, thread, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for event := range stream.Events() {
		switch event.EventType() {
		case adk.EventThinkingStart:
			fmt.Print("\n[Thinking]\n")
		case adk.EventThinkingDelta:
			// Use type assertion for typed events
			if delta, ok := event.(adk.ThinkingDeltaEvent); ok {
				fmt.Print(delta.Delta)
			}
		case adk.EventThinkingEnd:
			fmt.Print("\n[ThinkingEnd]\n")
		case adk.EventTextStart:
			fmt.Print("\n[Response]\n")
		case adk.EventTextDelta:
			if delta, ok := event.(adk.TextDeltaEvent); ok {
				fmt.Print(delta.Delta)
			}
		case adk.EventTextEnd:
			fmt.Print("\n[ResponseEnd]\n")
		case adk.EventDone:
			if done, ok := event.(adk.DoneEvent); ok {
				fmt.Printf("\n\n--- Done (stop: %s, tokens: %d in / %d out) ---\n",
					done.Reason,
					done.Message.Usage.Input,
					done.Message.Usage.Output,
				)
			}
		case adk.EventError:
			if err, ok := event.(adk.ErrorEvent); ok {
				fmt.Fprintf(os.Stderr, "\nerror: %s\n", err.Error.ErrorMessage)
				os.Exit(1)
			}
		}
	}
}
