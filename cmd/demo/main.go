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
	// For thinking/reasoning, use: ollama pull deepseek-r1:8b
	// For basic chat, use: ollama pull llama3.2
	model := ollama.NewModel("deepseek-r1:8b",
		ollama.WithReasoning(), // Enable reasoning/thinking mode
		ollama.WithContextWindow(128_000),
		ollama.WithMaxTokens(4096),
	)

	// Build a conversation context
	// Use a problem that benefits from reasoning
	conv := &ai.Context{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []ai.Message{
			ai.NewUserMessage("If a train leaves Paris at 2pm traveling at 120 km/h, and another leaves Lyon (465km away) at 2:30pm traveling at 100 km/h toward Paris, when do they meet?"),
		},
	}

	fmt.Printf("Model: %s (%s)\n\n", model.Name, model.ID)

	// --- Streaming example ---
	stream, err := ai.Stream(context.Background(), model, conv, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for event := range stream.Events() {
		switch event.Type {
		case ai.EventThinkingStart:
			fmt.Print("\n[Thinking] ")
		case ai.EventThinkingDelta:
			fmt.Print(event.Delta)
		case ai.EventThinkingEnd:
			fmt.Print("\n\n")
		case ai.EventTextStart:
			fmt.Print("[Response] ")
		case ai.EventTextDelta:
			fmt.Print(event.Delta)
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
