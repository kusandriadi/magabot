// Package backup handles backup and restore operations
package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manager handles backup operations
type Manager struct {
	backupPath string
	keepCount  int
}

// BackupInfo contains metadata about a backup
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
	Platforms []string  `json:"platforms"`
}

// New creates a new backup manager
func New(backupPath string, keepCount int) *Manager {
	return &Manager{
		backupPath: backupPath,
		keepCount:  keepCount,
	}
}

// Create creates a new backup
func (m *Manager) Create(dataDir string, platforms []string) (*BackupInfo, error) {
	if err := os.MkdirAll(m.backupPath, 0700); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now()
	filename := fmt.Sprintf("magabot-backup-%s.tar.gz", timestamp.Format("20060102-150405"))
	backupFile := filepath.Join(m.backupPath, filename)

	// Create tar.gz archive
	f, err := os.OpenFile(backupFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Files to backup
	filesToBackup := []string{
		filepath.Join(dataDir, "magabot.db"),
		filepath.Join(dataDir, "sessions"),
	}

	for _, path := range filesToBackup {
		if err := m.addToArchive(tw, path, dataDir); err != nil {
			// Skip if file doesn't exist
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("add %s: %w", path, err)
		}
	}

	// Add manifest
	manifest := map[string]interface{}{
		"version":    "1.0",
		"timestamp":  timestamp,
		"platforms":  platforms,
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	
	header := &tar.Header{
		Name:    "manifest.json",
		Mode:    0600,
		Size:    int64(len(manifestData)),
		ModTime: timestamp,
	}
	tw.WriteHeader(header)
	tw.Write(manifestData)

	// Get final size
	tw.Close()
	gw.Close()
	f.Close()

	stat, _ := os.Stat(backupFile)
	
	info := &BackupInfo{
		Filename:  filename,
		Timestamp: timestamp,
		Size:      stat.Size(),
		Platforms: platforms,
	}

	// Cleanup old backups
	m.cleanup()

	return info, nil
}

// addToArchive adds a file or directory to the archive
func (m *Manager) addToArchive(tw *tar.Writer, path, baseDir string) error {
	return filepath.Walk(path, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(baseDir, file)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if fi.IsDir() {
			return nil
		}

		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
}

// Restore restores from a backup
func (m *Manager) Restore(filename, dataDir string) error {
	backupFile := filepath.Join(m.backupPath, filename)
	
	f, err := os.Open(backupFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dataDir, header.Name)

		// Prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dataDir)) {
			return fmt.Errorf("invalid path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// List lists available backups
func (m *Manager) List() ([]BackupInfo, error) {
	entries, err := os.ReadDir(m.backupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Parse timestamp from filename
		timestamp := info.ModTime()
		if parts := strings.Split(entry.Name(), "-"); len(parts) >= 3 {
			if t, err := time.Parse("20060102-150405", strings.TrimSuffix(parts[2]+"-"+strings.Split(parts[3], ".")[0], ".tar.gz")); err == nil {
				timestamp = t
			}
		}

		backups = append(backups, BackupInfo{
			Filename:  entry.Name(),
			Timestamp: timestamp,
			Size:      info.Size(),
		})
	}

	// Sort by timestamp descending
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// cleanup removes old backups keeping only keepCount
func (m *Manager) cleanup() {
	backups, err := m.List()
	if err != nil || len(backups) <= m.keepCount {
		return
	}

	for _, b := range backups[m.keepCount:] {
		os.Remove(filepath.Join(m.backupPath, b.Filename))
	}
}

// Delete deletes a specific backup
func (m *Manager) Delete(filename string) error {
	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "..") {
		return fmt.Errorf("invalid filename")
	}
	return os.Remove(filepath.Join(m.backupPath, filename))
}
