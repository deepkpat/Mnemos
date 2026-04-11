package coding_agent

import (
	"mnemos/pkg/agent"
	"mnemos/pkg/ai"
)

// ============================================================================
// Tool Definitions
// ============================================================================

// ToolDefinition defines a tool's interface for LLM interaction.
type ToolDefinition struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Label       string        `json:"label"`
	Parameters  interface{}   `json:"parameters"` // JSON Schema
	Details     string        `json:"details,omitempty"`
	Examples    []ToolExample `json:"examples,omitempty"`
}

// ToolExample is an example of tool usage.
type ToolExample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

// ============================================================================
// Tool Implementations
// ============================================================================

// BashToolDefinition returns the definition for the bash tool.
func BashToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "bash",
		Description: "Executes a shell command and returns its output. Use for running git, npm, compilation, testing, or any shell operation.",
		Label:       "Bash",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
			},
			"required": []string{"command"},
		},
		Details: "Runs in the working directory. Returns stdout, stderr, and exit code.",
		Examples: []ToolExample{
			{Input: `{"command": "git status"}`, Output: "Shows git status"},
			{Input: `{"command": "npm test"}`, Output: "Runs tests"},
		},
	}
}

// ReadToolDefinition returns the definition for the read tool.
func ReadToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "read",
		Description: "Reads a file and returns its contents. Supports truncation to limit output size.",
		Label:       "Read",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read",
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Line number to start from (1-based, first line is 1)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of lines to read",
				},
				"showLines": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to show line numbers",
				},
			},
			"required": []string{"path"},
		},
		Details: "Use offset and limit for large files. Returns truncated output if file is too large.",
		Examples: []ToolExample{
			{Input: `{"path": "/src/main.ts"}`, Output: "File contents"},
			{Input: `{"path": "/src/main.ts", "offset": 100, "limit": 50}`, Output: "Lines 100-149"},
		},
	}
}

// WriteToolDefinition returns the definition for the write tool.
func WriteToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "write",
		Description: "Writes content to a file. Creates parent directories if needed.",
		Label:       "Write",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
		Details: "Creates the file if it doesn't exist. Overwrites existing files.",
		Examples: []ToolExample{
			{Input: `{"path": "/src/test.ts", "content": "console.log('hello')"}`, Output: "File written"},
		},
	}
}

// EditToolDefinition returns the definition for the edit tool.
func EditToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "edit",
		Description: "Edits a file by replacing text with new content.",
		Label:       "Edit",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to edit",
				},
				"edits": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"oldText": map[string]interface{}{
								"type":        "string",
								"description": "The text to find and replace (must match exactly)",
							},
							"newText": map[string]interface{}{
								"type":        "string",
								"description": "The replacement text",
							},
						},
						"required": []string{"oldText", "newText"},
					},
					"description": "Array of edits to apply",
				},
			},
			"required": []string{"path", "edits"},
		},
		Details: "Each edit replaces oldText with newText. Text must match exactly including whitespace and newlines.",
		Examples: []ToolExample{
			{Input: `{"path": "/src/main.ts", "edits": [{"oldText": "const x = 1;", "newText": "const x = 2;"}]}`, Output: "Replaced text"},
		},
	}
}

// LsToolDefinition returns the definition for the ls tool.
func LsToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "ls",
		Description: "Lists directory contents with optional sorting.",
		Label:       "List",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the directory to list",
				},
				"all": map[string]interface{}{
					"type":        "boolean",
					"description": "Show hidden files",
				},
				"long": map[string]interface{}{
					"type":        "boolean",
					"description": "Show detailed listing",
				},
				"sortBy": map[string]interface{}{
					"type":        "string",
					"description": "Sort by: name, size, modified",
				},
			},
		},
		Details: "Shows files and directories. Type indicators: / (dir), * (executable), @ (link).",
		Examples: []ToolExample{
			{Input: `{"path": "/src"}`, Output: "Directory listing"},
			{Input: `{"path": "/src", "long": true, "sortBy": "modified"}`, Output: "Detailed, sorted by time"},
		},
	}
}

