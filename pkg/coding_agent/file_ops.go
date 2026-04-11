package coding_agent

import (
	"strings"
	"time"

	"mnemos/pkg/ai"
)

// nowMillis returns current time in milliseconds.
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// ============================================================================
// File Operation Tracking
// ============================================================================

// FileOperations tracks read/write/edit operations.
type FileOperations struct {
	Read    map[string]bool
	Written map[string]bool
	Edited  map[string]bool
}

// NewFileOperations creates a new file operations tracker.
func NewFileOperations() *FileOperations {
	return &FileOperations{
		Read:    make(map[string]bool),
		Written: make(map[string]bool),
		Edited:  make(map[string]bool),
	}
}

// ExtractFileOps extracts file operations from entries.
func ExtractFileOps(entries []SessionEntry) *FileOperations {
	ops := NewFileOperations()

	for _, e := range entries {
		me, ok := e.(*MessageEntry)
		if !ok || me.Role != "assistant" {
			continue
		}

		for _, c := range me.Content {
			tc, ok := c.(ai.ToolCall)
			if !ok {
				continue
			}

			path, _ := tc.Arguments["path"].(string)
			if path == "" {
				continue
			}

			switch tc.Name {
			case "Read":
				ops.Read[path] = true
			case "Write":
				ops.Written[path] = true
			case "Edit":
				ops.Edited[path] = true
			}
		}
	}

	return ops
}

// GetReadFiles returns files that were read but not modified.
func (f *FileOperations) GetReadFiles() []string {
	modified := make(map[string]bool)
	for k := range f.Edited {
		modified[k] = true
	}
	for k := range f.Written {
		modified[k] = true
	}

	result := []string{}
	for k := range f.Read {
		if !modified[k] {
			result = append(result, k)
		}
	}
	return result
}

// GetModifiedFiles returns files that were written or edited.
func (f *FileOperations) GetModifiedFiles() []string {
	result := []string{}
	for k := range f.Written {
		result = append(result, k)
	}
	for k := range f.Edited {
		result = append(result, k)
	}
	return result
}

// FormatFileOperations formats file operations as XML for the summary.
func (f *FileOperations) FormatFileOperations() string {
	var sections []string

	readFiles := f.GetReadFiles()
	if len(readFiles) > 0 {
		sections = append(sections, "<read-files>\n"+strings.Join(readFiles, "\n")+"\n</read-files>")
	}

	modifiedFiles := f.GetModifiedFiles()
	if len(modifiedFiles) > 0 {
		sections = append(sections, "<modified-files>\n"+strings.Join(modifiedFiles, "\n")+"\n</modified-files>")
	}

	if len(sections) == 0 {
		return ""
	}

	return "\n\n" + strings.Join(sections, "\n\n")
}

// ============================================================================
// Compaction Entry Types
// ============================================================================

// CompactionEntry stores a summary of compacted history.
type CompactionEntry struct {
	EntryType_       string   `json:"entryType"`
	ID               string   `json:"id"`
	ParentID         string   `json:"parentId"`
	Timestamp        int64    `json:"timestamp"`
	Summary          string   `json:"summary"`
	FirstKeptEntryID string   `json:"firstKeptEntryId"`
	TokensBefore     int      `json:"tokensBefore"`
	ReadFiles        []string `json:"readFiles,omitempty"`
	ModifiedFiles    []string `json:"modifiedFiles,omitempty"`
	FromHook         bool     `json:"fromHook,omitempty"`
}

// EntryType returns the entry type.
func (e *CompactionEntry) EntryType() string { return e.EntryType_ }

// NewCompactionEntry creates a new compaction entry.
func NewCompactionEntry(parentID, summary, firstKeptEntryID string, tokensBefore int, fileOps *FileOperations) *CompactionEntry {
	readFiles := []string{}
	if fileOps != nil {
		readFiles = fileOps.GetReadFiles()
	}

	modifiedFiles := []string{}
	if fileOps != nil {
		modifiedFiles = fileOps.GetModifiedFiles()
	}

	return &CompactionEntry{
		EntryType_:       "compaction",
		ID:               generateID(),
		ParentID:         parentID,
		Timestamp:        nowMillis(),
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
		ReadFiles:        readFiles,
		ModifiedFiles:    modifiedFiles,
	}
}

// BranchSummaryEntry stores a summary when branching.
type BranchSummaryEntry struct {
	EntryType_ string `json:"entryType"`
	ID         string `json:"id"`
	ParentID   string `json:"parentId"`
	Summary    string `json:"summary"`
	FromID     string `json:"fromId"`
	Timestamp  int64  `json:"timestamp"`
}

// EntryType returns the entry type.
func (e *BranchSummaryEntry) EntryType() string { return e.EntryType_ }

// NewBranchSummaryEntry creates a new branch summary entry.
func NewBranchSummaryEntry(parentID, summary, fromID string) *BranchSummaryEntry {
	return &BranchSummaryEntry{
		EntryType_: "branch_summary",
		ID:         generateID(),
		ParentID:   parentID,
		Summary:    summary,
		FromID:     fromID,
		Timestamp:  nowMillis(),
	}
}

// ============================================================================
// Summary Generation Prompts
// ============================================================================

// SummarizationPrompt is the prompt used for summarization.
const SummarizationPrompt = `The messages above are a conversation to summarize. Create a structured context checkpoint summary that another LLM will use to continue the work.

Use this EXACT format:

## Goal
[What is the user trying to accomplish? Can be multiple items if the session covers different tasks.]

## Constraints & Preferences
- [Any constraints, preferences, or requirements mentioned by user]
- [Or "(none)" if none were mentioned]

## Progress
### Done
- [x] [Completed tasks/changes]

### In Progress
- [ ] [Current work]

### Blocked
- [Issues preventing progress, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [Ordered list of what should happen next]

## Critical Context
- [Any data, examples, or references needed to continue]
- [Or "(none)" if not applicable]

Keep each section concise. Preserve exact file paths, function names, and error messages.`

// UpdateSummarizationPrompt is used when updating an existing summary.
const UpdateSummarizationPrompt = `The messages above are NEW conversation messages to incorporate into the existing summary provided in <previous-summary> tags.

Update the existing structured summary with new information. RULES:
- PRESERVE all existing information from the previous summary
- ADD new progress, decisions, and context from the new messages
- UPDATE the Progress section: move items from "In Progress" to "Done" when completed
- UPDATE "Next Steps" based on what was accomplished
- PRESERVE exact file paths, function names, and error messages
- If something is no longer relevant, you may remove it

Use this EXACT format:

## Goal
[Preserve existing goals, add new ones if the task expanded]

## Constraints & Preferences
- [Preserve existing, add new ones discovered]

## Progress
### Done
- [x] [Include previously done items AND newly completed items]

### In Progress
- [ ] [Current work - update based on progress]

### Blocked
- [Current blockers - remove if resolved]

## Key Decisions
- **[Decision]**: [Brief rationale] (preserve all previous, add new)

## Next Steps
1. [Update based on current state]

## Critical Context
- [Preserve important context, add new if needed]

Keep each section concise. Preserve exact file paths, function names, and error messages.`
