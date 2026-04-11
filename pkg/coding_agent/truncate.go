package coding_agent

import (
	"fmt"
	"strings"
)

// Default limits for tool outputs
const (
	DefaultMaxLines   = 2000
	DefaultMaxBytes   = 50 * 1024 // 50KB
	GrepMaxLineLength = 500       // Max chars per grep match line
)

// TruncationResult holds information about truncation applied to content
type TruncationResult struct {
	Content               string `json:"content"`
	Truncated             bool   `json:"truncated"`
	TruncatedBy           string `json:"truncatedBy,omitempty"` // "lines", "bytes", or null
	TotalLines            int    `json:"totalLines"`
	TotalBytes            int    `json:"totalBytes"`
	OutputLines           int    `json:"outputLines"`
	OutputBytes           int    `json:"outputBytes"`
	LastLinePartial       bool   `json:"lastLinePartial"`
	FirstLineExceedsLimit bool   `json:"firstLineExceedsLimit"`
	MaxLines              int    `json:"maxLines"`
	MaxBytes              int    `json:"maxBytes"`
}

// TruncationOptions controls truncation behavior
type TruncationOptions struct {
	MaxLines int // Maximum number of lines (default: 2000)
	MaxBytes int // Maximum number of bytes (default: 50KB)
}

// FormatSize formats bytes as human-readable size
func FormatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	} else {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

// byteLen returns the number of bytes in a string (UTF-8)
func byteLen(s string) int {
	return len([]byte(s))
}

// truncateHead truncates content from the head (keep first N lines/bytes)
// Suitable for file reads where you want to see the beginning
func truncateHead(content string, opts TruncationOptions) TruncationResult {
	maxLines := opts.MaxLines
	if maxLines == 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := byteLen(content)
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Check if no truncation needed
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:               content,
			Truncated:             false,
			TruncatedBy:           "",
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			OutputLines:           totalLines,
			OutputBytes:           totalBytes,
			LastLinePartial:       false,
			FirstLineExceedsLimit: false,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	// Check if first line alone exceeds byte limit
	firstLineBytes := byteLen(lines[0])
	if firstLineBytes > maxBytes {
		return TruncationResult{
			Content:               "",
			Truncated:             true,
			TruncatedBy:           "bytes",
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			OutputLines:           0,
			OutputBytes:           0,
			LastLinePartial:       false,
			FirstLineExceedsLimit: true,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	// Collect complete lines that fit
	outputLinesArr := []string{}
	outputBytesCount := 0
	truncatedBy := "lines"

	for i := 0; i < len(lines) && i < maxLines; i++ {
		line := lines[i]
		lineBytes := byteLen(line)
		if i > 0 {
			lineBytes++ // +1 for newline
		}

		if outputBytesCount+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}

		outputLinesArr = append(outputLinesArr, line)
		outputBytesCount += lineBytes
	}

	// If we exited due to line limit
	if len(outputLinesArr) >= maxLines && outputBytesCount <= maxBytes {
		truncatedBy = "lines"
	}

	outputContent := strings.Join(outputLinesArr, "\n")
	finalOutputBytes := byteLen(outputContent)

	return TruncationResult{
		Content:               outputContent,
		Truncated:             true,
		TruncatedBy:           truncatedBy,
		TotalLines:            totalLines,
		TotalBytes:            totalBytes,
		OutputLines:           len(outputLinesArr),
		OutputBytes:           finalOutputBytes,
		LastLinePartial:       false,
		FirstLineExceedsLimit: false,
		MaxLines:              maxLines,
		MaxBytes:              maxBytes,
	}
}

// truncateTail truncates content from the tail (keep last N lines/bytes)
// Suitable for bash output where you want to see the end (errors, final results)
func truncateTail(content string, opts TruncationOptions) TruncationResult {
	maxLines := opts.MaxLines
	if maxLines == 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := byteLen(content)
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Check if no truncation needed
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:               content,
			Truncated:             false,
			TruncatedBy:           "",
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			OutputLines:           totalLines,
			OutputBytes:           totalBytes,
			LastLinePartial:       false,
			FirstLineExceedsLimit: false,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
		}
	}

	// Work backwards from the end
	outputLinesArr := []string{}
	outputBytesCount := 0
	truncatedBy := "lines"
	lastLinePartial := false

	for i := len(lines) - 1; i >= 0 && len(outputLinesArr) < maxLines; i-- {
		line := lines[i]
		lineBytes := byteLen(line)
		if len(outputLinesArr) > 0 {
			lineBytes++ // +1 for newline
		}

		if outputBytesCount+lineBytes > maxBytes {
			truncatedBy = "bytes"
			// Edge case: if we haven't added ANY lines yet and this line exceeds maxBytes
			if len(outputLinesArr) == 0 {
				truncatedLine := truncateStringToBytesFromEnd(line, maxBytes)
				outputLinesArr = append([]string{truncatedLine}, outputLinesArr...)
				outputBytesCount = byteLen(truncatedLine)
				lastLinePartial = true
			}
			break
		}

		outputLinesArr = append([]string{line}, outputLinesArr...)
		outputBytesCount += lineBytes
	}

	// If we exited due to line limit
	if len(outputLinesArr) >= maxLines && outputBytesCount <= maxBytes {
		truncatedBy = "lines"
	}

	outputContent := strings.Join(outputLinesArr, "\n")
	finalOutputBytes := byteLen(outputContent)

	return TruncationResult{
		Content:               outputContent,
		Truncated:             true,
		TruncatedBy:           truncatedBy,
		TotalLines:            totalLines,
		TotalBytes:            totalBytes,
		OutputLines:           len(outputLinesArr),
		OutputBytes:           finalOutputBytes,
		LastLinePartial:       lastLinePartial,
		FirstLineExceedsLimit: false,
		MaxLines:              maxLines,
		MaxBytes:              maxBytes,
	}
}

// truncateStringToBytesFromEnd truncates a string to fit within a byte limit (from the end)
func truncateStringToBytesFromEnd(s string, maxBytes int) string {
	buf := []byte(s)
	if len(buf) <= maxBytes {
		return s
	}

	// Start from the end, skip maxBytes back
	start := len(buf) - maxBytes

	// Find a valid UTF-8 boundary (start of a character)
	for start < len(buf) && (buf[start]&0xc0) == 0x80 {
		start++
	}

	return string(buf[start:])
}

// TruncateLineResult represents the result of truncating a single line
type TruncateLineResult struct {
	Text         string `json:"text"`
	WasTruncated bool   `json:"wasTruncated"`
}

// truncateLine truncates a single line to max characters, adding [truncated] suffix
func truncateLine(line string, maxChars int) TruncateLineResult {
	if len(line) <= maxChars {
		return TruncateLineResult{Text: line, WasTruncated: false}
	}
	return TruncateLineResult{
		Text:         line[:maxChars] + "... [truncated]",
		WasTruncated: true,
	}
}
