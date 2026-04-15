package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
		"write": {
			Name:        "write",
			Description: "Write content to a file. For existing files, shows diff by default. Use apply=true to overwrite.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path to the file to write"},
					"content": map[string]any{"type": "string", "description": "Content to write to the file"},
					"apply":   map[string]any{"type": "boolean", "description": "Apply write (default: false for existing files, always for new files)"},
				},
				"required": []string{"path", "content"},
			},
		},
		"edit": {
			Name:        "edit",
			Description: "Edit a file by replacing exact text. Changes are applied automatically.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Path to the file to edit"},
					"oldText": map[string]any{"type": "string", "description": "Exact text to find and replace"},
					"newText": map[string]any{"type": "string", "description": "Replacement text"},
				},
				"required": []string{"path", "oldText", "newText"},
			},
		},
	}

	// Tool handlers
	handlers := map[string]ToolHandler{
		"bash":  bashTool,
		"read":  readTool,
		"ls":    lsTool,
		"grep":  grepTool,
		"find":  findTool,
		"write": writeTool,
		"edit":  editTool,
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
	err := findFilesRecursive(path, func(filePath string) error {
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

	// Convert glob pattern to regex for matching
	regexPattern := globToRegex(pattern)

	var matches []string
	err := findFilesRecursive(path, func(filePath string) error {
		// Get just the filename
		name := filepath.Base(filePath)
		// Match against the pattern
		if matchesPattern(name, regexPattern) {
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

// globToRegex converts a glob pattern to a regex pattern
func globToRegex(pattern string) string {
	// Handle ** for recursive matching
	pattern = strings.ReplaceAll(pattern, "**/", ".*/")
	pattern = strings.ReplaceAll(pattern, "**", ".*")

	// Escape special regex chars except glob wildcards
	var result strings.Builder
	for _, c := range pattern {
		switch c {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteByte('.')
		case '.', '(', ')', '+', '^', '$', '[', ']', '{', '}', '|', '\\':
			result.WriteString("\\")
			result.WriteRune(c)
		default:
			result.WriteRune(c)
		}
	}
	return result.String()
}

// matchesPattern checks if a filename matches the regex pattern
func matchesPattern(name string, pattern string) bool {
	if pattern == "" {
		return true
	}
	// For exact match (no wildcards), do simple string matching
	if !strings.Contains(pattern, ".*") && !strings.Contains(pattern, ".") {
		return strings.Contains(name, pattern)
	}
	// Use regex for patterns with wildcards
	matched, err := regexp.MatchString("^"+pattern+"$", name)
	return err == nil && matched
}

// Helper function to recursively find files
func findFilesRecursive(dir string, fn func(string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := dir + "/" + e.Name()
		if e.IsDir() {
			if err := findFilesRecursive(path, fn); err != nil {
				return err
			}
		} else {
			if err := fn(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeTool writes content to a file. For existing files, shows diff by default.
// Set apply=true to overwrite immediately.
func writeTool(name string, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing path argument")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing content argument")
	}

	// Check if apply flag is set
	apply := false
	if a, ok := args["apply"].(bool); ok {
		apply = a
	}

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directories: %v", err)
	}

	// Check if file exists
	oldData, err := os.ReadFile(path)
	if err != nil {
		// File doesn't exist - create it
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("failed to write file: %v", err)
		}
		return fmt.Sprintf("Created %s", path), nil
	}

	// File exists - compute diff
	oldLines := strings.Split(string(oldData), "\n")
	newLines := strings.Split(content, "\n")
	diff := computeDiff(oldLines, newLines, path)

	if diff == "" {
		return fmt.Sprintf("No changes: %s", path), nil
	}

	// If apply=false (default), show diff
	if !apply {
		return fmt.Sprintf("File exists - diff (use apply=true to overwrite):\n\n%s", diff), nil
	}

	// Apply the write
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}

	return fmt.Sprintf("Overwrote %s:\n\n%s", path, diff), nil
}

// computeDiff generates a simple unified diff-style output
func computeDiff(oldLines, newLines []string, path string) string {
	var buf strings.Builder

	oldSet := make(map[string]bool)
	for _, line := range oldLines {
		oldSet[line] = true
	}

	// Find added lines (in new but not in old)
	var added []string
	for _, line := range newLines {
		if !oldSet[line] {
			added = append(added, line)
		}
	}

	// Find removed lines (in old but not in new)
	newSet := make(map[string]bool)
	for _, line := range newLines {
		newSet[line] = true
	}

	var removed []string
	for _, line := range oldLines {
		if !newSet[line] {
			removed = append(removed, line)
		}
	}

	// Generate diff-like output
	if len(removed) > 0 {
		buf.WriteString("--- Removed\n")
		for i, line := range removed {
			buf.WriteString(fmt.Sprintf("- %s\n", line))
			if i >= 3 && len(removed) > 4 {
				buf.WriteString(fmt.Sprintf("  ... (%d more removed)", len(removed)-3))
				break
			}
		}
	}

	if len(added) > 0 {
		buf.WriteString("\n+++ Added\n")
		for i, line := range added {
			buf.WriteString(fmt.Sprintf("+ %s\n", line))
			if i >= 3 && len(added) > 4 {
				buf.WriteString(fmt.Sprintf("  ... (%d more added)", len(added)-3))
				break
			}
		}
	}

	return buf.String()
}

// editTool edits a file by showing diff and applying automatically.
// Ensures the file is within the current working directory.
func editTool(name string, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("missing path argument")
	}
	oldText, ok := args["oldText"].(string)
	if !ok {
		return "", fmt.Errorf("missing oldText argument")
	}
	newText, ok := args["newText"].(string)
	if !ok {
		return "", fmt.Errorf("missing newText argument")
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	// Resolve path to absolute
	absPath := path
	if !filepath.IsAbs(path) {
		absPath, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to resolve path: %v", err)
		}
	}

	// Normalize both paths for comparison
	cwdAbs, _ := filepath.Abs(cwd)
	absPathClean := filepath.Clean(absPath)
	cwdClean := filepath.Clean(cwdAbs)

	// Check if file is within current working directory
	if !strings.HasPrefix(absPathClean, cwdClean+string(filepath.Separator)) && absPathClean != cwdClean {
		return "", fmt.Errorf("file must be within working directory (%s): %s", cwd, path)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", path)
	}

	// Read the file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}
	content := string(data)

	// Check if oldText exists
	if !strings.Contains(content, oldText) {
		return "", fmt.Errorf("oldText not found in file")
	}

	// Compute what new content would look like
	newContent := strings.Replace(content, oldText, newText, 1)

	// Generate diff between old and new
	oldLines := strings.Split(content, "\n")
	newLines := strings.Split(newContent, "\n")
	diff := computeDiff(oldLines, newLines, absPath)

	if diff == "" {
		return fmt.Sprintf("No changes: %s", absPath), nil
	}

	// Apply the changes automatically
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}

	return fmt.Sprintf("Applied edit to %s:\n\n%s", absPath, diff), nil
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
		ContextWindow: 32000,
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
	// Optimized system prompt for small models (qwen3.5:0.8b)
	// Key: concise, one-tool-per-response, explicit stop condition
	systemPrompt := BuildSystemPrompt(cwd)

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
	maxIterations := 16

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

				// Print tool arguments
				args := make(map[string]any)
				for k, v := range ct.Arguments {
					args[k] = v
				}
				if len(args) > 0 {
					fmt.Printf("  Args: %+v\n", args)
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
	// Shorter version for interactive mode
	systemPrompt := BuildInteractivePrompt(cwd)

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

		// Allow multiple turns for interactive mode so agent can continue after tool results
		maxIterations := 8
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
					// Print tool arguments
					if len(args) > 0 {
						fmt.Printf("  Args: %+v\n", args)
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
