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
	// Just change the model ID string to switch models.
	model := ollama.NewModel("llama3.2",
		ollama.WithContextWindow(128_000),
		ollama.WithMaxTokens(4096),
	)

	// Build a conversation context
	conv := &ai.Context{
		SystemPrompt: "You are a helpful assistant. Be concise.",
		Messages: []ai.Message{
			ai.NewUserMessage("What is the capital of France? Answer in one sentence."),
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