// GrepToolDefinition returns the definition for the grep tool.
func GrepToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "grep",
		Description: "Searches for patterns in files.",
		Label:       "Grep",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regular expression to search for",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to search in",
				},
				"include": map[string]interface{}{
					"type":        "string",
					"description": "File pattern to include (e.g., *.ts)",
				},
				"glob": map[string]interface{}{
					"type":        "string",
					"description": "File pattern to include (e.g., *.ts). Alias for include.",
				},
				"exclude": map[string]interface{}{
					"type":        "string",
					"description": "File pattern to exclude",
				},
				"context": map[string]interface{}{
					"type":        "integer",
					"description": "Number of context lines to show",
				},
				"ignoreCase": map[string]interface{}{
					"type":        "boolean",
					"description": "Case insensitive search",
				},
			},
			"required": []string{"pattern", "path"},
		},
		Details: "Returns matches with line numbers and context.",
		Examples: []ToolExample{
			{Input: `{"pattern": "function", "path": "/src"}`, Output: "Matches with context"},
			{Input: `{"pattern": "TODO", "path": "/src", "ignoreCase": true}`, Output: "Case insensitive"},
		},
	}
}

// FindToolDefinition returns the definition for the find tool.
func FindToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "find",
		Description: "Finds files and directories by name or type.",
		Label:       "Find",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Root path to search from",
				},
				"types": map[string]interface{}{
					"type":        "string",
					"description": "Types to find: f (files), d (dirs), l (links)",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Name pattern (glob)",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Name pattern (glob). Alias for name.",
				},
				"maxDepth": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum directory depth",
				},
			},
			"required": []string{"path"},
		},
		Details: "Finds files matching name pattern or type.",
		Examples: []ToolExample{
			{Input: `{"path": "/src", "name": "*.ts"}`, Output: "TypeScript files"},
			{Input: `{"path": "/src", "types": "d"}`, Output: "Directories only"},
		},
	}
}

// AllToolDefinitions returns all coding tool definitions.
func AllToolDefinitions() []ToolDefinition {
	return []ToolDefinition{
		BashToolDefinition(),
		ReadToolDefinition(),
		WriteToolDefinition(),
		EditToolDefinition(),
		LsToolDefinition(),
		GrepToolDefinition(),
		FindToolDefinition(),
	}
}

// ============================================================================
// Tool Interface (for pkg/agent integration)
// ============================================================================

// CodingTool implements agent.AgentTool for coding tools.
type CodingTool struct {
	Name        string
	Description string
	Label       string
	Parameters  map[string]interface{}
	Categories  []string
	Run         func(input map[string]any, signal <-chan struct{}) ([]ai.Content, error)
}

// ToolName returns the tool name.
func (t *CodingTool) ToolName() string { return t.Name }

// ToolDescription returns the tool description.
func (t *CodingTool) ToolDescription() string { return t.Description }

// ToolLabel returns the tool label.
func (t *CodingTool) ToolLabel() string { return t.Label }

// ToolParameters returns the tool parameters.
func (t *CodingTool) ToolParameters() map[string]any {
	if t.Parameters == nil {
		return nil
	}
	return t.Parameters
}

// Execute runs the tool.
func (t *CodingTool) Execute(toolCallID string, args map[string]any, signal <-chan struct{}, onUpdate func(*agent.AgentToolResult[any])) (*agent.AgentToolResult[any], error) {
	content, err := t.Run(args, signal)
	if err != nil {
		return &agent.AgentToolResult[any]{
			Content: []ai.Content{ai.TextContent{Text: err.Error()}},
			Details: nil,
		}, nil
	}
	return &agent.AgentToolResult[any]{
		Content: content,
		Details: nil,
	}, nil
}

// NewBashTool creates a new bash tool with the given executor.
func NewBashTool(exec func(string, *BashOptions) (*BashResult, error)) *CodingTool {
	return &CodingTool{
		Name:        "bash",
		Description: "Executes a shell command",
		Label:       "Bash",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{"type": "string"},
			},
			"required": []string{"command"},
		},
		Categories: []string{"tool"},
		Run: func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
			cmd, _ := input["command"].(string)
			opts := DefaultBashOptions()
			result, err := exec(cmd, opts)
			if err != nil {
				return []ai.Content{ai.TextContent{Text: err.Error()}}, err
			}
			return result.Content, nil
		},
	}
}
