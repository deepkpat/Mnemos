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
	"time"

	"mnemos/pkg/agent"
	"mnemos/pkg/ai"
	"mnemos/pkg/ai/ollama"
	"mnemos/pkg/coding_agent"
)

// aiMessageDistiller is a distiller that works at the ai.Message level.
type aiMessageDistiller interface {
	Distill(messages []ai.Message) []ai.Message
}

// identityDistiller returns messages unchanged.
type identityDistiller struct{}

func (identityDistiller) Distill(messages []ai.Message) []ai.Message {
	return messages
}

// simpleDistillerFilterToolNames is the set of tool names to filter.
var simpleDistillerFilterToolNames = map[string]bool{
	"read":  true,
	"write": true,
	"edit":  true,
}

// simpleDistiller removes read/write/edit tool calls and replaces them with
// the actual file contents read from disk.
type simpleDistiller struct{}

func (d simpleDistiller) Distill(messages []ai.Message) []ai.Message {
	// First pass: filter tool result messages and collect modified files
	result := make([]ai.Message, 0, len(messages))
	modifiedFiles := make(map[string]bool) // path -> true

	for _, msg := range messages {
		switch m := msg.(type) {
		case ai.ToolResultMessage:
			// Skip read, write, edit tool results
			if simpleDistillerFilterToolNames[m.ToolName] {
				dummy := []ai.Content{
					ai.TextContent{Text: m.ToolName + " was called"},
				}
				result = append(result, ai.ToolResultMessage{
					ToolCallID: m.ToolCallID,
					ToolName:   m.ToolName,
					Content:    dummy,
				})
				continue
			}
			result = append(result, m)
		case ai.AssistantMessage:
			// Extract file paths from tool calls and filter them out
			filteredContent := make([]ai.Content, 0, len(m.Content))
			for _, c := range m.Content {
				if tc, ok := c.(ai.ToolCall); ok {
					if simpleDistillerFilterToolNames[tc.Name] {
						if path, ok := tc.Arguments["path"].(string); ok {
							modifiedFiles[path] = true
						}
						continue // Skip this tool call
					}
					filteredContent = append(filteredContent, c)
				} else {
					filteredContent = append(filteredContent, c)
				}
			}
			// Add assistant message without the filtered tool calls
			m.Content = filteredContent
			result = append(result, m)
		default:
			result = append(result, m)
		}
	}

	// If we have modified files, actually read them and add as tool result messages
	if len(modifiedFiles) > 0 {
		for path := range modifiedFiles {
			// Resolve path relative to CWD if not absolute
			absPath := path
			if !filepath.IsAbs(path) {
				absPath = filepath.Join(getCwd(), path)
			}
			data, err := os.ReadFile(absPath)
			prefix := "this is the most updated " + path + " previous tool results may be from an older version of the code \n"
			var content []ai.Content
			if err != nil {
				content = []ai.Content{ai.TextContent{Text: fmt.Sprintf("Error reading file: %v", err)}}
			} else {
				content = []ai.Content{ai.TextContent{Text: prefix + string(data)}}
			}
			// Add as a tool result message
			result = append(result, ai.ToolResultMessage{
				ToolCallID: "distilled-read-" + path,
				ToolName:   "read",
				Content:    content,
				IsError:    err != nil,
				Timestamp:  time.Now(),
			})
		}
	}

	return result
}

var (
	modelName          = flag.String("model", "qwen3.5:0.8b", "Model to use")
	sessionDir         = flag.String("session", "./.sessions", "Session directory")
	systemPrompt       = flag.String("system", "", "System prompt")
	systemPromptFile   = flag.String("system-prompt-file", "", "File to read system prompt from (overrides --system)")
	distiller          = flag.String("distiller", "simple", "Context distiller: identity, simple, none")
	agentTools         []agent.AgentTool
	maxTurns           = 16
	currentDistiller   aiMessageDistiller
)

func main() {
	flag.Parse()

	if *modelName == "" {
		fmt.Fprintf(os.Stderr, "Error: --model is required\n")
		os.Exit(1)
	}

	// Create distiller based on flag
	currentDistiller = createDistiller(*distiller)

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
	// If --system-prompt-file is provided, read from file (overrides --system)
	if *systemPromptFile != "" {
		data, err := os.ReadFile(*systemPromptFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading system prompt file: %v\n", err)
			os.Exit(1)
		}
		sysPrompt = string(data)
	}
	if sysPrompt == "" {
		sysPrompt = `## PHILOSOPHY
1. Code Quality: You write clean, maintainable, and "boring" code.
2. Simplicity: You follow SOLID and DRY principles; avoid over-engineering.
3. Language Agnostic: Adapt to the tech stack in the directory.

## RULES
1. Tool Limit: Only call ONE tool per response.
2. Loop: After execution, analyze the output and decide the next step.
3. Precision: The 'edit' tool requires an exact character-for-character match.
4. Boundary: Do not read/write files outside the current directory.
5. Response Format: Always trigger tools using JSON: {"tool": "name", "args": { ... }}
6. State Traceability: If a test or any code fails, you must explicitly write out the current state of variables (e.g., pointers, list order, or index values) before proposing a fix. Do not assume the test is wrong until you have mapped the pointer logic step-by-step.

## TOOLS
- ls({"path": "."}): List directory contents.
- read({"path": "filename"}): Read full file content.
- write({"path": "filename", "content": "..."}): Create/overwrite a file.
- edit({"path": "filename", "oldText": "...", "newText": "..."}): Exact string replacement.
- bash({"command": "..."}): Execute shell commands (tests, builds, etc.).

## EXECUTION PROCESS
1. Discovery: List files. If AGENTS.md exists, read it for specific constraints.
2. Analysis & Trace:
   - If fixing a bug: Briefly trace the execution of the failing code.
   - Identify the exact line where the actual state diverges from the expected state.
3. Planning: Explain your logic and the specific state change you intend to make.
4. Verification: After any 'write' or 'edit', use 'bash' to run tests. If the test still fails, re-read the file to ensure the 'edit' applied correctly before trying a different fix.`
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

		// Apply context distiller if configured
		if currentDistiller != nil {
			messages = currentDistiller.Distill(messages)
		}

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

func createDistiller(name string) aiMessageDistiller {
	switch name {
	case "identity":
		return identityDistiller{}
	case "simple":
		return simpleDistiller{}
	case "none":
		// No-op - return nil to skip distillation
		return nil
	default:
		fmt.Fprintf(os.Stderr, "Warning: unknown distiller %q, using identity\n", name)
		return identityDistiller{}
	}
}
