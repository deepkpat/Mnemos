package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"mnemos/pkg/ai"
	"mnemos/pkg/ai/ollama"
)

// ToolHandler handles tool execution
type ToolHandler func(name string, args map[string]any) (string, error)

// DefaultTools returns the available tools with their handlers
func DefaultTools() (map[string]ai.Tool, map[string]ToolHandler) {
	tools := map[string]ai.Tool{
		"bash": {
			Name:        "bash",
			Description: "Execute a bash command in the current working directory. Returns stdout and stderr.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Bash command to execute"},
				},
				"required": []string{"command"},
			},
		},
		"read": {
			Name:        "read",
			Description: "Read the contents of a file. Returns the file content or error.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path to the file to read"},
				},
				"required": []string{"path"},
			},
		},
		"ls": {
			Name:        "ls",
			Description: "List directory contents. Returns entries sorted alphabetically.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Directory to list (default: .)"},
				},
			},
		},
		"grep": {
			Name:        "grep",
			Description: "Search file contents for a pattern. Returns matching lines.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Search pattern"},
					"path":    map[string]any{"type": "string", "description": "Directory to search (default: .)"},
				},
				"required": []string{"pattern"},
			},
		},
		"find": {
			Name:        "find",
			Description: "Find files matching a pattern.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "File pattern to find"},
					"path":    map[string]any{"type": "string", "description": "Directory to search (default: .)"},
				},
				"required": []string{"pattern"},
			},
		},
	}

	// Tool handlers
	handlers := map[string]ToolHandler{
		"bash": bashTool,
		"read": readTool,
		"ls":   lsTool,
		"grep": grepTool,
		"find": findTool,
	}

	return tools, handlers
}

// Tool implementations
func bashTool(name string, args map[string]any) (string, error) {
	cmd, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("missing command argument")
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	// Run command using os/exec
	execCmd := exec.Command("sh", "-c", cmd)
	execCmd.Dir = cwd
	output, err := execCmd.CombinedOutput()

	if err != nil {
		// Command failed but still has output
		if len(output) > 0 {
			return string(output), nil
		}
		return "", fmt.Errorf("command failed: %v", err)
	}

	return string(output), nil
}

func readTool(name string, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing path argument")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}
	return string(data), nil
}

func lsTool(name string, args map[string]any) (string, error) {
	path := "."
	if p, ok := args["path"].(string); ok {
		path = p
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %v", err)
	}

	// Sort alphabetically
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name()+"/")
		} else {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	return strings.Join(names, "\n"), nil
}

func grepTool(name string, args map[string]any) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok {
		return "", fmt.Errorf("missing pattern argument")
	}
	path := "."
	if p, ok := args["path"].(string); ok {
		path = p
	}

	var matches []string
	err := findFiles(path, func(filePath string) error {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", filePath, i+1, line))
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No matches found", nil
	}
	return strings.Join(matches, "\n"), nil
}

