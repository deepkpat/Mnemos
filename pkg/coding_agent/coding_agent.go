package coding_agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"mnemos/pkg/agent"
	"mnemos/pkg/ai"
)

// ============================================================================
// CodingAgent
// ============================================================================

// CodingAgent is the main interface for the coding agent.
// It wraps the base agent with coding-specific tools and session management.
type CodingAgent struct {
	mu      sync.RWMutex
	agent   *agent.Agent
	session *Session
	cwd     string
	tools   map[string]ToolExecutor

	// Event handlers
	handlers map[string]EventHandler
}

type EventHandler func(event CodingEvent)

type CodingEvent struct {
	Type string
	Data interface{}
}

// ToolExecutor executes a tool with the given input.
type ToolExecutor func(input map[string]any, signal <-chan struct{}) ([]ai.Content, error)

// NewCodingAgent creates a new coding agent.
func NewCodingAgent(opts *CodingAgentOptions) (*CodingAgent, error) {
	if opts == nil {
		opts = DefaultCodingAgentOptions()
	}

	// Get working directory
	cwd := opts.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("coding_agent: failed to get cwd: %w", err)
		}
	}

	// Create base agent
	agentOpts := &agent.AgentOptions{
		InitialState: &agent.AgentState{
			SystemPrompt: buildSystemPrompt(cwd),
			Model:        opts.Model,
		},
		TransformContext: nil,
		StreamFn:         opts.StreamFn,
		GetAPIKey:        opts.GetAPIKey,
	}
	baseAgent := agent.New(agentOpts)

	// Create coding agent
	ca := &CodingAgent{
		agent:    baseAgent,
		cwd:      cwd,
		tools:    make(map[string]ToolExecutor),
		handlers: make(map[string]EventHandler),
	}

	// Register default tools
	if opts.RegisterDefaultTools {
		ca.RegisterDefaultTools()
	}

	// Create or load session
	if opts.SessionDir != "" {
		session, err := NewSession(opts.SessionDir, &SessionOptions{
			Cwd:      cwd,
			Provider: opts.Model.Provider,
			Model:    opts.Model.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("coding_agent: failed to create session: %w", err)
		}
		ca.session = session
	}

	return ca, nil
}

// Prompt sends a prompt to the agent.
func (ca *CodingAgent) Prompt(ctx context.Context, text string) error {
	return ca.agent.Prompt(text)
}

// PromptWithImages sends a prompt with images.
func (ca *CodingAgent) PromptWithImages(ctx context.Context, text string, images []ai.Content) error {
	return ca.agent.PromptWithImages(text, images)
}

// PromptMessage sends an agent message.
func (ca *CodingAgent) PromptMessage(ctx context.Context, msg agent.AgentMessage) error {
	return ca.agent.PromptMessage(msg)
}

// Continue continues from the current context.
func (ca *CodingAgent) Continue(ctx context.Context) error {
	return ca.agent.Continue()
}

// Wait waits for the current run to complete.
func (ca *CodingAgent) Wait() {
	ca.agent.Wait()
}

// Abort aborts the current run.
func (ca *CodingAgent) Abort() {
	ca.agent.Abort()
}

// Subscribe subscribes to agent events.
func (ca *CodingAgent) Subscribe(handler EventHandler) func() {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	id := fmt.Sprintf("handler_%d", len(ca.handlers))
	ca.handlers[id] = handler

	return func() {
		ca.mu.Lock()
		defer ca.mu.Unlock()
		delete(ca.handlers, id)
	}
}

// RegisterTool registers a tool executor.
func (ca *CodingAgent) RegisterTool(name string, executor ToolExecutor) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.tools[name] = executor
}

// RegisterDefaultTools registers the default coding tools.
func (ca *CodingAgent) RegisterDefaultTools() {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	// Register bash tool
	ca.tools["bash"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		cmd, _ := input["command"].(string)
		result, err := runBash(cmd, ca.cwd)
		return result, err
	}

	// Register read tool
	ca.tools["read"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		path, _ := input["path"].(string)
		result, err := runRead(path)
		return result, err
	}

	// Register write tool
	ca.tools["write"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		path, _ := input["path"].(string)
		content, _ := input["content"].(string)
		result, err := runWrite(path, content)
		return result, err
	}

	// Register edit tool
	ca.tools["edit"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		path, _ := input["path"].(string)
		diff, _ := input["diff"].(string)
		result, err := runEdit(path, diff)
		return result, err
	}

	// Register ls tool
	ca.tools["ls"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		path, _ := input["path"].(string)
		if path == "" {
			path = "."
		}
		result, err := runLs(path)
		return result, err
	}

	// Register grep tool
	ca.tools["grep"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		result, err := runGrep(pattern, path)
		return result, err
	}

	// Register find tool
	ca.tools["find"] = func(input map[string]any, _ <-chan struct{}) ([]ai.Content, error) {
		path, _ := input["path"].(string)
		name, _ := input["name"].(string)
		result, err := runFind(path, name)
		return result, err
	}
}

