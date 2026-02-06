// Package updater provides self-update functionality
package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Config holds updater configuration
type Config struct {
	RepoOwner    string // GitHub owner (e.g., "kusa")
	RepoName     string // GitHub repo (e.g., "magabot")
	CurrentVersion string
	BinaryName   string
	CheckInterval time.Duration
	AutoUpdate   bool
}

// Release represents a GitHub release
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"` // Release notes
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
	HTMLURL     string    `json:"html_url"`
}

// Asset represents a release asset
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadCount      int    `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// Updater handles checking and applying updates
type Updater struct {
	config Config
	client *http.Client
}

// New creates a new updater
func New(config Config) *Updater {
	if config.BinaryName == "" {
		config.BinaryName = "magabot"
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 24 * time.Hour
	}
	
	return &Updater{
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// CheckUpdate checks for available updates
func (u *Updater) CheckUpdate(ctx context.Context) (*Release, bool, error) {
	release, err := u.getLatestRelease(ctx)
	if err != nil {
		return nil, false, err
	}
	
	// Compare versions
	hasUpdate := isNewerVersion(u.config.CurrentVersion, release.TagName)
	
	return release, hasUpdate, nil
}

// getLatestRelease fetches the latest release from GitHub
func (u *Updater) getLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest",
		u.config.RepoOwner, u.config.RepoName)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "magabot-updater")
	
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check updates: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no releases found")
	}
	
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}
	
	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}
	
	return &release, nil
}

// Update downloads and applies the update
func (u *Updater) Update(ctx context.Context, release *Release) error {
	// Find the right asset for current platform
	asset := u.findAsset(release)
	if asset == nil {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	
	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	
	// Download to temp file
	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "magabot-update")
	
	if err := u.downloadAsset(ctx, asset, tmpFile); err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	
	// If it's a tar.gz, extract it
	if strings.HasSuffix(asset.Name, ".tar.gz") || strings.HasSuffix(asset.Name, ".tgz") {
		extractedPath, err := u.extractTarGz(tmpFile, tmpDir)
		if err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to extract update: %w", err)
		}
		os.Remove(tmpFile)
		tmpFile = extractedPath
	}
	
	// Make executable
	if err := os.Chmod(tmpFile, 0755); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to chmod: %w", err)
	}
	
	// Backup current binary
	backupPath := execPath + ".backup"
	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to backup current binary: %w", err)
	}
	
	// Move new binary
	if err := os.Rename(tmpFile, execPath); err != nil {
		// Restore backup
		os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install update: %w", err)
	}
	
	// Remove backup (optional, keep for safety)
	// os.Remove(backupPath)
	
	return nil
}

// findAsset finds the appropriate asset for the current platform
func (u *Updater) findAsset(release *Release) *Asset {
	// Build expected filename patterns
	os := runtime.GOOS
	arch := runtime.GOARCH
	
	// Common naming patterns
	patterns := []string{
		fmt.Sprintf("%s_%s_%s", u.config.BinaryName, os, arch),
		fmt.Sprintf("%s-%s-%s", u.config.BinaryName, os, arch),
		fmt.Sprintf("%s_%s_%s.tar.gz", u.config.BinaryName, os, arch),
		fmt.Sprintf("%s-%s-%s.tar.gz", u.config.BinaryName, os, arch),
	}
	
	// Also check for amd64 -> x86_64 mapping
	if arch == "amd64" {
		patterns = append(patterns,
			fmt.Sprintf("%s_%s_x86_64", u.config.BinaryName, os),
			fmt.Sprintf("%s-%s-x86_64", u.config.BinaryName, os),
			fmt.Sprintf("%s_%s_x86_64.tar.gz", u.config.BinaryName, os),
		)
	}
	
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		for _, pattern := range patterns {
			if strings.Contains(name, strings.ToLower(pattern)) {
				return &asset
			}
		}
	}
	
	return nil
}

// downloadAsset downloads an asset to a file
func (u *Updater) downloadAsset(ctx context.Context, asset *Asset, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()
	
	_, err = io.Copy(out, resp.Body)
	return err
}

// extractTarGz extracts a tar.gz file and returns the path to the binary
func (u *Updater) extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gzr.Close()
	
	tr := tar.NewReader(gzr)
	
	var binaryPath string
	
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		
		// Look for the binary
		if header.Typeflag == tar.TypeReg {
			name := filepath.Base(header.Name)
			if name == u.config.BinaryName || name == u.config.BinaryName+".exe" {
				binaryPath = filepath.Join(destDir, name)
				outFile, err := os.Create(binaryPath)
				if err != nil {
					return "", err
				}
				if _, err := io.Copy(outFile, tr); err != nil {
					outFile.Close()
					return "", err
				}
				outFile.Close()
			}
		}
	}
	
	if binaryPath == "" {
		return "", fmt.Errorf("binary not found in archive")
	}
	
	return binaryPath, nil
}

// Rollback restores the previous version
func (u *Updater) Rollback() error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}
	execPath, _ = filepath.EvalSymlinks(execPath)
	
	backupPath := execPath + ".backup"
	
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found")
	}
	
	// Swap files
	tmpPath := execPath + ".tmp"
	if err := os.Rename(execPath, tmpPath); err != nil {
		return err
	}
	if err := os.Rename(backupPath, execPath); err != nil {
		os.Rename(tmpPath, execPath)
		return err
	}
	os.Remove(tmpPath)
	
	return nil
}

// isNewerVersion compares version strings (v1.2.3 format)
func isNewerVersion(current, latest string) bool {
	// Strip 'v' prefix
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	
	// Simple string comparison works for semver
	// For proper comparison, use a semver library
	return latest > current
}

// FormatReleaseInfo formats release info for display
func FormatReleaseInfo(release *Release, hasUpdate bool) string {
	if release == nil {
		return "Unable to check for updates"
	}
	
	status := "✅ You're up to date"
	if hasUpdate {
		status = "🆕 Update available!"
	}
	
	return fmt.Sprintf(`%s

📦 Latest: %s
📅 Released: %s

📝 Release Notes:
%s

🔗 %s`,
		status,
		release.TagName,
		release.PublishedAt.Format("2006-01-02"),
		truncateNotes(release.Body, 500),
		release.HTMLURL,
	)
}

func truncateNotes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
