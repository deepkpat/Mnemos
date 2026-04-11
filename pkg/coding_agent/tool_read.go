package coding_agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mnemos/pkg/ai"
)

// ReadToolResult contains the result of a file read
type ReadToolResult struct {
	Content     []ai.Content
	Truncated   bool
	TotalLines  int
	OutputLines int
}

// ReadExecOptions configures file reading
type ReadExecOptions struct {
	Cwd      string
	Offset   int // 0-based offset, -1 means from start
	Limit    int // -1 means no limit
	MaxBytes int
	MaxLines int
}

// DefaultReadExecOptions returns default options for file reading
func DefaultReadExecOptions(cwd string) *ReadExecOptions {
	return &ReadExecOptions{
		Cwd:      cwd,
		Offset:   0,
		Limit:    -1,
		MaxBytes: DefaultMaxBytes,
		MaxLines: DefaultMaxLines,
	}
}

// detectImageMimeType detects the MIME type of an image file
func detectImageMimeType(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".bmp":
		return "image/bmp", true
	default:
		return "", false
	}
}

// ReadFile executes a read operation and returns the result
func ReadFile(ctx context.Context, path string, opts *ReadExecOptions) (*ReadToolResult, error) {
	if opts == nil {
		opts = DefaultReadExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	if opts.MaxLines == 0 {
		opts.MaxLines = DefaultMaxLines
	}

	// Resolve path
	absolutePath := path
	if !filepath.IsAbs(path) {
		absolutePath = filepath.Join(opts.Cwd, path)
	}

	// Check if file exists
	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Check if it's a directory
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", path)
	}

	// Check for image file
	mimeType, isImage := detectImageMimeType(absolutePath)
	if isImage {
		// Read image as base64
		data, err := os.ReadFile(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read image: %w", err)
		}
		// For now, return a text note about the image instead of actual image content
		// In a full implementation, we'd handle image resizing
		return &ReadToolResult{
			Content: []ai.Content{
				ai.TextContent{Text: fmt.Sprintf("Read image file [%s] (%d bytes)", mimeType, len(data))},
			},
			Truncated:   false,
			TotalLines:  1,
			OutputLines: 1,
		}, nil
	}

	// Read text file
	content, err := os.ReadFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	textContent := string(content)
	allLines := strings.Split(textContent, "\n")
	totalFileLines := len(allLines)

	// Apply offset (convert from 1-indexed to 0-indexed)
	startLine := opts.Offset
	if startLine < 0 {
		startLine = 0
	}
	startLineDisplay := startLine + 1

	// Check if offset is out of bounds
	if startLine >= len(allLines) {
		return nil, fmt.Errorf("offset %d is beyond end of file (%d lines total)", startLineDisplay, len(allLines))
	}

	// Apply limit
	endLine := len(allLines)
	if opts.Limit > 0 {
		endLine = startLine + opts.Limit
		if endLine > len(allLines) {
			endLine = len(allLines)
		}
	}

	// Get the selected content
	selectedContent := strings.Join(allLines[startLine:endLine], "\n")
	userLimitedLines := endLine - startLine

	// Apply truncation
	truncation := truncateHead(selectedContent, TruncationOptions{
		MaxLines: opts.MaxLines,
		MaxBytes: opts.MaxBytes,
	})

	result := &ReadToolResult{
		Content:     []ai.Content{ai.TextContent{Text: truncation.Content}},
		Truncated:   truncation.Truncated,
		TotalLines:  totalFileLines,
		OutputLines: truncation.OutputLines,
	}

	// Add notices based on truncation
	var notice string
	if truncation.FirstLineExceedsLimit {
		notice = fmt.Sprintf("\n\n[Line %d exceeds %s limit. Use offset=%d to continue.]",
			startLineDisplay, FormatSize(opts.MaxBytes), startLineDisplay+1)
		result.Content = []ai.Content{ai.TextContent{Text: truncation.Content + notice}}
	} else if truncation.Truncated {
		endLineDisplay := startLineDisplay + truncation.OutputLines - 1
		nextOffset := endLineDisplay + 1
		if truncation.TruncatedBy == "lines" {
			notice = fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]",
				startLineDisplay, endLineDisplay, totalFileLines, nextOffset)
		} else {
			notice = fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				startLineDisplay, endLineDisplay, totalFileLines, FormatSize(opts.MaxBytes), nextOffset)
		}
		result.Content = []ai.Content{ai.TextContent{Text: truncation.Content + notice}}
	} else if userLimitedLines > 0 && startLine+userLimitedLines < len(allLines) {
		// User-specified limit stopped early
		remaining := len(allLines) - (startLine + userLimitedLines)
		nextOffset := startLine + userLimitedLines + 1
		notice = fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remaining, nextOffset)
		result.Content = []ai.Content{ai.TextContent{Text: truncation.Content + notice}}
	}

	return result, nil
}

// RunReadTool executes the read tool with the given input
func RunReadTool(ctx context.Context, input *ReadInput, cwd string) (*ReadToolResult, error) {
	opts := DefaultReadExecOptions(cwd)
	if input.Offset > 0 {
		opts.Offset = input.Offset - 1 // Convert from 1-indexed to 0-indexed
	}
	if input.Limit > 0 {
		opts.Limit = input.Limit
	}
	return ReadFile(ctx, input.Path, opts)
}
