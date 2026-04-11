package coding_agent

import (
	"context"
	"time"

	"mnemos/pkg/ai"
)

// ToolHandler executes a tool with the given input and context.
// This is the same signature used in coding_agent.go
type ToolHandler func(ctx context.Context, input map[string]any) ([]ai.Content, error)

// ErrInvalidInput is returned when the input is invalid
var ErrInvalidInput = &InvalidInputError{}

type InvalidInputError struct{}

func (e *InvalidInputError) Error() string {
	return "invalid input"
}

// NewBashToolHandler creates a bash tool handler
func NewBashToolHandler(cwd string, timeout time.Duration) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		command, ok := input["command"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		bashInput := &BashInput{Command: command}
		result, err := RunBashTool(ctx, bashInput, cwd, timeout)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		if result.ExitCode != 0 {
			return result.Content, nil
		}

		return result.Content, nil
	}
}

// NewReadToolHandler creates a read tool handler
func NewReadToolHandler(cwd string) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		path, ok := input["path"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		readInput := &ReadInput{Path: path}
		if offset, ok := input["offset"].(float64); ok {
			readInput.Offset = int(offset)
		}
		if limit, ok := input["limit"].(float64); ok {
			readInput.Limit = int(limit)
		}

		result, err := RunReadTool(ctx, readInput, cwd)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		return result.Content, nil
	}
}

// NewWriteToolHandler creates a write tool handler
func NewWriteToolHandler(cwd string) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		path, ok := input["path"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}
		content, ok := input["content"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		writeInput := &WriteInput{Path: path, Content: content}
		result, err := RunWriteTool(ctx, writeInput, cwd)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		return result.Content, nil
	}
}

// NewEditToolHandler creates an edit tool handler
func NewEditToolHandler(cwd string) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		path, ok := input["path"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		editsIface, ok := input["edits"]
		if !ok {
			return nil, ErrInvalidInput
		}

		editsArray, ok := editsIface.([]any)
		if !ok {
			return nil, ErrInvalidInput
		}

		var edits []Edit
		for _, e := range editsArray {
			editMap, ok := e.(map[string]any)
			if !ok {
				continue
			}
			oldText, _ := editMap["oldText"].(string)
			newText, _ := editMap["newText"].(string)
			if oldText != "" {
				edits = append(edits, Edit{OldText: oldText, NewText: newText})
			}
		}

		if len(edits) == 0 {
			return nil, ErrInvalidInput
		}

		result, err := RunEditTool(ctx, path, edits, cwd)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		return result.Content, nil
	}
}

// NewLsToolHandler creates an ls tool handler
func NewLsToolHandler(cwd string) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		path := "."
		if p, ok := input["path"].(string); ok {
			path = p
		}

		lsInput := &LsInput{Path: path}
		if limit, ok := input["limit"].(float64); ok {
			lsInput.Limit = int(limit)
		}

		result, err := RunLsTool(ctx, lsInput, cwd)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		return result.Content, nil
	}
}

// NewGrepToolHandler creates a grep tool handler
func NewGrepToolHandler(cwd string) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		pattern, ok := input["pattern"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		path := "."
		if p, ok := input["path"].(string); ok {
			path = p
		}

		grepInput := &GrepInput{Pattern: pattern, Path: path}
		if glob, ok := input["glob"].(string); ok {
			grepInput.Include = glob
		}
		if ignoreCase, ok := input["ignoreCase"].(bool); ok {
			grepInput.IgnoreCase = ignoreCase
		}
		if ctxVal, ok := input["context"].(float64); ok {
			grepInput.Context = int(ctxVal)
		}
		if limit, ok := input["limit"].(float64); ok {
			grepInput.Limit = int(limit)
		}

		result, err := RunGrepTool(ctx, grepInput, cwd)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		return result.Content, nil
	}
}

// NewFindToolHandler creates a find tool handler
func NewFindToolHandler(cwd string) ToolHandler {
	return func(ctx context.Context, input map[string]any) ([]ai.Content, error) {
		pattern, ok := input["pattern"].(string)
		if !ok {
			if n, ok := input["name"].(string); ok {
				pattern = n
			} else {
				return nil, ErrInvalidInput
			}
		}

		path := "."
		if p, ok := input["path"].(string); ok {
			path = p
		}

		findInput := &FindInput{Name: pattern, Path: path}
		if limit, ok := input["limit"].(float64); ok {
			findInput.Limit = int(limit)
		}

		result, err := RunFindTool(ctx, findInput, cwd)
		if err != nil {
			return []ai.Content{ai.TextContent{Text: err.Error()}}, nil
		}

		return result.Content, nil
	}
}

// AllToolHandlers returns a map of all available tools for a given working directory
func AllToolHandlers(cwd string, timeout time.Duration) map[string]ToolHandler {
	return map[string]ToolHandler{
		"bash":  NewBashToolHandler(cwd, timeout),
		"read":  NewReadToolHandler(cwd),
		"write": NewWriteToolHandler(cwd),
		"edit":  NewEditToolHandler(cwd),
		"ls":    NewLsToolHandler(cwd),
		"grep":  NewGrepToolHandler(cwd),
		"find":  NewFindToolHandler(cwd),
	}
}
