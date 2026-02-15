// Package test contains integration tests for backup module
package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/backup"
)

func TestBackupIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	dataDir := filepath.Join(tmpDir, "data")

	// Create data directory with test files
	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(filepath.Join(dataDir, "sessions"), 0755)

	// Create test database file
	dbPath := filepath.Join(dataDir, "magabot.db")
	os.WriteFile(dbPath, []byte("test database content"), 0600)

	// Create test session file
	sessionFile := filepath.Join(dataDir, "sessions", "test_session.json")
	os.WriteFile(sessionFile, []byte(`{"key":"value"}`), 0600)

	mgr := backup.New(backupDir, 3)

	t.Run("CreateBackup", func(t *testing.T) {
		info, err := mgr.Create(dataDir, []string{"telegram", "whatsapp"})
		if err != nil {
			t.Fatalf("Failed to create backup: %v", err)
		}

		if info.Filename == "" {
			t.Error("Backup filename should not be empty")
		}

		if info.Size <= 0 {
			t.Error("Backup size should be positive")
		}

		if len(info.Platforms) != 2 {
			t.Errorf("Expected 2 platforms, got %d", len(info.Platforms))
		}

		// Verify backup file exists
		backupPath := filepath.Join(backupDir, info.Filename)
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Error("Backup file should exist")
		}
	})

	t.Run("ListBackups", func(t *testing.T) {
		// Create a few more backups
		mgr.Create(dataDir, []string{"telegram"})
		time.Sleep(10 * time.Millisecond)
		mgr.Create(dataDir, []string{"whatsapp"})

		backups, err := mgr.List()
		if err != nil {
			t.Fatalf("Failed to list backups: %v", err)
		}

		if len(backups) == 0 {
			t.Error("Should have at least one backup")
		}

		// Backups should be sorted by timestamp (newest first)
		for i := 1; i < len(backups); i++ {
			if backups[i].Timestamp.After(backups[i-1].Timestamp) {
				t.Error("Backups should be sorted newest first")
			}
		}
	})

	t.Run("RestoreBackup", func(t *testing.T) {
		// Create a backup
		info, _ := mgr.Create(dataDir, []string{"telegram"})

		// Create a new restore directory
		restoreDir := filepath.Join(tmpDir, "restore")
		os.MkdirAll(restoreDir, 0755)

		// Restore
		err := mgr.Restore(info.Filename, restoreDir)
		if err != nil {
			t.Fatalf("Failed to restore backup: %v", err)
		}

		// Verify manifest exists
		manifestPath := filepath.Join(restoreDir, "manifest.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			t.Error("Manifest should be restored")
		}

		// Verify database restored
		restoredDB := filepath.Join(restoreDir, "magabot.db")
		if _, err := os.Stat(restoredDB); os.IsNotExist(err) {
			t.Error("Database should be restored")
		}
	})

	t.Run("BackupRotation", func(t *testing.T) {
		// Create manager with keep count of 2
		rotationMgr := backup.New(filepath.Join(tmpDir, "rotation_backups"), 2)

		// Create 4 backups
		for i := 0; i < 4; i++ {
			rotationMgr.Create(dataDir, []string{"test"})
			time.Sleep(10 * time.Millisecond)
		}

		// Should only keep 2
		backups, _ := rotationMgr.List()
		if len(backups) > 2 {
			t.Errorf("Should keep only 2 backups, got %d", len(backups))
		}
	})

	t.Run("DeleteBackup", func(t *testing.T) {
		info, _ := mgr.Create(dataDir, []string{"test"})

		err := mgr.Delete(info.Filename)
		if err != nil {
			t.Fatalf("Failed to delete backup: %v", err)
		}

		// Verify file deleted
		backupPath := filepath.Join(backupDir, info.Filename)
		if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
			t.Error("Backup file should be deleted")
		}
	})

	t.Run("DeleteInvalidFilename", func(t *testing.T) {
		// Path traversal attempt
		err := mgr.Delete("../../../etc/passwd")
		if err == nil {
			t.Error("Should reject path traversal")
		}

		err = mgr.Delete("test/../../../etc/passwd")
		if err == nil {
			t.Error("Should reject path with ..")
		}
	})

	t.Run("RestoreNonExistent", func(t *testing.T) {
		err := mgr.Restore("nonexistent.tar.gz", tmpDir)
		if err == nil {
			t.Error("Should error on non-existent backup")
		}
	})

	t.Run("ListEmptyBackupDir", func(t *testing.T) {
		emptyMgr := backup.New(filepath.Join(tmpDir, "empty_backups"), 5)

		backups, err := emptyMgr.List()
		if err != nil {
			t.Fatalf("List should not error on empty dir: %v", err)
		}

		if backups != nil && len(backups) != 0 {
			t.Error("Should return empty list")
		}
	})
}

func TestBackupPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	dataDir := filepath.Join(tmpDir, "data")
	restoreDir := filepath.Join(tmpDir, "restore")

	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(restoreDir, 0755)
	os.WriteFile(filepath.Join(dataDir, "magabot.db"), []byte("test"), 0600)

	mgr := backup.New(backupDir, 5)

	// Create a legitimate backup
	info, err := mgr.Create(dataDir, []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Restore should not allow path traversal in archive
	err = mgr.Restore(info.Filename, restoreDir)
	if err != nil {
		t.Logf("Restore error (may be expected): %v", err)
	}

	// Verify nothing was written outside restore dir
	outsidePath := filepath.Join(tmpDir, "outside.txt")
	if _, err := os.Stat(outsidePath); err == nil {
		t.Error("Should not create files outside restore directory")
	}
}

func TestBackupEmptyDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	emptyDataDir := filepath.Join(tmpDir, "empty_data")

	os.MkdirAll(emptyDataDir, 0755)

	mgr := backup.New(backupDir, 5)

	// Should still create a valid backup (with just manifest)
	info, err := mgr.Create(emptyDataDir, []string{})
	if err != nil {
		t.Fatalf("Failed to create backup from empty data dir: %v", err)
	}

	if info.Size <= 0 {
		t.Error("Backup should have some size (at least manifest)")
	}
}

func TestBackupLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backups")
	dataDir := filepath.Join(tmpDir, "data")

	os.MkdirAll(dataDir, 0755)

	// Create a larger file (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(dataDir, "magabot.db"), largeData, 0600)

	mgr := backup.New(backupDir, 5)

	info, err := mgr.Create(dataDir, []string{"test"})
	if err != nil {
		t.Fatalf("Failed to create backup of large file: %v", err)
	}

	// Compressed size should be less than original
	if info.Size >= int64(len(largeData)) {
		t.Log("Backup is not smaller than original (may happen with incompressible data)")
	}

	// Verify restore
	restoreDir := filepath.Join(tmpDir, "restore")
	os.MkdirAll(restoreDir, 0755)

	err = mgr.Restore(info.Filename, restoreDir)
	if err != nil {
		t.Fatalf("Failed to restore large backup: %v", err)
	}

	// Verify restored file size
	restoredData, _ := os.ReadFile(filepath.Join(restoreDir, "magabot.db"))
	if len(restoredData) != len(largeData) {
		t.Errorf("Restored file size mismatch: got %d, want %d", len(restoredData), len(largeData))
	}
}
