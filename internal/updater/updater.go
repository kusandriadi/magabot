// Package updater provides self-update functionality
package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/util"
)

// Config holds updater configuration
type Config struct {
	RepoOwner    string // GitHub owner (e.g., "kusandriadi")
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
		client: util.NewHTTPClient(0),
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&release); err != nil {
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

	// Look for a checksums file in the release assets
	checksumAsset := u.findChecksumAsset(release)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Use a unique temporary directory to prevent TOCTOU race
	tmpDir, err := os.MkdirTemp("", "magabot-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "download")

	if err := u.downloadAsset(ctx, asset, tmpFile); err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	// Verify checksum if checksums file is available
	if checksumAsset != nil {
		if err := u.verifyChecksum(ctx, checksumAsset, asset.Name, tmpFile); err != nil {
			return fmt.Errorf("checksum verification failed: %w", err)
		}
	}

	// If it's a tar.gz, extract it
	if strings.HasSuffix(asset.Name, ".tar.gz") || strings.HasSuffix(asset.Name, ".tgz") {
		extractedPath, err := u.extractTarGz(tmpFile, tmpDir)
		if err != nil {
			return fmt.Errorf("failed to extract update: %w", err)
		}
		tmpFile = extractedPath
	}
	
	// Make executable
	if err := os.Chmod(tmpFile, 0755); err != nil { // #nosec G302 -- binary must be executable
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
		_ = os.Rename(backupPath, execPath)
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
	
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	// Limit download to 200MB to prevent disk exhaustion
	_, err = io.Copy(out, io.LimitReader(resp.Body, 200*1024*1024))
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

		// Only extract regular files
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// Reject path traversal attempts
		if strings.Contains(header.Name, "..") {
			return "", fmt.Errorf("invalid tar entry: %s", header.Name)
		}

		// Only extract the target binary
		name := filepath.Base(header.Name)
		if name != u.config.BinaryName && name != u.config.BinaryName+".exe" {
			continue
		}

		binaryPath = filepath.Join(destDir, name)

		// Verify destination is still within destDir
		if !strings.HasPrefix(filepath.Clean(binaryPath), filepath.Clean(destDir)) {
			return "", fmt.Errorf("path traversal detected: %s", header.Name)
		}

		outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		// Limit extracted file size to 200MB
		if _, err := io.Copy(outFile, io.LimitReader(tr, 200*1024*1024)); err != nil {
			outFile.Close()
			return "", err
		}
		outFile.Close()
	}

	if binaryPath == "" {
		return "", fmt.Errorf("binary not found in archive")
	}

	return binaryPath, nil
}

// findChecksumAsset looks for a SHA256 checksums file in release assets
func (u *Updater) findChecksumAsset(release *Release) *Asset {
	names := []string{"checksums.txt", "SHA256SUMS", "sha256sums.txt"}
	for _, asset := range release.Assets {
		for _, name := range names {
			if strings.EqualFold(asset.Name, name) {
				return &asset
			}
		}
	}
	return nil
}

// verifyChecksum downloads checksums file and verifies the downloaded file
func (u *Updater) verifyChecksum(ctx context.Context, checksumAsset *Asset, assetName, filePath string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", checksumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return err
	}

	// Parse checksums file (format: "hash  filename" per line)
	var expectedHash string
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == assetName {
			expectedHash = parts[0]
			break
		}
	}

	if expectedHash == "" {
		return fmt.Errorf("no checksum found for %s in checksums file", assetName)
	}

	// Compute SHA-256 of downloaded file
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
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
		_ = os.Rename(tmpPath, execPath)
		return err
	}
	_ = os.Remove(tmpPath)
	
	return nil
}

// isNewerVersion compares version strings (v1.2.3 format)
func isNewerVersion(current, latest string) bool {
	// Strip 'v' prefix
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// "dev" is always older than any release
	if current == "dev" || current == "unknown" {
		return latest != "dev" && latest != "unknown"
	}
	if latest == "dev" || latest == "unknown" {
		return false
	}

	cParts := parseVersion(current)
	lParts := parseVersion(latest)

	for i := 0; i < 3; i++ {
		if lParts[i] > cParts[i] {
			return true
		}
		if lParts[i] < cParts[i] {
			return false
		}
	}
	return false
}

// parseVersion splits "1.2.3" into [1, 2, 3]
func parseVersion(v string) [3]int {
	var parts [3]int
	segments := strings.SplitN(v, ".", 3)
	for i, s := range segments {
		if i >= 3 {
			break
		}
		// Strip any pre-release suffix (e.g. "3-beta")
		if idx := strings.IndexAny(s, "-+"); idx >= 0 {
			s = s[:idx]
		}
	_, _ = fmt.Sscanf(s, "%d", &parts[i]) // #nosec G602 -- parts is pre-allocated with known size
	}
	return parts
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
