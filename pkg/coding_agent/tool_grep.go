package coding_agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mnemos/pkg/ai"
)

// GrepMatch represents a single grep match
type GrepMatchResult struct {
	Path   string
	Line   int
	Column int
	Text   string
}

// GrepToolResult contains the result of a grep search
type GrepToolResult struct {
	Content        []ai.Content
	Matches        []GrepMatchResult
	TotalMatches   int
	Truncated      bool
	MatchLimit     int
	LinesTruncated bool
}

// GrepExecOptions configures grep search
type GrepExecOptions struct {
	Cwd        string
	Pattern    string
	Path       string
	Glob       string
	IgnoreCase bool
	Literal    bool
	Context    int
	Limit      int
}

// DefaultGrepExecOptions returns default options for grep
func DefaultGrepExecOptions(cwd string) *GrepExecOptions {
	return &GrepExecOptions{
		Cwd:     cwd,
		Limit:   100,
		Context: 0,
	}
}

// GrepSearch searches for a pattern in files and returns the result
func GrepSearch(ctx context.Context, pattern string, searchPath string, opts *GrepExecOptions) (*GrepToolResult, error) {
	if opts == nil {
		opts = DefaultGrepExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}
	if opts.Limit == 0 {
		opts.Limit = 100
	}
	if opts.Pattern == "" {
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

	// Build ripgrep command
	args := []string{"--line-number", "--color=never", "--hidden"}
	if opts.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if opts.Literal {
		args = append(args, "--fixed-strings")
	}
	if opts.Glob != "" {
		args = append(args, "--glob", opts.Glob)
	}
	if opts.Context > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", opts.Context))
	}
	args = append(args, pattern, absolutePath)

	// Execute ripgrep
	cmd := exec.CommandContext(ctx, "rg", args...)
	output, err := cmd.Output()
	if err != nil {
		// Check if it's "no matches" or actual error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No matches found
			return &GrepToolResult{
				Content:      []ai.Content{ai.TextContent{Text: "No matches found"}},
				Matches:      []GrepMatchResult{},
				TotalMatches: 0,
			}, nil
		}
		return nil, fmt.Errorf("ripgrep failed: %w", err)
	}

	// Parse output
	result, err := parseGrepOutput(string(output), opts.Context, opts.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to parse grep output: %w", err)
	}

	return result, nil
}

// parseGrepOutput parses ripgrep output into matches
func parseGrepOutput(output string, context int, limit int) (*GrepToolResult, error) {
	var matches []GrepMatchResult
	var linesTruncated bool
	scanner := bufio.NewScanner(strings.NewReader(output))
	matchCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		if matchCount >= limit {
			break
		}

		// Parse ripgrep output format: file:line:column:content
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 2 {
			continue
		}

		path := parts[0]
		var lineNum, colNum int
		var text string
		if len(parts) >= 3 {
			_, err := fmt.Sscanf(parts[1], "%d", &lineNum)
			if err != nil {
				continue
			}
			_, err = fmt.Sscanf(parts[2], "%d", &colNum)
			if err != nil {
				text = parts[2]
			} else if len(parts) > 3 {
				text = parts[3]
			}
		} else {
			text = parts[1]
		}

		// Truncate long lines
		if len(text) > GrepMaxLineLength {
			text = text[:GrepMaxLineLength] + "... [truncated]"
			linesTruncated = true
		}

		matches = append(matches, GrepMatchResult{
			Path:   path,
			Line:   lineNum,
			Column: colNum,
			Text:   text,
		})
		matchCount++
	}

	// Format output
	var outputText strings.Builder
	for _, m := range matches {
		outputText.WriteString(fmt.Sprintf("%s:%d: %s\n", m.Path, m.Line, m.Text))
	}

	result := &GrepToolResult{
		Content:        []ai.Content{ai.TextContent{Text: strings.TrimSpace(outputText.String())}},
		Matches:        matches,
		TotalMatches:   len(matches),
		LinesTruncated: linesTruncated,
	}

	if len(matches) >= limit {
		result.Truncated = true
		result.MatchLimit = limit
		notice := fmt.Sprintf("\n\n[%d matches limit reached. Use limit=%d for more.]", limit, limit*2)
		result.Content = []ai.Content{ai.TextContent{Text: strings.TrimSpace(outputText.String()) + notice}}
	}

	return result, nil
}

// RunGrepTool executes the grep tool with the given input
func RunGrepTool(ctx context.Context, input *GrepInput, cwd string) (*GrepToolResult, error) {
	opts := DefaultGrepExecOptions(cwd)
	opts.Pattern = input.Pattern
	if input.Include != "" {
		opts.Glob = input.Include
	}
	opts.IgnoreCase = input.IgnoreCase
	opts.Context = input.Context
	if input.Limit > 0 {
		opts.Limit = input.Limit
	}
	return GrepSearch(ctx, input.Pattern, input.Path, opts)
}
