package telegram

import (
	"testing"
)

func TestSanitizeTelegramText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips h3 header at start of line",
			input: "### Phase 1: Setup",
			want:  "Phase 1: Setup",
		},
		{
			name:  "strips h2 header at start of line",
			input: "## Overview",
			want:  "Overview",
		},
		{
			name:  "strips h1 header at start of line",
			input: "# Title",
			want:  "Title",
		},
		{
			name:  "inserts blank line before inline header",
			input: "Student Domain### Phase 13: Enrollment### Phase 16: Finance",
			want:  "Student Domain\n\nPhase 13: Enrollment\n\nPhase 16: Finance",
		},
		{
			name:  "converts double-asterisk bold to single-asterisk",
			input: "This is **important** text.",
			want:  "This is *important* text.",
		},
		{
			name:  "strips markdown table rows",
			input: "| Phase | Status |\n| 1     | Done   |",
			want:  "Phase  Status\n1  Done",
		},
		{
			name:  "collapses excessive newlines",
			input: "Line 1\n\n\n\nLine 2",
			want:  "Line 1\n\nLine 2",
		},
		{
			name:  "trims leading and trailing whitespace",
			input: "\n\nHello\n\n",
			want:  "Hello",
		},
		{
			name:  "leaves clean text untouched",
			input: "Analisis selesai.\n\nAda 17 gap ditemukan.",
			want:  "Analisis selesai.\n\nAda 17 gap ditemukan.",
		},
		{
			name:  "handles mixed issues",
			input: "## Summary\nAll gaps found.### Next Steps\n**Update** the schema.",
			want:  "Summary\nAll gaps found.\n\nNext Steps\n*Update* the schema.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeTelegramText(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeTelegramText(%q)\n got:  %q\n want: %q", tc.input, got, tc.want)
			}
		})
	}
}
