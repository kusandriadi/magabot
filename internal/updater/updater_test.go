package updater

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.1", "1.0.0", false},
		{"dev", "1.0.0", true},
		{"1.0.0", "dev", false},
		{"dev", "dev", false},
		{"v1.0.0", "v1.1.0", true},
		{"v1.1.0", "v1.0.0", false},
		{"1.0.0-beta", "1.0.0", false},
		{"1.0.0", "1.0.0-beta", false},
		{"0.9.0", "1.0.0", true},
		{"1.0.0", "0.9.0", false},
		{"1.0.0", "2.0.0", true},
		{"2.0.0", "1.0.0", false},
		{"unknown", "1.0.0", true},
		{"1.0.0", "unknown", false},
		{"unknown", "unknown", false},
		{"unknown", "dev", false},
		{"dev", "unknown", false},
		{"v1.2.3", "v1.2.4", true},
		{"v1.2.4", "v1.2.3", false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_vs_%s", tt.current, tt.latest)
		t.Run(name, func(t *testing.T) {
			got := isNewerVersion(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v",
					tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"1.0.0", [3]int{1, 0, 0}},
		{"1.2.3", [3]int{1, 2, 3}},
		{"0.0.0", [3]int{0, 0, 0}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"1.0.0-beta", [3]int{1, 0, 0}},
		{"1.2.3-rc1", [3]int{1, 2, 3}},
		{"1.2.3+build", [3]int{1, 2, 3}},
		{"1", [3]int{1, 0, 0}},
		{"1.2", [3]int{1, 2, 0}},
		{"", [3]int{0, 0, 0}},
		{"abc", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseVersion(tt.input)
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindAsset(t *testing.T) {
	u := New(Config{BinaryName: "magabot"})

	expectedName := fmt.Sprintf("magabot_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	// Build assets list ensuring the expected name maps to "current" URL,
	// with a decoy asset for a different platform.
	decoyOS := "linux"
	if runtime.GOOS == "linux" {
		decoyOS = "darwin"
	}
	decoyName := fmt.Sprintf("magabot_%s_%s.tar.gz", decoyOS, runtime.GOARCH)

	release := &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: decoyName, Size: 1000, BrowserDownloadURL: "https://example.com/decoy"},
			{Name: expectedName, Size: 2000, BrowserDownloadURL: "https://example.com/current"},
			{Name: "checksums.txt", Size: 100, BrowserDownloadURL: "https://example.com/checksums"},
		},
	}

	asset := u.findAsset(release)
	if asset == nil {
		t.Fatal("expected to find matching asset")
	}
	if asset.BrowserDownloadURL != "https://example.com/current" {
		t.Errorf("expected download URL for current platform, got %q", asset.BrowserDownloadURL)
	}
}

func TestFindAsset_NotFound(t *testing.T) {
	u := New(Config{BinaryName: "magabot"})

	release := &Release{
		TagName: "v1.0.0",
		Assets: []Asset{
			{Name: "magabot_plan9_mips.tar.gz", Size: 1000},
			{Name: "checksums.txt", Size: 100},
		},
	}

	asset := u.findAsset(release)
	if asset != nil {
		t.Errorf("expected nil for platform with no matching asset, got %+v", asset)
	}
}

func TestFindAsset_EmptyRelease(t *testing.T) {
	u := New(Config{BinaryName: "magabot"})

	release := &Release{
		TagName: "v1.0.0",
		Assets:  []Asset{},
	}

	asset := u.findAsset(release)
	if asset != nil {
		t.Errorf("expected nil for release with no assets, got %+v", asset)
	}
}

func TestFindChecksumAsset(t *testing.T) {
	u := New(Config{BinaryName: "magabot"})

	tests := []struct {
		name       string
		assetNames []string
		wantFound  bool
		wantName   string
	}{
		{
			name:       "checksums.txt",
			assetNames: []string{"magabot_linux_amd64.tar.gz", "checksums.txt"},
			wantFound:  true,
			wantName:   "checksums.txt",
		},
		{
			name:       "SHA256SUMS",
			assetNames: []string{"magabot_linux_amd64.tar.gz", "SHA256SUMS"},
			wantFound:  true,
			wantName:   "SHA256SUMS",
		},
		{
			name:       "no_checksum_file",
			assetNames: []string{"magabot_linux_amd64.tar.gz", "README.md"},
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := make([]Asset, len(tt.assetNames))
			for i, n := range tt.assetNames {
				assets[i] = Asset{Name: n}
			}
			release := &Release{Assets: assets}

			got := u.findChecksumAsset(release)
			if tt.wantFound {
				if got == nil {
					t.Fatal("expected to find checksum asset")
				}
				if got.Name != tt.wantName {
					t.Errorf("expected name %q, got %q", tt.wantName, got.Name)
				}
			} else {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
			}
		})
	}
}

func TestFormatReleaseInfo(t *testing.T) {
	release := &Release{
		TagName:     "v1.2.3",
		Name:        "Release 1.2.3",
		Body:        "Bug fixes and improvements",
		PublishedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		HTMLURL:     "https://github.com/kusa/magabot/releases/tag/v1.2.3",
	}

	output := FormatReleaseInfo(release, false)

	if !strings.Contains(output, "v1.2.3") {
		t.Errorf("output should contain tag name, got:\n%s", output)
	}
	if !strings.Contains(output, "2025-06-15") {
		t.Errorf("output should contain formatted date, got:\n%s", output)
	}
	if !strings.Contains(output, "Bug fixes and improvements") {
		t.Errorf("output should contain release notes, got:\n%s", output)
	}
	if !strings.Contains(output, release.HTMLURL) {
		t.Errorf("output should contain HTML URL, got:\n%s", output)
	}
	if !strings.Contains(output, "up to date") {
		t.Errorf("output should indicate up to date when hasUpdate=false, got:\n%s", output)
	}
}

func TestFormatReleaseInfo_NilRelease(t *testing.T) {
	output := FormatReleaseInfo(nil, false)
	if !strings.Contains(output, "Unable to check") {
		t.Errorf("nil release should return 'Unable to check' message, got: %q", output)
	}
}

func TestFormatReleaseInfo_HasUpdate(t *testing.T) {
	release := &Release{
		TagName:     "v2.0.0",
		Name:        "Major Release",
		Body:        "New features",
		PublishedAt: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		HTMLURL:     "https://github.com/kusa/magabot/releases/tag/v2.0.0",
	}

	output := FormatReleaseInfo(release, true)

	if !strings.Contains(output, "Update available") {
		t.Errorf("output should contain 'Update available' when hasUpdate=true, got:\n%s", output)
	}
}

func TestFormatReleaseInfo_LongNotes(t *testing.T) {
	longBody := strings.Repeat("A", 600)
	release := &Release{
		TagName:     "v1.0.0",
		Body:        longBody,
		PublishedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		HTMLURL:     "https://example.com",
	}

	output := FormatReleaseInfo(release, false)

	// The truncateNotes function caps at 500 chars + "..."
	if strings.Contains(output, longBody) {
		t.Error("long notes should be truncated")
	}
	if !strings.Contains(output, "...") {
		t.Error("truncated notes should end with '...'")
	}
}

func TestNew_Defaults(t *testing.T) {
	u := New(Config{})

	if u.config.BinaryName != "magabot" {
		t.Errorf("expected default BinaryName 'magabot', got %q", u.config.BinaryName)
	}
	if u.config.CheckInterval != 24*time.Hour {
		t.Errorf("expected default CheckInterval 24h, got %v", u.config.CheckInterval)
	}
}

func TestNew_CustomConfig(t *testing.T) {
	u := New(Config{
		BinaryName:    "mybot",
		CheckInterval: 1 * time.Hour,
		RepoOwner:     "owner",
		RepoName:      "repo",
	})

	if u.config.BinaryName != "mybot" {
		t.Errorf("expected BinaryName 'mybot', got %q", u.config.BinaryName)
	}
	if u.config.CheckInterval != 1*time.Hour {
		t.Errorf("expected CheckInterval 1h, got %v", u.config.CheckInterval)
	}
	if u.config.RepoOwner != "owner" {
		t.Errorf("expected RepoOwner 'owner', got %q", u.config.RepoOwner)
	}
}

func TestTruncateNotes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "hello world", 5, "hello..."},
		{"empty", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateNotes(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncateNotes(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}
