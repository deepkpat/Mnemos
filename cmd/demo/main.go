package main

import (
	"context"
	"fmt"
	"os"

	"mnemos/pkg/ai"
	"mnemos/pkg/ai/ollama"
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

	// Build a conversation context
	conv := &ai.Context{
		SystemPrompt: "you are an expert programmer who writes clean and intuitive code, that anyone can explain just by looking at it. you first formalize the problem and break it in small parts before you code. you do not use emojis.",
		Messages: []ai.Message{
			ai.NewUserMessage("implement hash set from scratch in python, you can only use list/array"),
		},
	}

	fmt.Printf("Model: %s (%s)\n\n", model.Name, model.ID)

	// --- Streaming example with thinking budget ---
	// ThinkingBudget hard-limits thinking tokens (prevents rampage)
	opts := &ai.StreamOptions{
		ThinkingBudget: 100, // Max 100 tokens for thinking
	}
	stream, err := ai.Stream(context.Background(), model, conv, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for event := range stream.Events() {
		switch event.Type {
		case ai.EventThinkingStart:
			fmt.Print("\n[Thinking]\n")
		case ai.EventThinkingDelta:
			fmt.Print(event.Delta)
		case ai.EventThinkingEnd:
			fmt.Print("\n[ThinkingEnd]\n")
		case ai.EventTextStart:
			fmt.Print("\n[Response]\n")
		case ai.EventTextDelta:
			fmt.Print(event.Delta)
		case ai.EventTextEnd:
			fmt.Print("\n[ResponseEnd]\n")
		case ai.EventDone:
			fmt.Printf("\n\n--- Done (stop: %s, tokens: %d in / %d out) ---\n",
				event.Message.StopReason,
				event.Message.Usage.Input,
				event.Message.Usage.Output,
			)
		case ai.EventError:
			fmt.Fprintf(os.Stderr, "\nerror: %s\n", event.Error.ErrorMessage)
			os.Exit(1)
		}
	}
}
