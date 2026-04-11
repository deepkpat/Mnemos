package coding_agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var (
	ErrNoSession     = errors.New("coding_agent: no active session")
	ErrSessionClosed = errors.New("coding_agent: session already closed")
)

// ============================================================================
// Session Types
// ============================================================================

// SessionHeader contains session metadata.
type SessionHeader struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	File     string `json:"file,omitempty"`
	Cwd      string `json:"cwd"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Created  int64  `json:"created"`
	Modified int64  `json:"modified"`
}

// SessionEntry is the interface for session entries.
type SessionEntry interface {
	EntryType() string
}

// MessageEntry represents a user or assistant message.
type MessageEntry struct {
	EntryType_ string         `json:"entryType"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Timestamp  int64          `json:"timestamp"`
}

// EntryType returns the entry type.
func (e *MessageEntry) EntryType() string { return e.EntryType_ }

// ContentBlock is either text or image content.
type ContentBlock struct {
	Type string `json:"type"` // "text" or "image"
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"` // base64 for images
}

// ToolResultEntry represents a tool execution result.
type ToolResultEntry struct {
	EntryType_ string         `json:"entryType"`
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Content    []ContentBlock `json:"content"`
	IsError    bool           `json:"isError"`
	Timestamp  int64          `json:"timestamp"`
}

// EntryType returns the entry type.
func (e *ToolResultEntry) EntryType() string { return e.EntryType_ }

// ModelChangeEntry records a model change.
type ModelChangeEntry struct {
	EntryType_ string `json:"entryType"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Timestamp  int64  `json:"timestamp"`
}

// EntryType returns the entry type.
func (e *ModelChangeEntry) EntryType() string { return e.EntryType_ }

// ThinkingLevelChangeEntry records a thinking level change.
type ThinkingLevelChangeEntry struct {
	EntryType_ string `json:"entryType"`
	Level      string `json:"level"`
	Timestamp  int64  `json:"timestamp"`
}

// EntryType returns the entry type.
func (e *ThinkingLevelChangeEntry) EntryType() string { return e.EntryType_ }

// ============================================================================
// Session
// ============================================================================

// Session manages a coding agent session.
type Session struct {
	header  SessionHeader
	dir     string
	entries []SessionEntry
}

// NewSession creates a new session in the given directory.
func NewSession(dir string, opts *SessionOptions) (*Session, error) {
	if opts == nil {
		opts = DefaultSessionOptions()
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("coding_agent: failed to create session dir: %w", err)
	}

	header := SessionHeader{
		ID:       generateSessionID(),
		Name:     opts.Name,
		Cwd:      opts.Cwd,
		Provider: opts.Provider,
		Model:    opts.Model,
		Created:  time.Now().UnixMilli(),
		Modified: time.Now().UnixMilli(),
	}

	s := &Session{
		header:  header,
		dir:     dir,
		entries: []SessionEntry{},
	}

	// Set session file path
	header.File = filepath.Join(dir, fmt.Sprintf("session_%s.json", header.ID))
	s.header = header

	return s, nil
}

// LoadSession loads an existing session from a file.
func LoadSession(file string) (*Session, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("coding_agent: failed to read session: %w", err)
	}

	var header SessionHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("coding_agent: failed to parse session: %w", err)
	}

	entries, err := parseEntries(data)
	if err != nil {
		return nil, err
	}

	return &Session{
		header:  header,
		dir:     filepath.Dir(file),
		entries: entries,
	}, nil
}

// Save saves the session to disk.
func (s *Session) Save() error {
	data, err := json.MarshalIndent(s.header, "", "  ")
	if err != nil {
		return fmt.Errorf("coding_agent: failed to marshal header: %w", err)
	}

	// Append entries
	entriesData, err := json.Marshal(s.entries)
	if err != nil {
		return fmt.Errorf("coding_agent: failed to marshal entries: %w", err)
	}

	data = append(data, '\n')
	data = append(data, entriesData...)

	if err := os.WriteFile(s.header.File, data, 0644); err != nil {
		return fmt.Errorf("coding_agent: failed to write session: %w", err)
	}

	s.header.Modified = time.Now().UnixMilli()
	return nil
}

// Header returns the session header.
func (s *Session) Header() SessionHeader {
	return s.header
}

// Entries returns all session entries.
func (s *Session) Entries() []SessionEntry {
	return s.entries
}

// AppendMessage appends a message entry.
func (s *Session) AppendMessage(role string, content []ContentBlock) {
	s.entries = append(s.entries, &MessageEntry{
		EntryType_: "message",
		Role:       role,
		Content:    content,
		Timestamp:  time.Now().UnixMilli(),
	})
}

// AppendToolResult appends a tool result entry.
func (s *Session) AppendToolResult(toolCallID, toolName string, content []ContentBlock, isError bool) {
	s.entries = append(s.entries, &ToolResultEntry{
		EntryType_: "tool_result",
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Content:    content,
		IsError:    isError,
		Timestamp:  time.Now().UnixMilli(),
	})
}

// AppendModelChange appends a model change entry.
func (s *Session) AppendModelChange(provider, model string) {
	s.entries = append(s.entries, &ModelChangeEntry{
		EntryType_: "model_change",
		Provider:   provider,
		Model:      model,
		Timestamp:  time.Now().UnixMilli(),
	})
}

// AppendThinkingLevelChange appends a thinking level change entry.
func (s *Session) AppendThinkingLevelChange(level string) {
	s.entries = append(s.entries, &ThinkingLevelChangeEntry{
		EntryType_: "thinking_level_change",
		Level:      level,
		Timestamp:  time.Now().UnixMilli(),
	})
}

// ============================================================================
// Session Options
// ============================================================================

// SessionOptions configures a new session.
type SessionOptions struct {
	Name     string
	Cwd      string
	Provider string
	Model    string
}

// DefaultSessionOptions returns default session options.
func DefaultSessionOptions() *SessionOptions {
	cwd, _ := os.Getwd()
	return &SessionOptions{
		Cwd: cwd,
	}
}

// generateSessionID generates a unique session ID.
func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixMilli())
}

// parseEntries parses session entries from JSON data.
func parseEntries(data []byte) ([]SessionEntry, error) {
	var entries []SessionEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("coding_agent: failed to parse entries: %w", err)
	}
	return entries, nil
}
