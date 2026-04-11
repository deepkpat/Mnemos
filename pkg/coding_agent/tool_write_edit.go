package coding_agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"mnemos/pkg/ai"
)

// WriteToolResult contains the result of a file write
type WriteToolResult struct {
	Content  []ai.Content
	FilePath string
	Bytes    int
}

// WriteExecOptions configures file writing
type WriteExecOptions struct {
	Cwd        string
	CreateDirs bool
}

// DefaultWriteExecOptions returns default options for file writing
func DefaultWriteExecOptions(cwd string) *WriteExecOptions {
	return &WriteExecOptions{
		Cwd:        cwd,
		CreateDirs: true,
	}
}

// WriteFileToPath writes content to a file and returns the result
func WriteFileToPath(ctx context.Context, path string, content string, opts *WriteExecOptions) (*WriteToolResult, error) {
	if opts == nil {
		opts = DefaultWriteExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}

	// Resolve path
	absolutePath := path
	if !filepath.IsAbs(path) {
		absolutePath = filepath.Join(opts.Cwd, path)
	}

	// Create parent directories if needed
	if opts.CreateDirs {
		dir := filepath.Dir(absolutePath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}
		}
	}

	// Write the file
	if err := os.WriteFile(absolutePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return &WriteToolResult{
		Content:  []ai.Content{ai.TextContent{Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path)}},
		FilePath: absolutePath,
		Bytes:    len(content),
	}, nil
}

// RunWriteTool executes the write tool with the given input
func RunWriteTool(ctx context.Context, input *WriteInput, cwd string) (*WriteToolResult, error) {
	opts := DefaultWriteExecOptions(cwd)
	return WriteFileToPath(ctx, input.Path, input.Content, opts)
}

// EditToolResult contains the result of a file edit
type EditToolResult struct {
	Content        []ai.Content
	FilePath       string
	Diff           string
	FirstLine      int
	BlocksReplaced int
}

// EditExecOptions configures file editing
type EditExecOptions struct {
	Cwd string
}

// DefaultEditExecOptions returns default options for file editing
func DefaultEditExecOptions(cwd string) *EditExecOptions {
	return &EditExecOptions{
		Cwd: cwd,
	}
}

// EditFile edits a file with the given edits and returns the result
func EditFile(ctx context.Context, path string, edits []Edit, opts *EditExecOptions) (*EditToolResult, error) {
	if opts == nil {
		opts = DefaultEditExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}

	// Validate edits
	if len(edits) == 0 {
		return nil, fmt.Errorf("at least one edit is required")
	}
	for i, edit := range edits {
		if edit.OldText == "" {
			if len(edits) == 1 {
				return nil, fmt.Errorf("oldText must not be empty")
			}
			return nil, fmt.Errorf("edits[%d].oldText must not be empty", i)
		}
	}

	// Resolve path
	absolutePath := path
	if !filepath.IsAbs(path) {
		absolutePath = filepath.Join(opts.Cwd, path)
	}

	// Check if file exists
	if _, err := os.Stat(absolutePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	// Read the file
	rawContent, err := os.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Strip BOM and normalize
	_, text := stripBom(string(rawContent))
	normalizedContent := normalizeToLF(text)
	originalEnding := detectLineEnding(string(rawContent))

	// Apply edits
	result, err := applyEditsToNormalizedContent(normalizedContent, edits, path)
	if err != nil {
		return nil, err
	}

	// Restore line endings and write
	finalContent := restoreLineEndings(result.NewContent, originalEnding)
	if err := os.WriteFile(absolutePath, []byte(finalContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Generate diff
	diffResult := generateDiffString(result.BaseContent, result.NewContent, 4)

	return &EditToolResult{
		Content:        []ai.Content{ai.TextContent{Text: fmt.Sprintf("Successfully replaced %d block(s) in %s.", len(edits), path)}},
		FilePath:       absolutePath,
		Diff:           diffResult.Diff,
		FirstLine:      diffResult.FirstChangedLine,
		BlocksReplaced: len(edits),
	}, nil
}

// RunEditTool executes the edit tool with the given input
// Note: EditInput in tools.go currently uses a diff string, but the TypeScript version uses edits array
// This function adapts to use edits array for the actual implementation
func RunEditTool(ctx context.Context, path string, edits []Edit, cwd string) (*EditToolResult, error) {
	opts := DefaultEditExecOptions(cwd)
	return EditFile(ctx, path, edits, opts)
}
