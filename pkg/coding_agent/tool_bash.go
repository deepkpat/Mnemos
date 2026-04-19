package coding_agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"mnemos/pkg/ai"
)

// BashToolResult contains the result of a bash execution
type BashToolResult struct {
	Content        []ai.Content
	ExitCode       int
	Truncation     *TruncationResult
	FullOutputPath string
}

// BashExecOptions configures bash execution
type BashExecOptions struct {
	Cwd      string
	Env      map[string]string
	Timeout  time.Duration
	MaxLines int
	MaxBytes int
}

// DefaultBashExecOptions returns default options for bash execution
func DefaultBashExecOptions(cwd string) *BashExecOptions {
	return &BashExecOptions{
		Cwd:      cwd,
		Env:      make(map[string]string),
		Timeout:  60 * time.Second,
		MaxLines: DefaultMaxLines,
		MaxBytes: DefaultMaxBytes,
	}
}

// BashExec executes a bash command and returns the result
func BashExec(ctx context.Context, command string, opts *BashExecOptions) (*BashToolResult, error) {
	if opts == nil {
		opts = DefaultBashExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}
	if opts.MaxLines == 0 {
		opts.MaxLines = DefaultMaxLines
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultMaxBytes
	}

	// Check if directory exists
	if _, err := os.Stat(opts.Cwd); os.IsNotExist(err) {
		return nil, fmt.Errorf("working directory does not exist: %s\nCannot execute bash commands.", opts.Cwd)
	}

	// Prepare environment
	env := os.Environ()
	for k, v := range opts.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create context with timeout
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Execute command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = opts.Cwd
	cmd.Env = env
	cmd.Stdin = nil

	// Use pipe for capturing output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Read output concurrently
	var wg sync.WaitGroup
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer([]byte{}, 1024*1024) // 1MB max line
		for scanner.Scan() {
			stdoutBuf.Write(scanner.Bytes())
			stdoutBuf.WriteByte('\n')
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer([]byte{}, 1024*1024)
		for scanner.Scan() {
			stderrBuf.Write(scanner.Bytes())
			stderrBuf.WriteByte('\n')
		}
	}()

	// Wait for command to complete
	err = cmd.Wait()
	wg.Wait()

	// Combine stdout and stderr
	output := stdoutBuf.String()
	if stderrBuf.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderrBuf.String()
	}

	// Get exit code
	exitCode := 0
	timeoutOccurred := false
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			// Kill the process
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			timeoutOccurred = true
		} else if ctx.Err() == context.Canceled {
			// Command was cancelled
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			return nil, fmt.Errorf("command aborted")
		} else {
			return nil, fmt.Errorf("command failed: %w", err)
		}
	}

	// Apply tail truncation
	truncation := truncateTail(output, TruncationOptions{
		MaxLines: opts.MaxLines,
		MaxBytes: opts.MaxBytes,
	})

	result := &BashToolResult{
		Content:  []ai.Content{ai.TextContent{Text: truncation.Content}},
		ExitCode: exitCode,
	}

	if timeoutOccurred {
		// Add timeout notice to output
		timeoutMsg := fmt.Sprintf("\n\n[Command timed out after %v. Output captured so far shown above.]", opts.Timeout)
		result.Content = []ai.Content{ai.TextContent{Text: truncation.Content + timeoutMsg}}
	} else if truncation.Truncated {
		result.Truncation = &truncation
		// Add truncation notice to output
		notice := ""
		if truncation.LastLinePartial {
			lastLineSize := FormatSize(byteLen(output))
			notice = fmt.Sprintf("\n\n[Showing last %s of line %d (line is %s).]", FormatSize(truncation.OutputBytes), truncation.TotalLines, lastLineSize)
		} else if truncation.TruncatedBy == "lines" {
			notice = fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Full output truncated.]", truncation.TotalLines-truncation.OutputLines+1, truncation.TotalLines, truncation.TotalLines)
		} else {
			notice = fmt.Sprintf("\n\n[Showing lines %d-%d of %d (%s limit). Full output truncated.]", truncation.TotalLines-truncation.OutputLines+1, truncation.TotalLines, truncation.TotalLines, FormatSize(opts.MaxBytes))
		}
		result.Content = []ai.Content{ai.TextContent{Text: truncation.Content + notice}}
	}

	return result, nil
}

// RunBashTool executes the bash tool with the given input
func RunBashTool(ctx context.Context, input *BashInput, cwd string, timeout time.Duration) (*BashToolResult, error) {
	opts := DefaultBashExecOptions(cwd)
	opts.Timeout = timeout
	return BashExec(ctx, input.Command, opts)
}
