package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"mnemos/pkg/agent"
	"mnemos/pkg/ai"
	"mnemos/pkg/ai/ollama"
	"mnemos/pkg/coding_agent"
)

var (
	modelName    = flag.String("model", "qwen2.5:0.8b", "Model to use")
	sessionDir   = flag.String("session", "./sessions", "Session directory")
	systemPrompt = flag.String("system", "", "System prompt")
	agentTools   []agent.AgentTool
)

func main() {
	flag.Parse()

	if *modelName == "" {
		fmt.Fprintf(os.Stderr, "Error: --model is required\n")
		os.Exit(1)
	}

	// Register Ollama provider
	ollama.Register()

	// Create model
	model := ollama.NewModel(*modelName,
		ollama.WithContextWindow(32_000),
		ollama.WithMaxTokens(4096),
	)

	// Set up system prompt if provided
	sysPrompt := *systemPrompt
	if sysPrompt == "" {
		sysPrompt = `You are an expert coding assistant. You have tools to read, write, edit files and run commands.
When you write code, make it clean and readable. Explain your thinking before coding.`
	}

	// Create session manager
	sm, err := coding_agent.NewSessionManager(*sessionDir, &coding_agent.SessionOptions{
		Cwd: getCwd(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating session: %v\n", err)
		os.Exit(1)
	}
	defer sm.Close()

	// Create agent tools
	agentTools = createTools()

	// Create agent
	ag := agent.New(&agent.AgentOptions{
		InitialState: &agent.AgentState{
			SystemPrompt:  sysPrompt,
			Model:         model,
			Tools:         agentTools,
			ThinkingLevel: agent.ThinkingOff,
		},
		StreamFn: func(ctx context.Context, model *ai.Model, conv *ai.Context, opts *ai.StreamOptions) (*ai.EventStream, error) {
			return ai.Stream(ctx, model, conv, opts)
		},
	})

	fmt.Printf("=== Mnemos ===\n")
	fmt.Printf("Model: %s\n", *modelName)
	fmt.Printf("Type /help for commands, Ctrl+C to exit\n\n")

	// Main interaction loop
	scanner := bufio.NewScanner(os.Stdin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nExiting...")
		cancel()
		os.Exit(0)
	}()

	// Subscribe to events for streaming display
	ag.Subscribe(func(e agent.AgentEvent) {
		switch e.Type {
		case agent.EventMessageStart:
			if msg, ok := e.Message.(*agent.AssistantAgentMessage); ok {
				fmt.Printf("\n[Assistant]\n")
				for _, c := range msg.Content {
					if tc, ok := c.(ai.TextContent); ok {
						fmt.Printf("%s", tc.Text)
					}
				}
			}
		case agent.EventMessageUpdate:
			if msg, ok := e.Message.(*agent.AssistantAgentMessage); ok {
				for _, c := range msg.Content {
					if tc, ok := c.(ai.TextContent); ok {
						fmt.Printf("%s", tc.Text)
					}
				}
			}
		case agent.EventMessageEnd:
			fmt.Println()
		case agent.EventToolExecutionStart:
			fmt.Printf("\n[Tool: %s] ", e.ToolName)
		case agent.EventToolExecutionEnd:
			fmt.Printf(" -> done\n")
		case agent.EventTurnStart:
			// Turn started
		case agent.EventTurnEnd:
			// Turn ended
		}
	})

	// Process prompts
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle commands
		if input == "/help" {
			printHelp()
			continue
		}
		if input == "/exit" || input == "/quit" {
			break
		}
		if input == "/session" {
			fmt.Printf("Session: %s\n", sm.GetID())
			continue
		}

		// Add user message to session
		sm.AppendMessage("user", []ai.Content{ai.TextContent{Text: input}})

		// Run agent with streaming
		err := runAgentLoopStreaming(ctx, ag, sm, sysPrompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

func runAgentLoopStreaming(ctx context.Context, a *agent.Agent, sm *coding_agent.SessionManager, systemPrompt string) error {
	// Get session entries and convert to LLM messages
	entries := sm.GetEntries()
	messages := convertToLLM(entries)

	// Convert agent tools to AI tools
	llmTools := convertTools(agentTools)

	conv := &ai.Context{
		SystemPrompt: systemPrompt,
		Messages:     messages,
		Tools:        llmTools,
	}

	stream, err := ai.Stream(ctx, a.Model(), conv, &ai.StreamOptions{})
	if err != nil {
		return err
	}

	for event := range stream.Events() {
		switch event.Type {
		case ai.EventStart:
			// Response started
		case ai.EventTextStart, ai.EventTextDelta:
			fmt.Print(event.Delta)
		case ai.EventThinkingStart, ai.EventThinkingDelta:
			// Thinking - could display differently
			fmt.Print(event.Delta)
		case ai.EventThinkingEnd:
			fmt.Println()
		case ai.EventToolCallEnd:
			// Execute tool call
			if event.ToolCall != nil {
				tc := event.ToolCall
				result := executeToolCall(*tc)
				sm.AppendToolResult(tc.ID, tc.Name, result.Content, result.IsError)
			}
		case ai.EventDone:
			// Save assistant message
			sm.AppendMessage("assistant", event.Message.Content)
			fmt.Printf("\n[tokens: %d in / %d out]\n",
				event.Message.Usage.Input,
				event.Message.Usage.Output,
			)
			return nil
		case ai.EventError:
			return fmt.Errorf("error: %s", event.Error.ErrorMessage)
		}
	}

	return nil
}

func createTools() []agent.AgentTool {
	return []agent.AgentTool{
		&codingAgentTool{
			name:        "Read",
			description: "Read a file from the filesystem",
			label:       "Read",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path to file"},
				},
				"required": []any{"path"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				data, err := os.ReadFile(path)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}}, nil
				}
				content := string(data)
				// Truncate if too large
				if len(content) > 50*1024 {
					content = content[:50*1024] + "\n... [truncated]"
				}
				return []ai.Content{ai.TextContent{Text: content}}, nil
			},
		},
		&codingAgentTool{
			name:        "Write",
			description: "Write content to a file",
			label:       "Write",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
				},
				"required": []any{"path", "content"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				content, _ := args["content"].(string)
				err := os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}}, nil
				}
				return []ai.Content{ai.TextContent{Text: "File written successfully"}}, nil
			},
		},
		&codingAgentTool{
			name:        "Edit",
			description: "Edit a file by replacing text",
			label:       "Edit",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string"},
					"oldString": map[string]any{"type": "string"},
					"newString": map[string]any{"type": "string"},
				},
				"required": []any{"path", "oldString"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				oldStr, _ := args["oldString"].(string)
				newStr, _ := args["newString"].(string)

				data, err := os.ReadFile(path)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}}, nil
				}

				content := string(data)
				if !strings.Contains(content, oldStr) {
					return []ai.Content{ai.TextContent{Text: "Old string not found in file"}}, nil
				}

				content = strings.Replace(content, oldStr, newStr, 1)
				err = os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}}, nil
				}
				return []ai.Content{ai.TextContent{Text: "File edited successfully"}}, nil
			},
		},
		&codingAgentTool{
			name:        "Bash",
			description: "Run a shell command",
			label:       "Bash",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
				"required": []any{"command"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				cmd, _ := args["command"].(string)
				output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
				if err != nil {
					return []ai.Content{ai.TextContent{Text: string(output) + "\nError: " + err.Error()}}, nil
				}
				return []ai.Content{ai.TextContent{Text: string(output)}}, nil
			},
		},
	}
}