// ============================================================================
// State Access
// ============================================================================

// State returns the current agent state.
func (ca *CodingAgent) State() agent.AgentState {
	return ca.agent.State()
}

// Model returns the current model.
func (ca *CodingAgent) Model() *ai.Model {
	return ca.agent.Model()
}

// SetModel sets the current model.
func (ca *CodingAgent) SetModel(model *ai.Model) {
	ca.agent.SetModel(model)
}

// Messages returns the conversation messages.
func (ca *CodingAgent) Messages() []agent.AgentMessage {
	return ca.agent.Messages()
}

// IsStreaming returns whether a response is being generated.
func (ca *CodingAgent) IsStreaming() bool {
	return ca.agent.IsStreaming()
}

// Session returns the current session.
func (ca *CodingAgent) Session() *Session {
	ca.mu.RLock()
	defer ca.mu.RUnlock()
	return ca.session
}

// ============================================================================
// CodingAgent Options
// ============================================================================

// CodingAgentOptions configures a new coding agent.
type CodingAgentOptions struct {
	Cwd                  string
	Model                *ai.Model
	StreamFn             agent.StreamFunction
	GetAPIKey            func(provider string) (string, error)
	SessionDir           string
	RegisterDefaultTools bool
}

// DefaultCodingAgentOptions returns default options.
func DefaultCodingAgentOptions() *CodingAgentOptions {
	return &CodingAgentOptions{
		RegisterDefaultTools: true,
		Cwd:                  "",
	}
}

// ============================================================================
// System Prompt
// ============================================================================

func buildSystemPrompt(cwd string) string {
	return fmt.Sprintf(`You are an expert coding assistant. You have access to tools for file operations:
- read: Read file contents
- write: Write files
- edit: Edit files using ed-style diffs
- bash: Execute shell commands
- ls: List directory contents
- grep: Search for patterns
- find: Find files by name

Working directory: %s

Use tools appropriately to complete tasks.`, cwd)
}

// ============================================================================
// Tool Implementations (basic)
// ============================================================================

func runBash(cmd, cwd string) ([]ai.Content, error) {
	// Basic implementation - uses os/exec
	return []ai.Content{ai.TextContent{
		Text: fmt.Sprintf("bash: command execution not implemented in basic mode"),
	}}, nil
}

func runRead(path string) ([]ai.Content, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return []ai.Content{ai.TextContent{Text: err.Error()}}, err
	}
	return []ai.Content{ai.TextContent{Text: string(data)}}, nil
}

func runWrite(path, content string) ([]ai.Content, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return []ai.Content{ai.TextContent{Text: err.Error()}}, err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return []ai.Content{ai.TextContent{Text: err.Error()}}, err
	}
	return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Written to %s", path)}}, nil
}

func runEdit(path, diff string) ([]ai.Content, error) {
	// Basic ed-style diff implementation
	return []ai.Content{ai.TextContent{Text: fmt.Sprintf("Edit not fully implemented: %s", diff)}}, nil
}

func runLs(path string) ([]ai.Content, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return []ai.Content{ai.TextContent{Text: err.Error()}}, err
	}
	var output string
	for _, e := range entries {
		t := "file"
		if e.IsDir() {
			t = "dir"
		}
		output += fmt.Sprintf("%s (%s)\n", e.Name(), t)
	}
	return []ai.Content{ai.TextContent{Text: output}}, nil
}

func runGrep(pattern, path string) ([]ai.Content, error) {
	return []ai.Content{ai.TextContent{Text: "grep not implemented"}}, nil
}

func runFind(path, name string) ([]ai.Content, error) {
	return []ai.Content{ai.TextContent{Text: "find not implemented"}}, nil
}
