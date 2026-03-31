package platform

import (
	"regexp"
	"strings"
)

var (
	// reInlineHeader matches markdown headers that appear mid-text without a preceding newline.
	reInlineHeader = regexp.MustCompile(`([^\n])(#{1,6} )`)
	// reMarkdownHeader matches markdown headers (e.g. "### Title") at start of a line.
	reMarkdownHeader = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	// reDoubleAsterisk matches **bold** markdown (not valid on chat platforms).
	reDoubleAsterisk = regexp.MustCompile(`\*\*(.+?)\*\*`)
	// reTableLine matches lines that look like markdown table rows (|col|col|).
	reTableLine = regexp.MustCompile(`(?m)^\|.+\|$`)
	// reExcessiveNewlines collapses 3+ consecutive newlines into 2.
	reExcessiveNewlines = regexp.MustCompile(`\n{3,}`)
)

// SanitizeText cleans up common LLM formatting issues for the given platform.
// It strips markdown constructs that render poorly or are unsupported in chat UIs.
//
// Applied to all platforms:
//   - Markdown headers (### Title) are stripped and given a preceding blank line.
//   - Table pipe rows are converted to space-separated text.
//   - 3+ consecutive newlines are collapsed to 2.
//
// Applied to chat platforms (telegram, slack, whatsapp):
//   - **bold** is converted to *bold* (single-asterisk, the format these platforms use).
func SanitizeText(platform, text string) string {
	// Insert blank line before any header that immediately follows other text.
	text = reInlineHeader.ReplaceAllString(text, "$1\n\n")
	// Strip the leading # characters from headers at the start of a line.
	text = reMarkdownHeader.ReplaceAllString(text, "")
	// Remove table rows — replace with cell content joined by spaces.
	text = reTableLine.ReplaceAllStringFunc(text, func(line string) string {
		line = strings.Trim(line, "|")
		parts := strings.Split(line, "|")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
		}
		return strings.Join(parts, "  ")
	})
	// Collapse 3+ blank lines into a single blank line.
	text = reExcessiveNewlines.ReplaceAllString(text, "\n\n")
	// Chat platforms use single-asterisk bold (*text*), not double-asterisk (**text**).
	switch platform {
	case "telegram", "slack", "whatsapp":
		text = reDoubleAsterisk.ReplaceAllString(text, "*$1*")
	}
	return strings.TrimSpace(text)
}
