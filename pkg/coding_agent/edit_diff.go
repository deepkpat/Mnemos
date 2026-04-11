package coding_agent

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// detectLineEnding detects the line ending used in content
func detectLineEnding(content string) string {
	crlfIdx := strings.Index(content, "\r\n")
	lfIdx := strings.Index(content, "\n")
	if lfIdx == -1 {
		return "\n"
	}
	if crlfIdx == -1 {
		return "\n"
	}
	if crlfIdx < lfIdx {
		return "\r\n"
	}
	return "\n"
}

// normalizeToLF normalizes all line endings to LF
func normalizeToLF(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// restoreLineEndings converts LF to the original line ending
func restoreLineEndings(text string, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

// normalizeForFuzzyMatch normalizes text for fuzzy matching
func normalizeForFuzzyMatch(text string) string {
	// Normalize unicode and strip trailing whitespace
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// Strip trailing whitespace
		lines[i] = strings.TrimRightFunc(line, unicode.IsSpace)
	}
	text = strings.Join(lines, "\n")

	// Normalize smart quotes to ASCII
	text = strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", "\u201A", "'", "\u201B", "'",
		"\u201C", "\"", "\u201D", "\"", "\u201E", "\"", "\u201F", "\"",
	).Replace(text)

	// Normalize various dashes/hyphens to ASCII hyphen
	text = strings.NewReplacer(
		"\u2010", "-", "\u2011", "-", "\u2012", "-",
		"\u2013", "-", "\u2014", "-", "\u2015", "-", "\u2212", "-",
	).Replace(text)

	// Normalize special Unicode spaces to regular space
	text = strings.NewReplacer(
		"\u00A0", " ", // NBSP
		"\u2002", " ", "\u2003", " ", "\u2004", " ", "\u2005", " ",
		"\u2006", " ", "\u2007", " ", "\u2008", " ", "\u2009", " ", "\u200A", " ",
		"\u202F", " ", "\u205F", " ", "\u3000", " ",
	).Replace(text)

	return text
}

// FuzzyMatchResult holds information about a fuzzy match
type FuzzyMatchResult struct {
	Found                 bool
	Index                 int
	MatchLength           int
	UsedFuzzyMatch        bool
	ContentForReplacement string
}

// Edit represents a single edit operation
type Edit struct {
	OldText string
	NewText string
}

// fuzzyFindText finds oldText in content, trying exact match first, then fuzzy match
func fuzzyFindText(content string, oldText string) FuzzyMatchResult {
	// Try exact match first
	exactIndex := strings.Index(content, oldText)
	if exactIndex != -1 {
		return FuzzyMatchResult{
			Found:                 true,
			Index:                 exactIndex,
			MatchLength:           len(oldText),
			UsedFuzzyMatch:        false,
			ContentForReplacement: content,
		}
	}

	// Try fuzzy match - work entirely in normalized space
	fuzzyContent := normalizeForFuzzyMatch(content)
	fuzzyOldText := normalizeForFuzzyMatch(oldText)
	fuzzyIndex := strings.Index(fuzzyContent, fuzzyOldText)

	if fuzzyIndex == -1 {
		return FuzzyMatchResult{
			Found:                 false,
			Index:                 -1,
			MatchLength:           0,
			UsedFuzzyMatch:        false,
			ContentForReplacement: content,
		}
	}

	return FuzzyMatchResult{
		Found:                 true,
		Index:                 fuzzyIndex,
		MatchLength:           len(fuzzyOldText),
		UsedFuzzyMatch:        true,
		ContentForReplacement: fuzzyContent,
	}
}

// stripBom strips UTF-8 BOM if present
func stripBom(content string) (bom string, text string) {
	if strings.HasPrefix(content, "\uFEFF") {
		return "\uFEFF", content[1:]
	}
	return "", content
}

func countOccurrences(content string, oldText string) int {
	fuzzyContent := normalizeForFuzzyMatch(content)
	fuzzyOldText := normalizeForFuzzyMatch(oldText)
	return strings.Count(fuzzyContent, fuzzyOldText)
}

// ApplyEditsResult holds the result of applying edits
type ApplyEditsResult struct {
	BaseContent string
	NewContent  string
}