func findTool(name string, args map[string]any) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok {
		return "", fmt.Errorf("missing pattern argument")
	}
	path := "."
	if p, ok := args["path"].(string); ok {
		path = p
	}

	var matches []string
	err := findFiles(path, func(filePath string) error {
		name := filePath
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		// Simple glob matching
		if strings.Contains(name, pattern) {
			matches = append(matches, filePath)
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No files found", nil
	}
	return strings.Join(matches, "\n"), nil
}

// Helper function to recursively find files
func findFiles(dir string, fn func(string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := dir + "/" + e.Name()
		if e.IsDir() {
			findFiles(path, fn)
		} else {
			fn(path)
		}
	}
	return nil
}

func main() {
	modelID := flag.String("model", "qwen3.5:0.8b", "Model ID to use")
	baseURL := flag.String("url", "http://localhost:11434", "Base URL")
	prompt := flag.String("p", "", "Prompt to send")
	interactive := flag.Bool("i", false, "Interactive mode")
	flag.Parse()

	// Register provider
	ollama.Register()

	// Create model
	model := &ai.Model{
		ID:            *modelID,
		Name:          *modelID,
		API:           "ollama",
		Provider:      "ollama",
		BaseURL:       *baseURL,
		MaxTokens:     4096,
		ContextWindow: 128000,
		Input:         []ai.InputModality{ai.InputText},
	}

	// Get working directory
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	// Get tools and handlers
	tools, handlers := DefaultTools()

	ctx := context.Background()

	// If interactive mode, run interactive loop
	if *interactive {
		runInteractive(ctx, model, cwd, tools, handlers)
		return
	}

	// Must have a prompt
	if *prompt == "" {
		fmt.Fprintf(os.Stderr, "Error: -p flag is required (or use -i for interactive mode)\n")
		flag.Usage()
		os.Exit(1)
	}

	// Send single prompt with tool loop
	sendPromptWithTools(ctx, model, cwd, *prompt, tools, handlers)
}

func sendPromptWithTools(ctx context.Context, model *ai.Model, cwd string, prompt string, tools map[string]ai.Tool, handlers map[string]ToolHandler) {
	// Build system prompt - be very explicit about tool usage
	systemPrompt := fmt.Sprintf(`You are a helpful coding assistant.

## Available Tools
You can call these tools when needed:
- bash: Execute shell commands
- read: Read file contents
- ls: List directory contents
- grep: Search file contents
- find: Find files

Working directory: %s

## Important Rules
1. After calling a tool and getting the result, provide your FINAL answer to the user
2. Do NOT call more tools unless you need additional information
3. If the tool result answers the question, STOP and respond to the user
4. Only call another tool if you need MORE information than you already have

## Response Format
When you need a tool, use: {"name": "tool_name", "arguments": {"arg": "value"}}
When you're done, just respond normally with your answer.`, cwd)

	userMsg := ai.NewUserMessage(prompt)

	conv := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage(systemPrompt),
			userMsg,
		},
		Tools: func() []ai.Tool {
			result := make([]ai.Tool, 0, len(tools))
			for _, t := range tools {
				result = append(result, t)
			}
			return result
		}(),
	}

	fmt.Printf("User: %s\n\n", prompt)

	// Only allow one tool call iteration - the model doesn't learn from results
	// This is a known issue with qwen3.5 - it keeps calling tools even after results
	maxIterations := 4

	for iter := 0; iter < maxIterations; iter++ {
		// Call LLM
		result, err := ai.Complete(ctx, model, conv, nil)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		// Process response
		hasToolCall := false
		for _, c := range result.Content {
			switch ct := c.(type) {
			case ai.TextContent:
				if ct.Text != "" {
					fmt.Printf("Assistant: %s\n", ct.Text)
				}
			case ai.ThinkingContent:
				if ct.Thinking != "" {
					fmt.Printf("[Thinking: %s]\n", truncate(ct.Thinking, 300))
				}
			case ai.ToolCall:
				hasToolCall = true
				fmt.Printf("\n→ Calling tool: %s\n", ct.Name)

				// Get arguments
				args := make(map[string]any)
				for k, v := range ct.Arguments {
					args[k] = v
				}

				// Execute tool
				handler, ok := handlers[ct.Name]
				if !ok {
					fmt.Printf("  Error: unknown tool: %s\n", ct.Name)
					continue
				}

				toolResult, err := handler(ct.Name, args)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
					conv.Messages = append(conv.Messages, &ai.ToolResultMessage{
						ToolCallID: ct.ID,
						ToolName:   ct.Name,
						Content:    []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
						IsError:    true,
						Timestamp:  time.Now(),
					})
				} else {
					displayResult := toolResult
					if len(displayResult) > 500 {
						displayResult = displayResult[:500] + "\n... (truncated)"
					}
					fmt.Printf("  Result: %s\n", displayResult)

					conv.Messages = append(conv.Messages, &ai.ToolResultMessage{
						ToolCallID: ct.ID,
						ToolName:   ct.Name,
						Content:    []ai.Content{ai.TextContent{Text: toolResult}},
						IsError:    false,
						Timestamp:  time.Now(),
					})
				}
			}
		}

		// If no tool calls, we're done
		if !hasToolCall {
			break
		}

		// If we only got thinking + tool calls, continue the loop
		// But after tool results, the model should give a final answer
		fmt.Println()
	}
}

