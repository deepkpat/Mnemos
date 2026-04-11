package coding_agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ============================================================================
// Session Manager with Tree Structure and JSONL Persistence
// ============================================================================

// SessionManager manages a coding agent session with tree structure.
type SessionManager struct {
	mu      sync.RWMutex
	header  SessionHeader
	file    *os.File
	writer  *bufio.Writer
	entries []SessionEntry
	byID    map[string]SessionEntry
	leafID  string
}

// NewSessionManager creates a new session in the given directory.
func NewSessionManager(dir string, opts *SessionOptions) (*SessionManager, error) {
	if opts == nil {
		opts = DefaultSessionOptions()
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("session: failed to create directory: %w", err)
	}

	now := time.Now()
	header := SessionHeader{
		ID:      generateSessionID(),
		Name:    opts.Name,
		Cwd:     opts.Cwd,
		Created: now.UnixMilli(),
	}

	// Create file: timestamp_id.jsonl
	filename := fmt.Sprintf("%d_%s.jsonl", now.UnixMilli(), header.ID)
	filePath := filepath.Join(dir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("session: failed to create file: %w", err)
	}

	writer := bufio.NewWriter(file)

	// Write header as first line (as JSON)
	if err := json.NewEncoder(writer).Encode(map[string]interface{}{
		"type":    "session",
		"version": 3,
		"id":      header.ID,
		"cwd":     header.Cwd,
		"name":    header.Name,
		"created": header.Created,
	}); err != nil {
		file.Close()
		return nil, fmt.Errorf("session: failed to write header: %w", err)
	}

	sm := &SessionManager{
		header:  header,
		file:    file,
		writer:  writer,
		entries: []SessionEntry{},
		byID:    make(map[string]SessionEntry),
		leafID:  "",
	}

	return sm, nil
}

// OpenSessionManager opens an existing session file.
func OpenSessionManager(filePath string) (*SessionManager, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("session: failed to open: %w", err)
	}

	scanner := bufio.NewScanner(file)
	entries := []SessionEntry{}
	byID := make(map[string]SessionEntry)
	var header SessionHeader
	var leafID string

	for scanner.Scan() {
		var raw map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
			continue
		}

		entryType, _ := raw["entryType"].(string)
		if entryType == "" {
			entryType, _ = raw["type"].(string)
		}

		var entry SessionEntry
		switch entryType {
		case "session":
			header.ID, _ = raw["id"].(string)
			header.Cwd, _ = raw["cwd"].(string)
			header.Name, _ = raw["name"].(string)
			v, _ := raw["created"].(float64)
			header.Created = int64(v)
			continue
		case "message":
			entry = parseMessageEntry(raw)
		case "tool_result":
			entry = parseToolResultEntry(raw)
		case "model_change":
			entry = parseModelChangeEntry(raw)
		case "thinking_level_change":
			entry = parseThinkingLevelChangeEntry(raw)
		case "compaction":
			entry = parseCompactionEntry(raw)
		case "branch_summary":
			entry = parseBranchSummaryEntry(raw)
		}

		if entry != nil {
			entries = append(entries, entry)
			byID[entry.(*MessageEntry).EntryType_] = entry // Using EntryType as ID for now
			leafID = entry.(*MessageEntry).EntryType_      // Placeholder
		}
	}

	file.Close()

	// Re-open for appending
	appendFile, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("session: failed to reopen: %w", err)
	}

	sm := &SessionManager{
		header:  header,
		file:    appendFile,
		writer:  bufio.NewWriter(appendFile),
		entries: entries,
		byID:    byID,
		leafID:  leafID,
	}

	return sm, nil
}

// Close closes the session.
func (sm *SessionManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.writer != nil {
		sm.writer.Flush()
	}
	if sm.file != nil {
		sm.file.Close()
	}
	sm.writer = nil
	sm.file = nil
	return nil
}

// ============================================================================
// Entry Access
// ============================================================================

// GetID returns the session ID.
func (sm *SessionManager) GetID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.header.ID
}

// GetLeafID returns the current leaf entry ID.
func (sm *SessionManager) GetLeafID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.leafID
}

// GetEntries returns all entries.
func (sm *SessionManager) GetEntries() []SessionEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	result := make([]SessionEntry, len(sm.entries))
	copy(result, sm.entries)
	return result
}

// ============================================================================
// Branch Operations
// ============================================================================

// GetBranch returns all entries from current leaf to root.
func (sm *SessionManager) GetBranch() []SessionEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	path := []SessionEntry{}
	currentID := sm.leafID

	for currentID != "" {
		if entry, ok := sm.byID[currentID]; ok {
			path = append(path, entry)
			// Get parent - need to store parentID in entries
			break // Simplified for now
		}
		break
	}

	// Reverse to root-to-leaf
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// Branch moves the leaf to an earlier entry.
func (sm *SessionManager) Branch(branchFromID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, ok := sm.byID[branchFromID]; !ok {
		return fmt.Errorf("session: entry not found: %s", branchFromID)
	}

	sm.leafID = branchFromID
	return nil
}