// codingAgentTool is a simple tool wrapper
type codingAgentTool struct {
	name        string
	description string
	label       string
	params      map[string]any
	run         func(args map[string]any) ([]ai.Content, error)
}

func (t *codingAgentTool) ToolName() string               { return t.name }
func (t *codingAgentTool) ToolDescription() string        { return t.description }
func (t *codingAgentTool) ToolLabel() string              { return t.label }
func (t *codingAgentTool) ToolParameters() map[string]any { return t.params }

func (t *codingAgentTool) Execute(toolCallID string, args map[string]any, signal <-chan struct{}, onUpdate func(*agent.AgentToolResult[any])) (*agent.AgentToolResult[any], error) {
	content, err := t.run(args)
	return &agent.AgentToolResult[any]{Content: content}, err
}

func executeToolCall(tc ai.ToolCall) agent.ToolResultAgentMessage {
	for _, tool := range agentTools {
		if tool.ToolName() == tc.Name {
			result, err := tool.Execute(tc.ID, tc.Arguments, nil, nil)
			if err != nil {
				return agent.ToolResultAgentMessage{
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Content:    []ai.Content{ai.TextContent{Text: err.Error()}},
					IsError:    true,
				}
			}
			return agent.ToolResultAgentMessage{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content:    result.Content,
				IsError:    false,
			}
		}
	}
	return agent.ToolResultAgentMessage{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    []ai.Content{ai.TextContent{Text: "Tool not found"}},
		IsError:    true,
	}
}

func convertToLLM(entries []coding_agent.SessionEntry) []ai.Message {
	result := []ai.Message{}
	for _, e := range entries {
		if me, ok := e.(*coding_agent.MessageEntry); ok {
			switch me.Role {
			case "user":
				result = append(result, ai.UserMessage{Content: me.Content})
			case "assistant":
				result = append(result, ai.AssistantMessage{Content: me.Content})
			case "toolResult":
				result = append(result, ai.ToolResultMessage{Content: me.Content})
			}
		}
	}
	return result
}

func convertTools(agentTools []agent.AgentTool) []ai.Tool {
	result := []ai.Tool{}
	for _, t := range agentTools {
		result = append(result, ai.Tool{
			Name:        t.ToolName(),
			Description: t.ToolDescription(),
			Parameters:  t.ToolParameters(),
		})
	}
	return result
}

func printHelp() {
	fmt.Println(`Commands:
  /help     - Show this help
  /session - Show session info
  /exit    - Exit the program

Tools available:
  Read <path>    - Read a file
  Write <path> <content> - Write a file
  Edit <path> <old> <new> - Edit a file (replace old with new)
  Bash <command> - Run a shell command`)
}

func getCwd() string {
	cwd, _ := os.Getwd()
	return cwd
}
