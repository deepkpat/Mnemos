package coding_agent

import (
	"time"

	"mnemos/pkg/ai"
)

// ============================================================================
// Tool Input Types
// ============================================================================

// BashInput defines parameters for the bash tool.
type BashInput struct {
	Command string `json:"command"`
}

// ReadInput defines parameters for the read tool.
type ReadInput struct {
	Path      string `json:"path"`
	Offset    int    `json:"offset,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	ShowLines *bool  `json:"showLines,omitempty"`
}

// WriteInput defines parameters for the write tool.
type WriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// EditInput defines parameters for the edit tool.
type EditInput struct {
	Path string `json:"path"`
	Diff string `json:"diff"` // ed-style diff
}

// LsInput defines parameters for the ls tool.
type LsInput struct {
	Path   string `json:"path"`
	Limit  int    `json:"limit,omitempty"`
	All    bool   `json:"all,omitempty"`
	Long   bool   `json:"long,omitempty"`
	SortBy string `json:"sortBy,omitempty"` // "name", "size", "modified"
}

// GrepInput defines parameters for the grep tool.
type GrepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Include    string `json:"include,omitempty"`
	Exclude    string `json:"exclude,omitempty"`
	Context    int    `json:"context,omitempty"`
	IgnoreCase bool   `json:"ignoreCase,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// FindInput defines parameters for the find tool.
type FindInput struct {
	Path     string `json:"path"`
	Limit    int    `json:"limit,omitempty"`
	Types    string `json:"types,omitempty"` // "f", "d", "l"
	Name     string `json:"name,omitempty"`
	MaxDepth int    `json:"maxDepth,omitempty"`
}

// ============================================================================
// Tool Result Types
// ============================================================================

// BashResult contains the result of a bash execution.
type BashResult struct {
	Content  []ai.Content `json:"content"`
	ExitCode int          `json:"exitCode"`
}

// ReadResult contains the result of a file read.
type ReadResult struct {
	Content   []ai.Content `json:"content"`
	Truncated bool         `json:"truncated"`
	Lines     int          `json:"lines"`
	Bytes     int          `json:"bytes"`
}

// WriteResult contains the result of a file write.
type WriteResult struct {
	Content  []ai.Content `json:"content"`
	FilePath string       `json:"filePath"`
}

// EditResult contains the result of a file edit.
type EditResult struct {
	Content  []ai.Content `json:"content"`
	FilePath string       `json:"filePath"`
	Diff     string       `json:"diff"`
}

// LsResult contains the result of a directory listing.
type LsResult struct {
	Content []ai.Content `json:"content"`
	Entries []LsEntry    `json:"entries"`
}

// GrepResult contains the result of a grep search.
type GrepResult struct {
	Content []ai.Content `json:"content"`
	Matches []GrepMatch  `json:"matches"`
}

// GrepMatch represents a single grep match.
type GrepMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

// FindResult contains the result of a find operation.
type FindResult struct {
	Content []ai.Content `json:"content"`
	Paths   []string     `json:"paths"`
}

// ============================================================================
// Tool Operations Interfaces
// ============================================================================

// BashOps defines the interface for bash tool operations.
type BashOps interface {
	Run(cmd string, opts *BashOptions) (*BashResult, error)
}

// ReadOps defines the interface for read tool operations.
type ReadOps interface {
	Read(path string, opts *ReadOptions) (*ReadResult, error)
}

// WriteOps defines the interface for write tool operations.
type WriteOps interface {
	Write(path string, content string, opts *WriteOptions) (*WriteResult, error)
}

// EditOps defines the interface for edit tool operations.
type EditOps interface {
	Edit(path string, diff string, opts *EditOptions) (*EditResult, error)
}

// LsOps defines the interface for ls tool operations.
type LsOps interface {
	Ls(path string, opts *LsOptions) (*LsResult, error)
}

// GrepOps defines the interface for grep tool operations.
type GrepOps interface {
	Grep(pattern string, path string, opts *GrepOptions) (*GrepResult, error)
}

// FindOps defines the interface for find tool operations.
type FindOps interface {
	Find(path string, opts *FindOptions) (*FindResult, error)
}

// ============================================================================
// Tool Options
// ============================================================================

// BashOptions configures bash execution.
type BashOptions struct {
	Cwd      string
	Env      map[string]string
	Timeout  time.Duration
	MaxLines int
	MaxBytes int
}

// ReadOptions configures file reading.
type ReadOptions struct {
	Offset    int
	Limit     int
	ShowLines bool
}

// WriteOptions configures file writing.
type WriteOptions struct {
	CreateDirs bool
	Backup     bool
}

// EditOptions configures editing.
type EditOptions struct {
}

// LsOptions configures directory listing.
type LsOptions struct {
	All    bool
	Long   bool
	SortBy string
	Filter string
}

// GrepOptions configures grep search.
type GrepOptions struct {
	Include    string
	Exclude    string
	Context    int
	IgnoreCase bool
}

// FindOptions configures find operation.
type FindOptions struct {
	Types    string
	Name     string
	MaxDepth int
}

// ============================================================================
// Tool Factories
// ============================================================================

// DefaultBashOptions returns default options for bash.
func DefaultBashOptions() *BashOptions {
	return &BashOptions{
		MaxLines: 10000,
		MaxBytes: 1024 * 1024,
		Timeout:  60 * time.Second,
	}
}

// DefaultReadOptions returns default options for reading.
func DefaultReadOptions() *ReadOptions {
	return &ReadOptions{
		Limit:     1000,
		ShowLines: true,
	}
}

// DefaultWriteOptions returns default options for writing.
func DefaultWriteOptions() *WriteOptions {
	return &WriteOptions{
		CreateDirs: true,
	}
}

// DefaultEditOptions returns default options for editing.
func DefaultEditOptions() *EditOptions {
	return &EditOptions{}
}

// DefaultLsOptions returns default options for ls.
func DefaultLsOptions() *LsOptions {
	return &LsOptions{
		SortBy: "name",
	}
}

// DefaultGrepOptions returns default options for grep.
func DefaultGrepOptions() *GrepOptions {
	return &GrepOptions{
		Context: 3,
	}
}

// DefaultFindOptions returns default options for find.
func DefaultFindOptions() *FindOptions {
	return &FindOptions{
		MaxDepth: -1,
	}
}
