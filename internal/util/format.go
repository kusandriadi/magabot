package util

import (
	"fmt"
	"strings"
)

// BoolIcon returns ✅ for true and ❌ for false.
func BoolIcon(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

// SearchResult holds a single search result for formatting.
type SearchResult struct {
	Title       string
	URL         string
	Description string
}

// FormatSearchResults formats numbered search results with a header.
func FormatSearchResults(header string, results []SearchResult, descLimit int) string {
	var sb strings.Builder
	sb.WriteString(header)
	sb.WriteString("\n\n")

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("**%d. %s**\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   🔗 %s\n", r.URL))
		if r.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", Truncate(r.Description, descLimit)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// WriteTruncatedFooter appends "... and N more <noun>" to sb when total > shown.
func WriteTruncatedFooter(sb *strings.Builder, total, shown int, noun string) {
	if total > shown {
		sb.WriteString(fmt.Sprintf("\n... and %d more %s", total-shown, noun))
	}
}

// FormatBytes returns a human-readable byte size string (e.g. "4.2 GB").
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
