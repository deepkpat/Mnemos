package coding_agent

import (
	"fmt"

	"mnemos/pkg/ai"
)

// ============================================================================
// Token Estimation
// ============================================================================

// EstimateTokens estimates token count for a message using chars/4 heuristic.
// This is conservative (overestimates), which is fine - we'd rather compact early than too late.
func EstimateTokens(entry SessionEntry) int {
	switch e := entry.(type) {
	case *MessageEntry:
		return estimateFromMessage(e)
	case *ToolResultEntry:
		return estimateFromToolResult(e)
	default:
		return 0
	}
}

func estimateFromMessage(m *MessageEntry) int {
	switch m.Role {
	case "user", "assistant":
		chars := 0
		for _, c := range m.Content {
			switch tc := c.(type) {
			case ai.TextContent:
				chars += len(tc.Text)
			case ai.ThinkingContent:
				chars += len(tc.Thinking)
			case ai.ToolCall:
				chars += len(tc.Name) + len(fmt.Sprintf("%v", tc.Arguments))
			}
		}
		return (chars + 3) / 4
	case "toolResult":
		chars := 0
		for _, c := range m.Content {
			if tc, ok := c.(ai.TextContent); ok {
				chars += len(tc.Text)
			}
		}
		return (chars + 3) / 4
	default:
		return 0
	}
}

func estimateFromToolResult(t *ToolResultEntry) int {
	chars := 0
	for _, c := range t.Content {
		if tc, ok := c.(ai.TextContent); ok {
			chars += len(tc.Text)
		}
	}
	return (chars + 3) / 4
}

// EstimateContextTokens estimates total tokens in a slice of entries.
func EstimateContextTokens(entries []SessionEntry) int {
	total := 0
	for _, e := range entries {
		total += EstimateTokens(e)
	}
	return total
}

// ============================================================================
// Compaction Settings
// ============================================================================

// CompactionSettings controls compaction behavior.
type CompactionSettings struct {
	Enabled          bool `json:"enabled"`
	ReserveTokens    int  `json:"reserveTokens"`    // Tokens to keep as reserve
	KeepRecentTokens int  `json:"keepRecentTokens"` // Recent tokens to always keep
}

// DefaultCompactionSettings returns sensible defaults.
func DefaultCompactionSettings() CompactionSettings {
	return CompactionSettings{
		Enabled:          true,
		ReserveTokens:    16384, // 16K tokens
		KeepRecentTokens: 20000, // 20K tokens
	}
}

// ============================================================================
// Compaction Decision
// ============================================================================

// ShouldCompact returns true if compaction should be triggered.
func ShouldCompact(contextTokens, contextWindow int, settings CompactionSettings) bool {
	if !settings.Enabled {
		return false
	}
	return contextTokens > contextWindow-settings.ReserveTokens
}

// ============================================================================
// Cut Point Detection
// ============================================================================

// CutPointResult contains the result of finding a cut point.
type CutPointResult struct {
	FirstKeptIndex int
	IsSplitTurn    bool
}

// FindCutPoint walks backwards and finds where to cut the conversation.
// It finds valid cut points at message boundaries (not tool results).
func FindCutPoint(entries []SessionEntry, keepRecentTokens int) CutPointResult {
	if len(entries) == 0 {
		return CutPointResult{FirstKeptIndex: 0, IsSplitTurn: false}
	}

	// Find valid cut points: messages where role is user, assistant
	validCutPoints := []int{}
	for i, e := range entries {
		if me, ok := e.(*MessageEntry); ok {
			if me.Role == "user" || me.Role == "assistant" {
				validCutPoints = append(validCutPoints, i)
			}
		}
	}

	if len(validCutPoints) == 0 {
		return CutPointResult{FirstKeptIndex: 0, IsSplitTurn: false}
	}

	// Walk backwards from newest
	accumulated := 0
	cutIndex := 0

	for i := len(entries) - 1; i >= 0; i-- {
		accumulated += EstimateTokens(entries[i])

		if accumulated >= keepRecentTokens {
			// Find closest valid cut point at or after this entry
			for _, cp := range validCutPoints {
				if cp >= i {
					cutIndex = cp
					break
				}
			}
			break
		}
	}

	return CutPointResult{FirstKeptIndex: cutIndex, IsSplitTurn: cutIndex > 0}
}
