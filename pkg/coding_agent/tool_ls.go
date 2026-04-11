package coding_agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mnemos/pkg/ai"
)

// LsEntry represents a single directory entry
type LsEntry struct {
	Name    string
	Type    string // "file", "dir", "link"
	Size    int64
	ModTime int64
}

// LsToolResult contains the result of a directory listing
type LsToolResult struct {
	Content      []ai.Content
	Entries      []LsEntry
	TotalEntries int
	Truncated    bool
	EntryLimit   int
}

// LsExecOptions configures directory listing
type LsExecOptions struct {
	Cwd   string
	Limit int
}

// DefaultLsExecOptions returns default options for directory listing
func DefaultLsExecOptions(cwd string) *LsExecOptions {
	return &LsExecOptions{
		Cwd:   cwd,
		Limit: 500,
	}
}

// ListDirectory lists directory contents and returns the result
func ListDirectory(ctx context.Context, path string, opts *LsExecOptions) (*LsToolResult, error) {
	if opts == nil {
		opts = DefaultLsExecOptions("")
	}
	if opts.Cwd == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get current directory: %w", err)
		}
		opts.Cwd = cwd
	}
	if opts.Limit == 0 {
		opts.Limit = 500
	}

	// Resolve path
	dirPath := path
	if dirPath == "" {
		dirPath = "."
	}
	absolutePath := dirPath
	if !filepath.IsAbs(dirPath) {
		absolutePath = filepath.Join(opts.Cwd, dirPath)
	}

	// Check if path exists
	info, err := os.Stat(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path not found: %s", dirPath)
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	// If it's a file, return just that file
	if !info.IsDir() {
		entry := LsEntry{
			Name:    info.Name(),
			Type:    "file",
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		}
		return &LsToolResult{
			Content:      []ai.Content{ai.TextContent{Text: info.Name()}},
			Entries:      []LsEntry{entry},
			TotalEntries: 1,
		}, nil
	}

	// Read directory entries
	entries, err := os.ReadDir(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory: %w", err)
	}

	// Sort alphabetically (case-insensitive)
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	// Build results
	var results []LsEntry
	entryLimitReached := false
	for _, entry := range entries {
		if len(results) >= opts.Limit {
			entryLimitReached = true
			break
		}

		info, err := entry.Info()
		if err != nil {
			// Skip entries we can't stat
			continue
		}

		entryType := "file"
		if info.IsDir() {
			entryType = "dir"
		} else if entry.Type()&os.ModeSymlink != 0 {
			entryType = "link"
		}

		results = append(results, LsEntry{
			Name:    entry.Name(),
			Type:    entryType,
			Size:    info.Size(),
			ModTime: info.ModTime().Unix(),
		})
	}

	// Format output
	var output strings.Builder
	for _, e := range results {
		if e.Type == "dir" {
			output.WriteString(e.Name + "/\n")
		} else {
			output.WriteString(e.Name + "\n")
		}
	}

	result := &LsToolResult{
		Content:      []ai.Content{ai.TextContent{Text: strings.TrimSpace(output.String())}},
		Entries:      results,
		TotalEntries: len(entries),
	}

	if entryLimitReached {
		result.Truncated = true
		result.EntryLimit = opts.Limit
		notice := fmt.Sprintf("\n\n[%d entries limit reached. Use limit=%d for more.]", opts.Limit, opts.Limit*2)
		result.Content = []ai.Content{ai.TextContent{Text: strings.TrimSpace(output.String()) + notice}}
	}

	return result, nil
}

// RunLsTool executes the ls tool with the given input
func RunLsTool(ctx context.Context, input *LsInput, cwd string) (*LsToolResult, error) {
	opts := DefaultLsExecOptions(cwd)
	if input.Limit > 0 {
		opts.Limit = input.Limit
	}
	return ListDirectory(ctx, input.Path, opts)
}
