package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	maxTurns     = 10
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
		ollama.WithReasoning(),
		ollama.WithContextWindow(32_000),
		ollama.WithMaxTokens(4096),
	)

	// Set up system prompt if provided
	sysPrompt := *systemPrompt
	if sysPrompt == "" {
		sysPrompt = `You are a Principal Software Engineer. You are currently working in ` + getCwd() + `

## PHILOSOPHY
1. Code Quality: You write clean, maintainable, and "boring" code.
2. Simplicity: You follow YAGNI (You Ain't Gonna Need It) and avoid over-engineering.
3. Language Agnostic: You adapt to the tech stack found in the directory.

## RULES
1. Tool Limit: Only call ONE tool per response.
2. Loop: After tool execution, you will receive the output. Analyze it, then decide whether to use another tool or provide a final answer.
3. Precision: The 'edit' tool requires an exact character-for-character match of 'oldText'.
4. Boundary: Do not attempt to read or write files outside of '{{getCwd()}}'.
5. Response Format: You must always trigger tools using the JSON format: {"tool": "name", "args": { ... }}
6. File Access: You MUST stay inside the current directory, the tool calls should strictly NEVER affect or see files outside the current dir for example if my current directory is /home/username/project, the i can NOT access '/' or '/home' or '/home/username' or '/home/username2/project2'

## TOOLS
- ls({"path": "."}): List directory contents to explore the project structure.
- read({"path": "filename"}): Read the full content of a specific file.
- write({"path": "filename", "content": "..."}): Create or overwrite a file.
- edit({"path": "filename", "oldText": "...", "newText": "..."}): Replace an exact string with new text.
- bash({"command": "..."}): Execute shell commands (compilers, tests, linters, or package managers).

## EXECUTION PROCESS
1. Discovery: Start by listing files to identify the programming language and architecture.
2. Planning: Briefly explain your logic before issuing a tool call.
3. Verification: After modifying files, always use the 'bash' tool to run the appropriate test or build command for that environment to ensure stability.`
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
		err := runAgentLoopStreaming(ctx, ag, sm)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		}
	}
}

func runAgentLoopStreaming(ctx context.Context, a *agent.Agent, sm *coding_agent.SessionManager) error {
	for turn := 0; turn < maxTurns; turn++ {
		// Get session entries and convert to LLM messages
		entries := sm.GetEntries()
		messages := convertToLLM(entries)

		// Convert agent tools to AI tools
		llmTools := convertTools(agentTools)

		// Get initial state for sys prompt (could change if we want)
		sysPrompt := a.State().SystemPrompt

		conv := &ai.Context{
			SystemPrompt: sysPrompt,
			Messages:     messages,
			Tools:        llmTools,
		}

		stream, err := ai.Stream(ctx, a.Model(), conv, &ai.StreamOptions{
			ThinkingBudget: 500,
		})
		if err != nil {
			return err
		}

		var toolCalls []ai.ToolCall
		var assistantContent []ai.Content

		fmt.Printf("\n[Turn %d]\n", turn+1)

		for event := range stream.Events() {
			switch event.Type {
			case ai.EventThinkingStart:
				fmt.Print("\n[Thinking]\n")
			case ai.EventThinkingDelta:
				fmt.Print(event.Delta)
			case ai.EventThinkingEnd:
				fmt.Print("\n")

			case ai.EventTextStart:
				fmt.Print("\n[Assistant]\n")
			case ai.EventTextDelta:
				fmt.Print(event.Delta)
			case ai.EventTextEnd:
				fmt.Print("\n")

			case ai.EventToolCallStart:
				// Start of a tool call
			case ai.EventToolCallEnd:
				if event.ToolCall != nil {
					toolCalls = append(toolCalls, *event.ToolCall)
				}

			case ai.EventDone:
				assistantContent = event.Message.Content
				sm.AppendMessage("assistant", assistantContent)

				usage := event.Message.Usage
				if usage.Input > 0 || usage.Output > 0 {
					fmt.Printf("\n[Usage: %d in / %d out]\n", usage.Input, usage.Output)
				}

			case ai.EventError:
				return fmt.Errorf("ai error: %s", event.Error.ErrorMessage)
			}
		}

		// If no tool calls, we're done with the interaction
		if len(toolCalls) == 0 {
			return nil
		}

		// Execute tool calls and append results
		for _, tc := range toolCalls {
			fmt.Printf("\n[Tool: %s]\nArgs: %v\n", tc.Name, tc.Arguments)
			result := executeToolCall(tc)
			sm.AppendToolResult(tc.ID, tc.Name, result.Content, result.IsError)

			// Print short summary of result
			resText := ""
			for _, c := range result.Content {
				if t, ok := c.(ai.TextContent); ok {
					resText += t.Text
				}
			}
			if len(resText) > 200 {
				resText = resText[:200] + "..."
			}
			fmt.Printf("Result: %s\n", resText)
		}
	}

	return fmt.Errorf("reached maximum turns (%d)", maxTurns)
}