// applyEditsToNormalizedContent applies one or more exact-text replacements to LF-normalized content
func applyEditsToNormalizedContent(normalizedContent string, edits []Edit, path string) (ApplyEditsResult, error) {
	// Normalize edits
	normalizedEdits := make([]Edit, len(edits))
	for i, edit := range edits {
		normalizedEdits[i] = Edit{
			OldText: normalizeToLF(edit.OldText),
			NewText: normalizeToLF(edit.NewText),
		}
	}

	// Check for empty oldText
	for i, edit := range normalizedEdits {
		if len(edit.OldText) == 0 {
			if len(normalizedEdits) == 1 {
				return ApplyEditsResult{}, fmt.Errorf("oldText must not be empty in %s", path)
			}
			return ApplyEditsResult{}, fmt.Errorf("edits[%d].oldText must not be empty in %s", i, path)
		}
	}

	// Find matches
	matches := make([]FuzzyMatchResult, len(normalizedEdits))
	for i, edit := range normalizedEdits {
		matches[i] = fuzzyFindText(normalizedContent, edit.OldText)
	}

	// Determine base content
	baseContent := normalizedContent
	anyFuzzy := false
	for _, m := range matches {
		if m.UsedFuzzyMatch {
			anyFuzzy = true
			break
		}
	}
	if anyFuzzy {
		baseContent = normalizeForFuzzyMatch(normalizedContent)
	}

	// Re-match in the base content
	matchedEdits := make([]struct {
		editIndex   int
		matchIndex  int
		matchLength int
		newText     string
	}, len(normalizedEdits))

	for i, edit := range normalizedEdits {
		matchResult := fuzzyFindText(baseContent, edit.OldText)
		if !matchResult.Found {
			if len(normalizedEdits) == 1 {
				return ApplyEditsResult{}, fmt.Errorf("Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", path)
			}
			return ApplyEditsResult{}, fmt.Errorf("Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.", i, path)
		}

		occurrences := countOccurrences(baseContent, edit.OldText)
		if occurrences > 1 {
			if len(normalizedEdits) == 1 {
				return ApplyEditsResult{}, fmt.Errorf("Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.", occurrences, path)
			}
			return ApplyEditsResult{}, fmt.Errorf("Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.", occurrences, i, path)
		}

		matchedEdits[i] = struct {
			editIndex   int
			matchIndex  int
			matchLength int
			newText     string
		}{
			editIndex:   i,
			matchIndex:  matchResult.Index,
			matchLength: matchResult.MatchLength,
			newText:     edit.NewText,
		}
	}

	// Sort by match index and check for overlaps
	for i := 1; i < len(matchedEdits); i++ {
		matchedEdits[i], matchedEdits[i-1] = matchedEdits[i-1], matchedEdits[i]
	}
	// Actually sort properly
	for i := 0; i < len(matchedEdits)-1; i++ {
		for j := i + 1; j < len(matchedEdits); j++ {
			if matchedEdits[j].matchIndex < matchedEdits[i].matchIndex {
				matchedEdits[i], matchedEdits[j] = matchedEdits[j], matchedEdits[i]
			}
		}
	}

	// Check for overlaps
	for i := 1; i < len(matchedEdits); i++ {
		prev := matchedEdits[i-1]
		curr := matchedEdits[i]
		if prev.matchIndex+prev.matchLength > curr.matchIndex {
			return ApplyEditsResult{}, fmt.Errorf("edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.", prev.editIndex, curr.editIndex, path)
		}
	}

	// Apply edits in reverse order
	newContent := baseContent
	for i := len(matchedEdits) - 1; i >= 0; i-- {
		edit := matchedEdits[i]
		newContent = newContent[:edit.matchIndex] + edit.newText + newContent[edit.matchIndex+edit.matchLength:]
	}

	// Check if no changes were made
	if baseContent == newContent {
		if len(normalizedEdits) == 1 {
			return ApplyEditsResult{}, fmt.Errorf("No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.", path)
		}
		return ApplyEditsResult{}, fmt.Errorf("No changes made to %s. The replacements produced identical content.", path)
	}

	return ApplyEditsResult{
		BaseContent: baseContent,
		NewContent:  newContent,
	}, nil
}

// DiffLine represents a single line in a diff
type DiffLine struct {
	Prefix  string // "+", "-", " "
	LineNum int    // Line number in old or new file
	Content string
}

// DiffResult holds the result of computing a diff
type DiffResult struct {
	Diff             string
	FirstChangedLine int
}