func runInteractive(ctx context.Context, model *ai.Model, cwd string, tools map[string]ai.Tool, handlers map[string]ToolHandler) {
	systemPrompt := fmt.Sprintf(`You are a helpful coding assistant.

## Available Tools
- bash, read, ls, grep, find

## Rules
1. After getting tool results, provide your FINAL answer
2. Do NOT call more tools unless you need more information
3. If the answer is complete, STOP

Working directory: %s`, cwd)

	conv := &ai.Context{
		Messages: []ai.Message{
			ai.NewUserMessage(systemPrompt),
		},
		Tools: func() []ai.Tool {
			result := make([]ai.Tool, 0, len(tools))
			for _, t := range tools {
				result = append(result, t)
			}
			return result
		}(),
	}

	fmt.Println("=== Mnemos Interactive Mode (with Tools) ===")
	fmt.Println("Type your prompts. Commands: :quit to exit, :reset to reset")
	fmt.Println("================================================================================\n")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("You: ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "" {
			continue
		}

		if input == ":quit" || input == ":q" {
			fmt.Println("Goodbye!")
			break
		}

		if input == ":reset" {
			conv = &ai.Context{
				Messages: []ai.Message{
					ai.NewUserMessage(systemPrompt),
				},
				Tools: func() []ai.Tool {
					result := make([]ai.Tool, 0, len(tools))
					for _, t := range tools {
						result = append(result, t)
					}
					return result
				}(),
			}
			fmt.Println("Conversation reset.\n")
			continue
		}

		conv.Messages = append(conv.Messages, ai.NewUserMessage(input))

		// Same limit as single prompt - model over-calls tools
		maxIterations := 1
		for iter := 0; iter < maxIterations; iter++ {
			result, err := ai.Complete(ctx, model, conv, nil)
			if err != nil {
				fmt.Printf("Error: %v\n\n", err)
				break
			}

			hasToolCall := false
			for _, c := range result.Content {
				switch ct := c.(type) {
				case ai.TextContent:
					if ct.Text != "" {
						fmt.Printf("Assistant: %s\n", ct.Text)
					}
				case ai.ThinkingContent:
					if ct.Thinking != "" {
						fmt.Printf("[Thinking: %s]\n", truncate(ct.Thinking, 300))
					}
				case ai.ToolCall:
					hasToolCall = true
					fmt.Printf("\n→ Tool: %s\n", ct.Name)

					args := make(map[string]any)
					for k, v := range ct.Arguments {
						args[k] = v
					}

					handler, ok := handlers[ct.Name]
					if !ok {
						fmt.Printf("  Error: unknown tool\n")
						continue
					}

					toolResult, err := handler(ct.Name, args)
					if err != nil {
						fmt.Printf("  Error: %v\n", err)
						conv.Messages = append(conv.Messages, &ai.ToolResultMessage{
							ToolCallID: ct.ID,
							ToolName:   ct.Name,
							Content:    []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
							IsError:    true,
							Timestamp:  time.Now(),
						})
					} else {
						displayResult := toolResult
						if len(displayResult) > 300 {
							displayResult = displayResult[:300] + "..."
						}
						fmt.Printf("  Result: %s\n", displayResult)
						conv.Messages = append(conv.Messages, &ai.ToolResultMessage{
							ToolCallID: ct.ID,
							ToolName:   ct.Name,
							Content:    []ai.Content{ai.TextContent{Text: toolResult}},
							IsError:    false,
							Timestamp:  time.Now(),
						})
					}
				}
			}

			if !hasToolCall {
				break
			}
			fmt.Println()
		}
		fmt.Println()
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
