package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	m := New("/tmp/backups", 5)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.backupPath != "/tmp/backups" {
		t.Errorf("expected backupPath '/tmp/backups', got %q", m.backupPath)
	}
	if m.keepCount != 5 {
		t.Errorf("expected keepCount 5, got %d", m.keepCount)
	}
}

func TestCreate_Success(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	dataDir := filepath.Join(tmpDir, "data")

	// Create test data
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "magabot.db"), []byte("test data"), 0600); err != nil {
		t.Fatal(err)
	}

	m := New(backupDir, 5)
	info, err := m.Create(dataDir, []string{"telegram"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.Filename == "" {
		t.Error("expected non-empty filename")
	}
	if info.Size <= 0 {
		t.Error("expected positive size")
	}
	if len(info.Platforms) != 1 || info.Platforms[0] != "telegram" {
		t.Errorf("expected platforms [telegram], got %v", info.Platforms)
	}

	// Verify file exists
	fullPath := filepath.Join(backupDir, info.Filename)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Error("backup file does not exist")
	}

	// Verify archive contains manifest
	f, _ := os.Open(fullPath)
	defer f.Close()
	gr, _ := gzip.NewReader(f)
	defer gr.Close()
	tr := tar.NewReader(gr)

	foundManifest := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "manifest.json" {
			foundManifest = true
		}
	}
	if !foundManifest {
		t.Error("manifest.json not found in archive")
	}
}

func TestCreate_InvalidPath(t *testing.T) {
	// Use a path that cannot be created on any platform.
	// On Unix, /dev/null is a file so subdirs fail.
	// On Windows, NUL is a reserved device name.
	invalidPath := "/dev/null/impossible/path"
	if runtime.GOOS == "windows" {
		invalidPath = `NUL\impossible\path`
	}
	m := New(invalidPath, 5)
	_, err := m.Create(t.TempDir(), []string{})
	if err == nil {
		t.Error("expected error for invalid backup path")
	}
}

func TestRestore_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	restoreDir := filepath.Join(tmpDir, "restore")

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(restoreDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create a malicious archive with path traversal
	archivePath := filepath.Join(backupDir, "malicious.tar.gz")
	f, _ := os.Create(archivePath)
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Use a cross-platform path traversal
	maliciousPath := "../../../etc/passwd"
	hdr := &tar.Header{
		Name: maliciousPath,
		Mode: 0600,
		Size: 4,
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte("evil"))
	tw.Close()
	gw.Close()
	f.Close()

	m := New(backupDir, 5)
	err := m.Restore("malicious.tar.gz", restoreDir)
	if err == nil {
		t.Error("expected error for path traversal")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid path") {
		t.Errorf("expected 'invalid path' error, got: %v", err)
	}
}

func TestRestore_TruncatedArchive(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Write truncated gzip data
	if err := os.WriteFile(filepath.Join(backupDir, "corrupt.tar.gz"), []byte{0x1f, 0x8b, 0x08}, 0600); err != nil {
		t.Fatal(err)
	}

	m := New(backupDir, 5)
	err := m.Restore("corrupt.tar.gz", t.TempDir())
	if err == nil {
		t.Error("expected error for truncated archive")
	}
}

func TestRestore_OversizeFile(t *testing.T) {
	// We can't easily create a 500MB+ file in tests, so we verify
	// the limit constant is defined correctly.
	if maxFileExtractSize != 500*1024*1024 {
		t.Errorf("expected maxFileExtractSize to be 500MB, got %d", maxFileExtractSize)
	}
}

func TestRestore_TotalSizeLimit(t *testing.T) {
	if maxTotalExtractSize != 2*1024*1024*1024 {
		t.Errorf("expected maxTotalExtractSize to be 2GB, got %d", maxTotalExtractSize)
	}
}

func TestDelete_PathTraversal(t *testing.T) {
	m := New(t.TempDir(), 5)

	tests := []string{
		"../../../file",
		"subdir/file",
		"../../etc/passwd",
	}

	for _, name := range tests {
		err := m.Delete(name)
		if err == nil {
			t.Errorf("expected error for path traversal with %q", name)
		}
	}
}

func TestDelete_NonExistent(t *testing.T) {
	m := New(t.TempDir(), 5)
	err := m.Delete("nonexistent.tar.gz")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestList_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	m := New(tmpDir, 5)

	backups, err := m.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("expected empty list, got %d backups", len(backups))
	}
}

func TestList_NonExistentDir(t *testing.T) {
	m := New("/nonexistent/path/backups", 5)

	backups, err := m.List()
	if err != nil {
		t.Fatalf("expected nil error for non-existent dir, got: %v", err)
	}
	if backups != nil {
		t.Errorf("expected nil backups, got %v", backups)
	}
}

func TestCleanup_KeepCount(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create 3 backup files directly (to avoid timestamp collision from same-second Create calls)
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("magabot-backup-20260101-12000%d.tar.gz", i)
		f, err := os.Create(filepath.Join(backupDir, name))
		if err != nil {
			t.Fatal(err)
		}
		// Write minimal valid gzip so it counts as a backup file
		f.Close()
	}

	// Verify all 3 exist
	backups, err := New(backupDir, 5).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups before cleanup, got %d", len(backups))
	}

	// Now create a new manager with keepCount=2 and trigger cleanup via Create
	dataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "magabot.db"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	m := New(backupDir, 2)
	_, err = m.Create(dataDir, []string{})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Should have at most 2 remaining (keepCount=2)
	backups, err = m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) > 2 {
		t.Errorf("expected at most 2 backups after cleanup, got %d", len(backups))
	}
}