// generateDiffString generates a unified diff string with line numbers
func generateDiffString(oldContent string, newContent string, contextLines int) DiffResult {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	// Compute LCS-based diff using simple line-by-line comparison
	diff := computeDiff(oldLines, newLines, contextLines)

	// Format the diff output
	var output strings.Builder
	maxLineNum := len(oldLines)
	if len(newLines) > maxLineNum {
		maxLineNum = len(newLines)
	}
	lineNumWidth := len(fmt.Sprintf("%d", maxLineNum))

	oldLineNum := 1
	newLineNum := 1

	for _, line := range diff {
		if line.Prefix == "+" {
			lineNumStr := fmt.Sprintf("%s%d", strings.Repeat(" ", lineNumWidth-len(fmt.Sprintf("%d", newLineNum))), newLineNum)
			output.WriteString(fmt.Sprintf("+%s %s\n", lineNumStr, line.Content))
			newLineNum++
		} else if line.Prefix == "-" {
			lineNumStr := fmt.Sprintf("%s%d", strings.Repeat(" ", lineNumWidth-len(fmt.Sprintf("%d", oldLineNum))), oldLineNum)
			output.WriteString(fmt.Sprintf("-%s %s\n", lineNumStr, line.Content))
			oldLineNum++
		} else {
			lineNumStr := fmt.Sprintf("%s%d", strings.Repeat(" ", lineNumWidth-len(fmt.Sprintf("%d", oldLineNum))), oldLineNum)
			output.WriteString(fmt.Sprintf(" %s %s\n", lineNumStr, line.Content))
			oldLineNum++
			newLineNum++
		}
	}

	return DiffResult{
		Diff:             output.String(),
		FirstChangedLine: findFirstChangedLine(diff),
	}
}

// computeDiff computes a simple line-by-line diff
func computeDiff(oldLines, newLines []string, contextLines int) []DiffLine {
	// Use a simple longest common subsequence approach
	lcs := longestCommonSubsequence(oldLines, newLines)

	var diff []DiffLine
	oldIdx := 0
	newIdx := 0
	lcsIdx := 0

	for oldIdx < len(oldLines) || newIdx < len(newLines) {
		if lcsIdx < len(lcs) {
			// Add context before the next change
			for oldIdx < len(oldLines) && oldLines[oldIdx] != lcs[lcsIdx] {
				diff = append(diff, DiffLine{Prefix: "-", LineNum: oldIdx + 1, Content: oldLines[oldIdx]})
				oldIdx++
			}
			for newIdx < len(newLines) && newLines[newIdx] != lcs[lcsIdx] {
				diff = append(diff, DiffLine{Prefix: "+", LineNum: newIdx + 1, Content: newLines[newIdx]})
				newIdx++
			}
			// Add the matching line
			if oldIdx < len(oldLines) && newIdx < len(newLines) {
				diff = append(diff, DiffLine{Prefix: " ", LineNum: oldIdx + 1, Content: oldLines[oldIdx]})
				oldIdx++
				newIdx++
				lcsIdx++
			}
		} else {
			// Add remaining lines
			for oldIdx < len(oldLines) {
				diff = append(diff, DiffLine{Prefix: "-", LineNum: oldIdx + 1, Content: oldLines[oldIdx]})
				oldIdx++
			}
			for newIdx < len(newLines) {
				diff = append(diff, DiffLine{Prefix: "+", LineNum: newIdx + 1, Content: newLines[newIdx]})
				newIdx++
			}
		}
	}

	// Simplify by removing unnecessary context lines
	return simplifyDiff(diff, contextLines)
}

// longestCommonSubsequence finds the longest common subsequence of two string slices
func longestCommonSubsequence(a, b []string) []string {
	m, n := len(a), len(b)
	// Use dynamic programming to find LCS
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	// Backtrack to find LCS
	var lcs []string
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			lcs = append([]string{a[i-1]}, lcs...)
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return lcs
}

// simplifyDiff removes redundant context lines from the diff
func simplifyDiff(diff []DiffLine, contextLines int) []DiffLine {
	if len(diff) == 0 {
		return diff
	}

	var result []DiffLine
	lastChange := -1

	for i, line := range diff {
		if line.Prefix != " " {
			lastChange = i
		}
	}

	// Keep changes and some context around them
	for i, line := range diff {
		if line.Prefix != " " {
			// Include context before
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			// Include context after
			end := i + contextLines
			if end >= len(diff) {
				end = len(diff) - 1
			}

			// Add any missing context
			for len(result) < start {
				result = append(result, diff[len(result)])
			}
			result = append(result, line)
		} else if i <= lastChange+contextLines && i >= lastChange-contextLines {
			// Keep context around changes
			result = append(result, line)
		}
	}

	return result
}

