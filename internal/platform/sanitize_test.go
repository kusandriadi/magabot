package platform

import (
	"testing"
)

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		input    string
		want     string
	}{
		{
			name:     "strips h3 header at start of line",
			platform: "telegram",
			input:    "### Phase 1: Setup",
			want:     "Phase 1: Setup",
		},
		{
			name:     "strips h2 header at start of line",
			platform: "telegram",
			input:    "## Overview",
			want:     "Overview",
		},
		{
			name:     "strips h1 header at start of line",
			platform: "telegram",
			input:    "# Title",
			want:     "Title",
		},
		{
			name:     "inserts blank line before inline header",
			platform: "telegram",
			input:    "Student Domain### Phase 13: Enrollment### Phase 16: Finance",
			want:     "Student Domain\n\nPhase 13: Enrollment\n\nPhase 16: Finance",
		},
		{
			name:     "converts double-asterisk bold to single for telegram",
			platform: "telegram",
			input:    "This is **important** text.",
			want:     "This is *important* text.",
		},
		{
			name:     "converts double-asterisk bold to single for slack",
			platform: "slack",
			input:    "This is **important** text.",
			want:     "This is *important* text.",
		},
		{
			name:     "converts double-asterisk bold to single for whatsapp",
			platform: "whatsapp",
			input:    "This is **important** text.",
			want:     "This is *important* text.",
		},
		{
			name:     "keeps double-asterisk bold for non-chat platforms",
			platform: "webhook",
			input:    "This is **important** text.",
			want:     "This is **important** text.",
		},
		{
			name:     "strips markdown table rows",
			platform: "telegram",
			input:    "| Phase | Status |\n| 1     | Done   |",
			want:     "Phase  Status\n1  Done",
		},
		{
			name:     "collapses excessive newlines",
			platform: "telegram",
			input:    "Line 1\n\n\n\nLine 2",
			want:     "Line 1\n\nLine 2",
		},
		{
			name:     "trims leading and trailing whitespace",
			platform: "telegram",
			input:    "\n\nHello\n\n",
			want:     "Hello",
		},
		{
			name:     "leaves clean text untouched",
			platform: "telegram",
			input:    "Analisis selesai.\n\nAda 17 gap ditemukan.",
			want:     "Analisis selesai.\n\nAda 17 gap ditemukan.",
		},
		{
			name:     "adds newline between run-together sentences",
			platform: "telegram",
			input:    "Fixed bug.Now add feature:Deploy and push.",
			want:     "Fixed bug.\n\nNow add feature:\n\nDeploy and push.",
		},
		{
			name:     "handles mixed issues",
			platform: "telegram",
			input:    "## Summary\nAll gaps found.### Next Steps\n**Update** the schema.",
			want:     "Summary\nAll gaps found.\n\nNext Steps\n*Update* the schema.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeText(tc.platform, tc.input)
			if got != tc.want {
				t.Errorf("SanitizeText(%q, %q)\n got:  %q\n want: %q", tc.platform, tc.input, got, tc.want)
			}
		})
	}
}
