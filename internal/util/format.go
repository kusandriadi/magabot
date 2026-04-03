package util

import (
	"fmt"
	"strings"
	"time"
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

// FormatDuration formats a duration in a human-readable way (e.g. "2h 15m", "3d 12h").
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// FormatServerStatsCLI returns server resource stats formatted for terminal output.
func FormatServerStatsCLI(s ServerStats) string {
	var sb strings.Builder
	sb.WriteString("\n   Server:\n")
	sb.WriteString(fmt.Sprintf("     CPU:    load %.2f / %.2f / %.2f (1/5/15m)\n", s.LoadAvg1, s.LoadAvg5, s.LoadAvg15))
	if s.MemTotal > 0 {
		memPct := float64(s.MemUsed) / float64(s.MemTotal) * 100
		sb.WriteString(fmt.Sprintf("     Memory: %s / %s (%.0f%%)\n", FormatBytes(s.MemUsed), FormatBytes(s.MemTotal), memPct))
	}
	if s.DiskTotal > 0 {
		diskPct := float64(s.DiskUsed) / float64(s.DiskTotal) * 100
		sb.WriteString(fmt.Sprintf("     Disk:   %s / %s (%.0f%%)\n", FormatBytes(s.DiskUsed), FormatBytes(s.DiskTotal), diskPct))
	}
	if s.HasGPU {
		gpuMemPct := float64(s.GPUMemUsed) / float64(s.GPUMemTotal) * 100
		sb.WriteString(fmt.Sprintf("     GPU:    %s — %s / %s (%.0f%% mem, %d%% util)\n",
			s.GPUName, FormatBytes(s.GPUMemUsed), FormatBytes(s.GPUMemTotal), gpuMemPct, s.GPUUtil))
	}
	return sb.String()
}

// FormatServerStatsChat returns server resource stats formatted for chat platforms.
func FormatServerStatsChat(s ServerStats) string {
	var sb strings.Builder
	sb.WriteString("\n💻 Server:\n")
	sb.WriteString(fmt.Sprintf("  • CPU: load %.2f / %.2f / %.2f (1/5/15m)\n", s.LoadAvg1, s.LoadAvg5, s.LoadAvg15))
	if s.MemTotal > 0 {
		memPct := float64(s.MemUsed) / float64(s.MemTotal) * 100
		sb.WriteString(fmt.Sprintf("  • Memory: %s / %s (%.0f%%)\n", FormatBytes(s.MemUsed), FormatBytes(s.MemTotal), memPct))
	}
	if s.DiskTotal > 0 {
		diskPct := float64(s.DiskUsed) / float64(s.DiskTotal) * 100
		sb.WriteString(fmt.Sprintf("  • Disk: %s / %s (%.0f%%)\n", FormatBytes(s.DiskUsed), FormatBytes(s.DiskTotal), diskPct))
	}
	if s.HasGPU {
		gpuMemPct := float64(s.GPUMemUsed) / float64(s.GPUMemTotal) * 100
		sb.WriteString(fmt.Sprintf("  • GPU: %s — %s / %s (%.0f%% mem, %d%% util)\n",
			s.GPUName, FormatBytes(s.GPUMemUsed), FormatBytes(s.GPUMemTotal), gpuMemPct, s.GPUUtil))
	}
	return sb.String()
}
