package coding_agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"mnemos/pkg/ai"
)

// FindToolResult contains the result of a find operation
type FindToolResult struct {
	Content      []ai.Content
	Paths        []string
	TotalResults int
	Truncated    bool
	ResultLimit  int
}

// FindExecOptions configures find operation
type FindExecOptions struct {
	Cwd   string
	Path  string
	Limit int
}

// DefaultFindExecOptions returns default options for find
func DefaultFindExecOptions(cwd string) *FindExecOptions {
	return &FindExecOptions{
		Cwd:   cwd,
		Limit: 1000,
	}
}

// FindFiles searches for files matching a glob pattern and returns the result
func FindFiles(ctx context.Context, pattern string, searchPath string, opts *FindExecOptions) (*FindToolResult, error) {
	if opts == nil {
		opts = DefaultFindExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}
	if opts.Limit == 0 {
		opts.Limit = 1000
	}
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	if searchPath == "" {
		searchPath = "."
	}

	// Resolve search path
	absolutePath := searchPath
	if !filepath.IsAbs(searchPath) {
		absolutePath = filepath.Join(opts.Cwd, searchPath)
	}

	// Check if path exists
	if _, err := os.Stat(absolutePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("path not found: %s", searchPath)
	}

	// Try using fd (fast alternative to find)
	fdPath, err := exec.LookPath("fd")
	if err == nil {
		// Use fd
		args := []string{
			"--glob", pattern,
			"--color=never",
			"--hidden",
			"--max-results", fmt.Sprintf("%d", opts.Limit),
			"-t", "f", // only files
		}
		args = append(args, absolutePath)

		cmd := exec.CommandContext(ctx, fdPath, args...)
		output, err := cmd.Output()
		if err != nil {
			// Fall through to fallback
		} else {
			return parseFindOutput(string(output), absolutePath, opts.Limit)
		}
	}

	// Fallback: use filepath.Glob
	searchPattern := pattern
	if !strings.Contains(pattern, "**") {
		// Convert simple glob to recursive
		searchPattern = "**/" + pattern
	}

	matches, err := filepath.Glob(filepath.Join(absolutePath, searchPattern))
	if err != nil {
		return nil, fmt.Errorf("glob failed: %w", err)
	}

	// Sort results
	sort.Strings(matches)

	// Limit results
	if len(matches) > opts.Limit {
		matches = matches[:opts.Limit]
	}

	// Format output (relative paths)
	var results []string
	for _, match := range matches {
		relPath, err := filepath.Rel(absolutePath, match)
		if err != nil {
			continue
		}
		// Convert to POSIX path
		results = append(results, strings.ReplaceAll(relPath, "\\", "/"))
	}

	output := strings.Join(results, "\n")
	result := &FindToolResult{
		Content:      []ai.Content{ai.TextContent{Text: output}},
		Paths:        results,
		TotalResults: len(matches),
	}

	if len(matches) >= opts.Limit {
		result.Truncated = true
		result.ResultLimit = opts.Limit
		notice := fmt.Sprintf("\n\n[%d results limit reached. Use limit=%d for more.]", opts.Limit, opts.Limit*2)
		result.Content = []ai.Content{ai.TextContent{Text: output + notice}}
	}

	if len(results) == 0 {
		result.Content = []ai.Content{ai.TextContent{Text: "No files found matching pattern"}}
	}

	return result, nil
}

// parseFindOutput parses fd output into results
func parseFindOutput(output string, searchPath string, limit int) (*FindToolResult, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return &FindToolResult{
			Content:      []ai.Content{ai.TextContent{Text: "No files found matching pattern"}},
			Paths:        []string{},
			TotalResults: 0,
		}, nil
	}

	var results []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(results) >= limit {
			break
		}

		// Convert to relative path
		relPath, err := filepath.Rel(searchPath, line)
		if err != nil {
			continue
		}
		// Convert to POSIX path
		results = append(results, strings.ReplaceAll(relPath, "\\", "/"))
	}

	outputStr := strings.Join(results, "\n")
	result := &FindToolResult{
		Content:      []ai.Content{ai.TextContent{Text: outputStr}},
		Paths:        results,
		TotalResults: len(lines),
	}

	if len(results) >= limit {
		result.Truncated = true
		result.ResultLimit = limit
		notice := fmt.Sprintf("\n\n[%d results limit reached. Use limit=%d for more.]", limit, limit*2)
		result.Content = []ai.Content{ai.TextContent{Text: outputStr + notice}}
	}

	return result, nil
}

// RunFindTool executes the find tool with the given input
func RunFindTool(ctx context.Context, input *FindInput, cwd string) (*FindToolResult, error) {
	opts := DefaultFindExecOptions(cwd)
	if input.Limit > 0 {
		opts.Limit = input.Limit
	}
	return FindFiles(ctx, input.Name, input.Path, opts)
}