// ============================================================================
// Entry Append
// ============================================================================

// appendEntry appends and persists an entry.
func (sm *SessionManager) appendEntry(entry SessionEntry) error {
	sm.entries = append(sm.entries, entry)

	// Store in byID map - need proper ID storage
	id := fmt.Sprintf("%d", len(sm.entries)-1)
	sm.byID[id] = entry
	sm.leafID = id

	if sm.writer != nil {
		if err := json.NewEncoder(sm.writer).Encode(entry); err != nil {
			return fmt.Errorf("session: failed to write: %w", err)
		}
	}

	return nil
}

// AppendMessage appends a message.
func (sm *SessionManager) AppendMessage(role string, content []ContentBlock) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := &MessageEntry{
		EntryType_: "message",
		Role:       role,
		Content:    content,
		Timestamp:  time.Now().UnixMilli(),
	}

	return sm.appendEntry(entry)
}

// AppendToolResult appends a tool result.
func (sm *SessionManager) AppendToolResult(toolCallID, toolName string, content []ContentBlock, isError bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := &ToolResultEntry{
		EntryType_: "tool_result",
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Content:    content,
		IsError:    isError,
		Timestamp:  time.Now().UnixMilli(),
	}

	return sm.appendEntry(entry)
}

// AppendCompaction appends a compaction entry.
func (sm *SessionManager) AppendCompaction(summary, firstKeptEntryID string, tokensBefore int, readFiles, modifiedFiles []string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := &CompactionEntry{
		EntryType_:       "compaction",
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
		ReadFiles:        readFiles,
		ModifiedFiles:    modifiedFiles,
		Timestamp:        time.Now().UnixMilli(),
	}

	return sm.appendEntry(entry)
}

// ============================================================================
// Parse Helpers
// ============================================================================

func parseMessageEntry(raw map[string]interface{}) *MessageEntry {
	entry := &MessageEntry{}
	if v, ok := raw["entryType"].(string); ok {
		entry.EntryType_ = v
	}
	if v, ok := raw["role"].(string); ok {
		entry.Role = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = int64(v)
	}
	return entry
}

func parseToolResultEntry(raw map[string]interface{}) *ToolResultEntry {
	entry := &ToolResultEntry{}
	if v, ok := raw["entryType"].(string); ok {
		entry.EntryType_ = v
	}
	if v, ok := raw["toolCallId"].(string); ok {
		entry.ToolCallID = v
	}
	if v, ok := raw["toolName"].(string); ok {
		entry.ToolName = v
	}
	if v, ok := raw["isError"].(bool); ok {
		entry.IsError = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = int64(v)
	}
	return entry
}

func parseModelChangeEntry(raw map[string]interface{}) *ModelChangeEntry {
	entry := &ModelChangeEntry{}
	if v, ok := raw["entryType"].(string); ok {
		entry.EntryType_ = v
	}
	if v, ok := raw["provider"].(string); ok {
		entry.Provider = v
	}
	if v, ok := raw["model"].(string); ok {
		entry.Model = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = int64(v)
	}
	return entry
}

func parseThinkingLevelChangeEntry(raw map[string]interface{}) *ThinkingLevelChangeEntry {
	entry := &ThinkingLevelChangeEntry{}
	if v, ok := raw["entryType"].(string); ok {
		entry.EntryType_ = v
	}
	if v, ok := raw["level"].(string); ok {
		entry.Level = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = int64(v)
	}
	return entry
}

func parseCompactionEntry(raw map[string]interface{}) *CompactionEntry {
	entry := &CompactionEntry{}
	if v, ok := raw["entryType"].(string); ok {
		entry.EntryType_ = v
	}
	if v, ok := raw["summary"].(string); ok {
		entry.Summary = v
	}
	if v, ok := raw["firstKeptEntryId"].(string); ok {
		entry.FirstKeptEntryID = v
	}
	if v, ok := raw["tokensBefore"].(float64); ok {
		entry.TokensBefore = int(v)
	}
	if v, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = int64(v)
	}
	return entry
}

func parseBranchSummaryEntry(raw map[string]interface{}) *BranchSummaryEntry {
	entry := &BranchSummaryEntry{}
	if v, ok := raw["entryType"].(string); ok {
		entry.EntryType_ = v
	}
	if v, ok := raw["summary"].(string); ok {
		entry.Summary = v
	}
	if v, ok := raw["fromId"].(string); ok {
		entry.FromID = v
	}
	if v, ok := raw["timestamp"].(float64); ok {
		entry.Timestamp = int64(v)
	}
	return entry
}