// findFirstChangedLine finds the first changed line number in the diff
func findFirstChangedLine(diff []DiffLine) int {
	for _, line := range diff {
		if line.Prefix != " " {
			return line.LineNum
		}
	}
	return 0
}

// EditDiffResult holds the result of computing an edit diff
type EditDiffResult struct {
	Diff             string
	FirstChangedLine int
}

// EditDiffError holds an error from computing an edit diff
type EditDiffError struct {
	Error string
}

// ComputeEditsDiff computes the diff for one or more edit operations without applying them
func ComputeEditsDiff(path string, edits []Edit, cwd string) (EditDiffResult, error) {
	absolutePath := resolveToCwd(path, cwd)

	// Check if file exists and is readable
	file, err := os.Open(absolutePath)
	if err != nil {
		return EditDiffResult{}, fmt.Errorf("File not found: %s", path)
	}
	defer file.Close()

	// Read the file
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	rawContent := strings.Join(lines, "\n")

	// Strip BOM before matching
	_, text := stripBom(rawContent)
	normalizedContent := normalizeToLF(text)

	// Apply edits
	result, err := applyEditsToNormalizedContent(normalizedContent, edits, path)
	if err != nil {
		return EditDiffResult{}, err
	}

	// Generate the diff
	diffResult := generateDiffString(result.BaseContent, result.NewContent, 4)
	return EditDiffResult{
		Diff:             diffResult.Diff,
		FirstChangedLine: diffResult.FirstChangedLine,
	}, nil
}

// ComputeEditDiff computes the diff for a single edit operation
func ComputeEditDiff(path string, oldText string, newText string, cwd string) (EditDiffResult, error) {
	return ComputeEditsDiff(path, []Edit{{OldText: oldText, NewText: newText}}, cwd)
}

// ReadFileForDiff reads a file and returns its content as a string (for diff computation)
func ReadFileForDiff(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFileForEdit writes content to a file
func WriteFileForEdit(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// FileExistsForEdit checks if a file exists
func FileExistsForEdit(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDirForEdit checks if a path is a directory
func IsDirForEdit(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ReadFileBytesForEdit reads a file and returns its content as bytes
func ReadFileBytesForEdit(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// WriteFileBytesForEdit writes bytes to a file
func WriteFileBytesForEdit(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// DirectoryExistsForEdit checks if a directory exists
func DirectoryExistsForEdit(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ReadDirForEdit reads a directory and returns entries
func ReadDirForEdit(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries, err := file.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// StatForEdit returns file info for a path
func StatForEdit(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// StringToBytesForEdit converts a string to bytes
func StringToBytesForEdit(s string) []byte {
	return []byte(s)
}

// WriteFileWithDirForEdit creates directory if needed and writes file
func WriteFileWithDirForEdit(path string, content string) error {
	// Get directory part
	dir := path
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash > 0 {
		dir = path[:lastSlash]
		if dir != "" && !DirectoryExistsForEdit(dir) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
		}
	}
	return WriteFileForEdit(path, content)
}

// ReadFileWithBytesForEdit reads a file as bytes
func ReadFileWithBytesForEdit(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// diffLines computes line-by-line diff between two strings
func diffLines(oldStr, newStr string) []byte {
	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	var buf bytes.Buffer
	lcs := longestCommonSubsequence(oldLines, newLines)

	oldIdx := 0
	newIdx := 0

	for _, line := range lcs {
		for oldIdx < len(oldLines) && oldLines[oldIdx] != line {
			buf.WriteString("- ")
			buf.WriteString(oldLines[oldIdx])
			buf.WriteString("\n")
			oldIdx++
		}
		for newIdx < len(newLines) && newLines[newIdx] != line {
			buf.WriteString("+ ")
			buf.WriteString(newLines[newIdx])
			buf.WriteString("\n")
			newIdx++
		}
		// Common line
		buf.WriteString("  ")
		buf.WriteString(line)
		buf.WriteString("\n")
		oldIdx++
		newIdx++
	}

	// Remaining lines
	for oldIdx < len(oldLines) {
		buf.WriteString("- ")
		buf.WriteString(oldLines[oldIdx])
		buf.WriteString("\n")
		oldIdx++
	}
	for newIdx < len(newLines) {
		buf.WriteString("+ ")
		buf.WriteString(newLines[newIdx])
		buf.WriteString("\n")
		newIdx++
	}

	return buf.Bytes()
}