func createTools() []agent.AgentTool {
	return []agent.AgentTool{
		&codingAgentTool{
			name:        "read",
			description: "Read a file and return its content",
			label:       "Read",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Path to the file"},
				},
				"required": []any{"path"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				// Resolve path relative to CWD if not absolute
				if !filepath.IsAbs(path) {
					path = filepath.Join(getCwd(), path)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error reading file: %v", err)}}, nil
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
			name:        "write",
			description: "Write full content to a file",
			label:       "Write",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path to file"},
					"content": map[string]any{"type": "string", "description": "Full file content"},
				},
				"required": []any{"path", "content"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				content, _ := args["content"].(string)

				if !filepath.IsAbs(path) {
					path = filepath.Join(getCwd(), path)
				}

				// Create parent directories
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error creating directories: %v", err)}}, nil
				}

				err := os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error writing file: %v", err)}}, nil
				}
				return []ai.Content{ai.TextContent{Text: "File written successfully"}}, nil
			},
		},
		&codingAgentTool{
			name:        "edit",
			description: "Edit a file by replacing oldText with newText",
			label:       "Edit",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path to file"},
					"oldText": map[string]any{"type": "string", "description": "Exact text to replace"},
					"newText": map[string]any{"type": "string", "description": "New replacement text"},
				},
				"required": []any{"path", "oldText", "newText"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				oldStr, _ := args["oldText"].(string)
				newStr, _ := args["newText"].(string)

				if !filepath.IsAbs(path) {
					path = filepath.Join(getCwd(), path)
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error reading file: %v", err)}}, nil
				}

				content := string(data)
				if !strings.Contains(content, oldStr) {
					return []ai.Content{ai.TextContent{Text: "Error: oldText not found in file. Make sure it matches exactly including whitespace."}}, nil
				}

				content = strings.Replace(content, oldStr, newStr, 1)
				err = os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error writing file: %v", err)}}, nil
				}
				return []ai.Content{ai.TextContent{Text: "File edited successfully"}}, nil
			},
		},
		&codingAgentTool{
			name:        "bash",
			description: "Run a shell command and return its output",
			label:       "Bash",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "The command to run"},
				},
				"required": []any{"command"},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				cmdStr, _ := args["command"].(string)
				c := exec.Command("sh", "-c", cmdStr)
				c.Dir = getCwd()
				output, err := c.CombinedOutput()
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Output: %s\nError: %v", string(output), err)}}, nil
				}
				return []ai.Content{ai.TextContent{Text: string(output)}}, nil
			},
		},
		&codingAgentTool{
			name:        "ls",
			description: "List directory contents",
			label:       "List",
			params: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Directory to list"},
				},
			},
			run: func(args map[string]any) ([]ai.Content, error) {
				path, _ := args["path"].(string)
				if path == "" {
					path = "."
				}
				if !filepath.IsAbs(path) {
					path = filepath.Join(getCwd(), path)
				}
				entries, err := os.ReadDir(path)
				if err != nil {
					return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error: %v", err)}}, nil
				}
				var names []string
				for _, e := range entries {
					if e.IsDir() {
						names = append(names, e.Name()+"/")
					} else {
						names = append(names, e.Name())
					}
				}
				return []ai.Content{ai.TextContent{Text: strings.Join(names, "\n")}}, nil
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
		switch entry := e.(type) {
		case *coding_agent.MessageEntry:
			switch entry.Role {
			case "user":
				result = append(result, ai.UserMessage{Content: entry.Content})
			case "assistant":
				result = append(result, ai.AssistantMessage{Content: entry.Content})
			}
		case *coding_agent.ToolResultEntry:
			result = append(result, ai.ToolResultMessage{
				ToolCallID: entry.ToolCallID,
				ToolName:   entry.ToolName,
				Content:    entry.Content,
				IsError:    entry.IsError,
			})
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
  read <path>
  write <path> <content>
  edit <path> <oldText> <newText>
  ls <path>
  bash <command>`)
}

func getCwd() string {
	cwd, _ := os.Getwd()
	return cwd
}
